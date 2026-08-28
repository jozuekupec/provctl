# provctl — implementační zadání (v1, pro předání implementačnímu agentovi)

**Status dokumentu:** závazné zadání pro implementaci v0.1.
**Cílový čtenář:** implementační agent (Codex) + autor projektu.
**Jazyk kódu, commitů, identifikátorů a komentářů:** angličtina. Tento dokument je česky.

---

## 0. Jak číst tento dokument

Dokument rozlišuje tři úrovně:

- **[MUST]** — povinné, nesmí být změněno bez souhlasu autora.
- **[SHOULD]** — doporučené; odchylka je možná, ale musí být v reportu odůvodněna.
- **[LATER]** — mimo rozsah v0.1, ale datový model a API to nesmí znemožnit.

Dále:

- **FAKT** — ověřeno nebo triviálně ověřitelné.
- **PŘEDPOKLAD** — nebylo ověřeno na cílovém serveru, implementace to musí ošetřit defenzivně.

> **Základní pravidlo pro implementaci:** nevymýšlet si. Co není v tomto dokumentu a nedá se bezpečně odvodit ze zdrojového kódu nebo oficiální dokumentace, se má vypsat do sekce „Otevřené otázky" v reportu, ne domyslet.

---

## 1. Pojmenování a základní rozhodnutí

### 1.1 Název

**[MUST]** Nástroj se jmenuje **`provctl`** (*provisioning control*).

Odůvodnění volby (aby bylo možné rozhodnutí případně revidovat):

- `myhosting` je příliš generické, špatně se hledá a v APT namespace je to nevhodné jméno pro veřejný balíček.
- `hostctl` je už obsazené existujícím open-source nástrojem pro správu `/etc/hosts` — kolize jména i v APT.
- `provctl` je krátké, dobře se píše, popisuje funkci a nekoliduje se známým balíčkem v Debianu.

Změna jména je **jeden find/replace** — název se v kódu nikdy nepíše natvrdo do řetězců rozptýlených po projektu, ale je definován na jediném místě:

```go
// internal/meta/meta.go
package meta

const (
    Name        = "provctl"
    ConfigDir   = "/etc/provctl"
    StateDir    = "/var/lib/provctl"
    LogDir      = "/var/log/provctl"
    TemplateDir = "/usr/share/provctl/templates"
    FilePrefix  = "provctl-" // prefix všech generovaných systémových souborů
)
```

**[MUST]** Verze se do binárky vkládá přes `-ldflags -X`, ne konstantou v kódu.

### 1.2 Klíčová architektonická rozhodnutí (shrnutí)

| # | Rozhodnutí | Důvod |
|---|---|---|
| 1 | SQLite je *source of truth*, systémové konfigurace jsou *generované artefakty* | umožňuje reconcile, drift detection, rollback |
| 2 | Core / CLI / TUI — CLI i TUI jsou pouze frontendy nad Core | žádná business logika v UI |
| 3 | Jediný `.deb` balíček v v0.1 | rozdělení na `provctl-core`/`provctl-cli` lze doplnit později bez breaking change |
| 4 | Bez cgo (`CGO_ENABLED=0`) | statická binárka, snadný cross-build amd64/arm64 |
| 5 | Žádné spouštění shellu z uživatelských dat | bezpečnost root nástroje |
| 6 | Instalace balíčku nikdy nemění hostingy | `apt upgrade` nesmí sáhnout na zákaznická data |

### 1.3 Závazný stack

**[MUST]**

| Účel | Knihovna | Poznámka |
|---|---|---|
| CLI | `github.com/spf13/cobra` | |
| TUI | `github.com/charmbracelet/bubbletea` + `lipgloss` | |
| SQLite | `modernc.org/sqlite` | **pure Go**, nesmí být použit `mattn/go-sqlite3` (vyžaduje cgo) |
| TOML | `github.com/BurntSushi/toml` | |
| Šablony | `text/template` ze stdlib | žádná externí šablonovací knihovna |
| Testy | `testing` + `github.com/google/go-cmp` | |

**[MUST]** Žádné další závislosti bez odůvodnění v reportu. Zejména: žádný ORM, žádný DI framework, žádný logging framework (stačí `log/slog` ze stdlib).

**[MUST]** Minimální verze Go: 1.22 (kvůli `log/slog` a `os.Root`-like vzorům; pokud je v build prostředí novější, použij `go.mod` s aktuální stabilní verzí).

---

## 2. Cílová platforma a předpoklady

### 2.1 Cíl

**[MUST]** Debian 13 (trixie), architektury `amd64` a `arm64`.

Komponenty:

- Apache 2.4 (balíček `apache2`)
- PHP-FPM — **verze se nesmí hardcodovat** (viz níže)
- MariaDB (lokální nebo vzdálená)
- Certbot (Let's Encrypt)

### 2.2 PHP — detekce, ne hardcode

**PŘEDPOKLAD:** Debian 13 ve výchozích repozitářích neobsahuje PHP 8.5; ta se instaluje z třetí strany (Sury). Autor si není jistý, co na cílovém serveru je.

**[MUST]** Implementace tento problém **obchází detekcí**, ne konfigurací:

1. Při startu provctl načte seznam dostupných PHP-FPM verzí skenem `/etc/php/*/fpm/pool.d` (adresář existuje ⇒ verze je nainstalovaná).
2. Ověří existenci binárky `/usr/sbin/php-fpm<ver>` a systemd unity `php<ver>-fpm.service`.
3. Výchozí verze = hodnota z configu, pokud je v seznamu; jinak **nejvyšší detekovaná verze**.
4. Pokud config uvádí verzi, která neexistuje, `provctl` **selže s jasnou chybou** a vypíše dostupné verze. Nikdy nesmí tiše fallbacknout.

```toml
[php]
# prázdné = automaticky nejvyšší detekovaná verze
default_version = ""
```

**[MUST]** Nikde v kódu, šablonách ani testech nesmí být natvrdo `8.4` ani `8.5`.

### 2.3 Kontrola prostředí — `provctl doctor`

**[MUST]** Příkaz `provctl doctor` ověří a vypíše:

- běží jako root (`os.Geteuid() == 0`)
- `apache2` nainstalován, verze, běží
- povolené Apache moduly: `proxy`, `proxy_fcgi`, `proxy_http`, `ssl`, `rewrite`, `headers`
- detekované PHP-FPM verze a jejich stav
- dostupnost `mysql`/`mariadb` klienta a konektivita
- dostupnost `certbot`
- **mechanismus obnovy certifikátů** (viz §11.2): stav `certbot.timer`, existence `/etc/cron.d/certbot`, přítomnost vlastního cron záznamu s `certbot` v root crontabu — a **varování, pokud jsou aktivní dva mechanismy současně**
- přítomnost provctl deploy hooku v `/etc/letsencrypt/renewal-hooks/deploy/`
- existence a práva `/etc/provctl`, `/var/lib/provctl`, `/var/log/provctl`, vhosts root
- zapisovatelnost SQLite, verze schématu, verze konfiguračního souboru (§23.1)

Výstup: seznam kontrol se stavem `OK / WARN / FAIL` + návrhem konkrétní nápravy. Exit kód 1, pokud je jakýkoli `FAIL`.

**[MUST]** `provctl doctor` **nic neopravuje**. Opravy dělá `provctl bootstrap` (viz §21.2).

---

## 3. Architektura a struktura projektu

```
provctl/
├── cmd/provctl/main.go          # pouze wiring: config → core → cobra/bubbletea
├── internal/
│   ├── meta/                    # název, cesty, verze
│   ├── config/                  # načtení a validace config.toml
│   ├── domain/                  # čisté datové typy + validace, ŽÁDNÉ I/O
│   │   ├── subscription.go
│   │   ├── website.go
│   │   ├── database.go
│   │   ├── certificate.go
│   │   ├── backup.go
│   │   └── validate.go
│   ├── repository/sqlite/       # perzistence, migrace schématu
│   ├── system/                  # JEDINÉ místo, které se dotýká OS
│   │   ├── commander.go         # spouštění procesů (interface)
│   │   ├── fs.go                # souborový systém (interface)
│   │   ├── systemd.go
│   │   ├── users.go             # useradd/usermod/userdel
│   │   └── fake/                # fake implementace pro testy
│   ├── plan/                    # executor kroků + rollback (§7)
│   ├── render/                  # šablony → konfigurační soubory
│   ├── service/                 # business logika (Core)
│   │   ├── subscription.go
│   │   ├── website.go
│   │   ├── apache.go
│   │   ├── phpfpm.go
│   │   ├── mariadb.go
│   │   ├── ssl.go
│   │   ├── backup.go
│   │   ├── cron.go
│   │   ├── ssh.go
│   │   ├── health.go
│   │   └── reconcile.go
│   ├── audit/
│   └── cli/                     # cobra příkazy
├── tui/
│   ├── model/
│   ├── screens/
│   └── components/
├── templates/                   # embed + instalace do /usr/share/provctl/templates
│   ├── apache/
│   ├── fpm/
│   └── logrotate/
├── packaging/
│   ├── nfpm.yaml
│   ├── debian/postinst
│   ├── debian/prerm
│   └── debian/postrm
├── .github/workflows/release.yml
└── docs/
```

### 3.1 Nepřekročitelné vrstvení

**[MUST]**

```
tui/  ──┐
cli/  ──┼──▶ service/ ──▶ plan/ ──▶ system/ ──▶ OS
        │        │
        │        ├──▶ repository/ ──▶ SQLite
        │        └──▶ render/ ──▶ templates
        └──▶ domain/  (čisté typy, bez I/O)
```

- `tui/` a `internal/cli/` **nesmí** importovat `internal/system` ani `internal/repository`.
- `internal/domain` **nesmí** importovat nic z projektu kromě sebe.
- `internal/service` komunikuje s OS **výhradně** přes rozhraní z `internal/system`.

**[MUST]** V CI běží kontrola vrstvení (jednoduchý test nad `go list -deps`, nebo `depguard`). Porušení = failing test.

---

## 4. Rozhraní k systému (`internal/system`)

Toto je klíč k testovatelnosti. Bez něj nelze projekt otestovat bez rootu a bez Debianu.

```go
package system

// Commander spouští externí procesy. Implementace NIKDY nespouští shell.
type Commander interface {
    // Run spustí příkaz s explicitními argumenty. Žádná interpolace do stringu.
    Run(ctx context.Context, name string, args ...string) (Result, error)
    // RunWithStdin je určeno pro předávání citlivých dat (hesla, SQL),
    // aby se neobjevila v `ps` ani v audit logu.
    RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (Result, error)
}

type Result struct {
    ExitCode int
    Stdout   string
    Stderr   string
    Duration time.Duration
}

type FS interface {
    Stat(path string) (os.FileInfo, error)
    ReadFile(path string) ([]byte, error)
    // WriteFileAtomic zapíše do temp souboru ve stejném adresáři a přejmenuje.
    WriteFileAtomic(path string, data []byte, mode os.FileMode) error
    Remove(path string) error
    MkdirAll(path string, mode os.FileMode) error
    Chown(path string, uid, gid int) error
    Chmod(path string, mode os.FileMode) error
    Symlink(oldname, newname string) error
    ReadDir(path string) ([]os.DirEntry, error)
    EvalSymlinks(path string) (string, error)
}

type Systemd interface {
    Reload(ctx context.Context, unit string) error
    Restart(ctx context.Context, unit string) error
    Start(ctx context.Context, unit string) error
    Stop(ctx context.Context, unit string) error
    IsActive(ctx context.Context, unit string) (bool, error)
    Enable(ctx context.Context, unit string) error
    Disable(ctx context.Context, unit string) error
}

type Users interface {
    Lookup(name string) (*user.User, error)
    Create(ctx context.Context, opts CreateUserOptions) error
    SetShell(ctx context.Context, name, shell string) error
    LockPassword(ctx context.Context, name string) error
    SetPassword(ctx context.Context, name, password string) error // přes chpasswd + stdin
    Delete(ctx context.Context, name string, removeHome bool) error
}
```

### 4.1 Allowlist binárek

**[MUST]** `Commander` má v produkční implementaci **allowlist povolených binárek** s absolutními cestami. Cokoli mimo allowlist = chyba, ne spuštění.

Povolené: `apachectl`/`apache2ctl`, `systemctl`, `useradd`, `usermod`, `userdel`, `chpasswd`, `crontab`, `certbot`, `mysql`, `mysqldump`, `tar`, `zstd`, `du`, `php-fpm<ver>`, `openssl`, `dig`/`getent`.

**[MUST]** Zakázáno kdekoli v projektu:

```go
exec.Command("sh", "-c", anything)   // NIKDY
exec.Command("bash", "-c", anything) // NIKDY
```

**[MUST]** V CI běží grep-test, který selže, pokud se v repozitáři objeví `"sh", "-c"` nebo `"bash", "-c"` mimo testovací fixtures.

---

## 5. Datový model a SQLite schéma

### 5.1 Zásadní změna oproti původnímu návrhu: tabulka `domains`

Původní návrh měl `websites` + `website_aliases`. To znemožňuje vynutit **globální unikátnost domény** (doména nesmí existovat dvakrát ani jako primární, ani jako alias). Proto:

**[MUST]** Jedna tabulka `domains` s `UNIQUE(name)`, kde `is_primary` označuje primární doménu webu.

### 5.2 Schéma (migrace 0001)

```sql
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE subscriptions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT    NOT NULL UNIQUE,
    unix_user         TEXT    NOT NULL UNIQUE,
    unix_uid          INTEGER,
    home              TEXT    NOT NULL,
    status            TEXT    NOT NULL CHECK (status IN ('active','suspended','archived')),
    php_version       TEXT,                 -- NULL = použij default z configu
    php_max_children  INTEGER NOT NULL DEFAULT 10,
    php_memory_limit  TEXT    NOT NULL DEFAULT '256M',
    php_upload_max    TEXT    NOT NULL DEFAULT '64M',
    php_max_exec_time INTEGER NOT NULL DEFAULT 60,
    ssh_access        TEXT    NOT NULL DEFAULT 'none'
                              CHECK (ssh_access IN ('none','key','password','key+password')),
    quota_disk_bytes  INTEGER,              -- NULL = bez limitu
    quota_websites    INTEGER,
    quota_databases   INTEGER,
    quota_backups     INTEGER,
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL
);

CREATE TABLE websites (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    type            TEXT    NOT NULL CHECK (type IN ('static','php-fpm','proxy','redirect')),
    document_root   TEXT,                   -- povinné pro static/php-fpm
    target          TEXT,                   -- povinné pro proxy (http://127.0.0.1:8080) a redirect (URL)
    redirect_code   INTEGER CHECK (redirect_code IN (301,302)),
    php_version     TEXT,                   -- NULL = zdědit ze subscription
    enabled         INTEGER NOT NULL DEFAULT 1,
    ssl_enabled     INTEGER NOT NULL DEFAULT 0,
    force_https     INTEGER NOT NULL DEFAULT 0,
    hsts            INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT    NOT NULL,
    updated_at      TEXT    NOT NULL
);

CREATE TABLE domains (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER NOT NULL REFERENCES websites(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL UNIQUE,     -- vždy ASCII/punycode, lowercase
    unicode    TEXT,                        -- původní podoba pro zobrazení
    is_primary INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL
);
CREATE UNIQUE INDEX idx_domains_primary ON domains(website_id) WHERE is_primary = 1;

CREATE TABLE databases (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    name            TEXT    NOT NULL UNIQUE,
    db_user         TEXT    NOT NULL,
    db_host         TEXT    NOT NULL DEFAULT 'localhost',
    charset         TEXT    NOT NULL DEFAULT 'utf8mb4',
    collation       TEXT    NOT NULL DEFAULT 'utf8mb4_unicode_ci',
    created_at      TEXT    NOT NULL
);
-- POZOR: hesla se do této tabulky NIKDY neukládají.

CREATE TABLE certificates (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    lineage         TEXT    NOT NULL UNIQUE,  -- název adresáře v /etc/letsencrypt/live
    primary_domain  TEXT    NOT NULL,
    sans            TEXT    NOT NULL,          -- JSON pole domén
    issuer          TEXT,
    not_before      TEXT,
    not_after       TEXT,
    last_checked_at TEXT
);

CREATE TABLE cron_jobs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    schedule        TEXT    NOT NULL,
    command         TEXT    NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    comment         TEXT,
    created_at      TEXT    NOT NULL
);

CREATE TABLE ssh_keys (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    comment         TEXT,
    fingerprint     TEXT    NOT NULL,
    public_key      TEXT    NOT NULL,
    created_at      TEXT    NOT NULL
);

CREATE TABLE backups (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    path            TEXT    NOT NULL UNIQUE,
    size_bytes      INTEGER,
    status          TEXT    NOT NULL CHECK (status IN ('running','complete','failed')),
    started_at      TEXT    NOT NULL,
    finished_at     TEXT,
    error           TEXT
);

-- Journal operací: umožňuje detekci a dokončení/rollback po pádu procesu.
CREATE TABLE operations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    action      TEXT    NOT NULL,       -- např. 'subscription.create'
    target      TEXT    NOT NULL,
    actor       TEXT,                   -- SUDO_USER nebo 'root'
    status      TEXT    NOT NULL CHECK (status IN ('running','done','failed','rolled_back','inconsistent')),
    plan_json   TEXT    NOT NULL,       -- kroky a jejich stav
    error       TEXT,
    started_at  TEXT    NOT NULL,
    finished_at TEXT
);
```

### 5.3 Migrace

**[MUST]** Migrace jsou očíslované SQL soubory embedované v binárce (`embed.FS`), aplikované v transakci, verze v `schema_migrations`. Při startu: pokud je verze schématu vyšší než verze binárky ⇒ **odmítnout běh** s jasnou chybou (starší binárka nesmí pracovat s novějším schématem).

---

## 6. Filesystem layout a práva

### 6.1 Systémové cesty (vlastní provctl)

| Cesta | Vlastník | Práva | Poznámka |
|---|---|---|---|
| `/usr/bin/provctl` | `root:root` | `0755` | |
| `/etc/provctl/` | `root:root` | `0755` | |
| `/etc/provctl/config.toml` | `root:root` | `0640` | conffile |
| `/etc/provctl/templates/` | `root:root` | `0755` | volitelné override šablon |
| `/usr/share/provctl/templates/` | `root:root` | `0755` | výchozí šablony, přepisované upgradem |
| `/var/lib/provctl/` | `root:root` | `0700` | |
| `/var/lib/provctl/provctl.db` | `root:root` | `0600` | |
| `/var/lib/provctl/acme-challenge/` | `root:root` | `0755` | centrální webroot pro ACME |
| `/var/log/provctl/` | `root:adm` | `0750` | |
| `/var/log/provctl/audit.jsonl` | `root:adm` | `0640` | |
| `/run/provctl.lock` | `root:root` | `0600` | |

### 6.2 Subscription (výchozí root `/var/www/vhosts`)

```
/var/www/vhosts/                          root:root      0755
└── acme/                                 acme:acme      0751   ← o+x pro traversal, ne o+r
    ├── sites/                            acme:acme      0751
    │   └── example.cz/                   acme:acme      0751
    │       ├── public/                   acme:acme      0755   ← Apache musí číst
    │       ├── app/                      acme:acme      0750   ← Apache NESMÍ číst
    │       └── storage/                  acme:acme      0750
    ├── tmp/                              acme:acme      0700
    │   └── sessions/                     acme:acme      0700   ← per-subscription PHP session
    ├── private/                          acme:acme      0700
    └── .ssh/                             acme:acme      0700
```

**[MUST]** Model práv — toto je bezpečnostní jádro, nesmí se zjednodušit:

- Adresáře na cestě k document rootu mají `0751` (`o+x`, **ne** `o+r`): `www-data` projde, ale nemůže vypsat obsah, a **jiná subscription nemůže číst cizí soubory**.
- `public/` má `0755` a soubory `0644` — Apache servíruje statiku jako `www-data`.
- `app/`, `storage/`, `private/`, `tmp/` mají `0750`/`0700` — čte je jen PHP-FPM, který běží jako uživatel subscription.
- **Nikdy** nepřidávat `www-data` do skupiny subscription. Prolomilo by to izolaci mezi zákazníky.
- `umask` pro operace provctl: `0022`, explicitně nastavovaný, ne dědičný.

### 6.3 Logy — bezpečnostní změna oproti původnímu návrhu

**Problém:** Apache otevírá logy **jako root**, ještě před dropnutím privilegií. Pokud by adresář s logy vlastnil uživatel subscription, mohl by tam podstrčit symlink a nechat root přepsat libovolný soubor v systému (privilege escalation).

**[MUST]** Logy tedy **nesmí** ležet v home subscription:

```
/var/log/provctl/acme/                    root:acme   0750
└── example.cz/                           root:acme   0750
    ├── access.log                        root:acme   0640
    └── error.log                         root:acme   0640
```

- provctl **předvytvoří** prázdné log soubory se správnými právy před prvním reloadem Apache (Apache do existujícího souboru jen appenduje a práva nemění).
- logrotate je udržuje přes `create 0640 root acme`.
- V home subscription může být **[LATER]** read-only pohled; v v0.1 se logy čtou přes `provctl website logs`.

---

## 7. Operační jádro: plán, rollback, zámek, idempotence

Toto je nejdůležitější část a v původním návrhu chyběla. Bez ní zůstane systém po první chybě uprostřed operace v nekonzistentním stavu.

### 7.1 Krok a plán

```go
package plan

type Step struct {
    Name   string
    Do     func(ctx context.Context) error
    Undo   func(ctx context.Context) error // nil = krok není vratný
    // Idempotent: pokud true, Do smí být zavoláno opakovaně bez efektu navíc.
    Idempotent bool
}

type Plan struct {
    Action string
    Target string
    Steps  []Step
}
```

**[MUST]** Executor:

1. Zapíše do `operations` řádek se `status='running'` a serializovaným plánem.
2. Provádí kroky postupně; po každém aktualizuje `plan_json`.
3. Při chybě spouští `Undo` **provedených kroků v opačném pořadí**.
4. Pokud `Undo` uspěje ⇒ `status='rolled_back'`. Pokud `Undo` selže ⇒ `status='inconsistent'` a **hlasitá chyba na výstupu** včetně přesného popisu, co zůstalo v systému a jak to ručně dočistit.
5. Úspěch ⇒ `status='done'`.

**[MUST]** Commit do SQLite (zápis entity, např. nové subscription) je **poslední krok plánu**, ne první. Databáze nesmí obsahovat záznam o něčem, co v systému nevzniklo.

**[MUST]** Při startu provctl zkontroluje `operations` na `status='running'` (pád procesu) a vypíše varování s ID operace a nabídne `provctl operation inspect <id>`.

### 7.2 Zámek

**[MUST]** Každá mutující operace drží exkluzivní `flock` na `/run/provctl.lock` po celou dobu. Read-only příkazy (`list`, `show`, `health`) zámek neberou. Při nedostupnosti zámku: čekat max. `lock_timeout` (default 30 s), pak selhat s informací, která operace zámek drží (PID + akce zapsané do lock souboru).

### 7.3 Idempotence a reconcile

**[MUST]** Generování konfigurací je čistá funkce `stav v DB → obsah souborů`. Z toho plyne:

- `provctl reconcile` přegeneruje **všechny** spravované soubory z DB a doreloaduje služby. Musí být bezpečně opakovatelný.
- `provctl reconcile --dry-run` vypíše unified diff mezi aktuálním a vygenerovaným stavem a **nic nezmění**. Exit kód 0 = žádný drift, 2 = drift nalezen.
- Každý generovaný soubor začíná hlavičkou:

```apache
# ============================================================
# GENERATED BY PROVCTL — DO NOT EDIT
# subscription: acme
# website:      example.cz
# generated:    2026-08-28T10:15:00Z
# source-hash:  sha256:ab12...
# Manual changes will be overwritten by `provctl reconcile`.
# ============================================================
```

**[MUST]** provctl spravuje **výhradně** soubory s prefixem `provctl-`. Soubor bez tohoto prefixu nikdy nesmaže ani nepřepíše. Pokud narazí na kolizi jména bez prefixu, je to chyba, ne důvod k přepsání.

### 7.4 `--dry-run` globálně

**[MUST]** Každý mutující CLI příkaz podporuje `--dry-run`, který vypíše plán (seznam kroků + diff souborů + příkazy, které by se spustily) a **neprovede nic**.

---

## 8. Apache

### 8.1 Generované soubory

```
/etc/apache2/sites-available/provctl-<subscription>-<domain>.conf
/etc/apache2/sites-enabled/provctl-<subscription>-<domain>.conf   → symlink
```

**[MUST]** Enable/disable se dělá **přímou správou symlinku**, ne přes `a2ensite`/`a2dissite` (předvídatelnost, žádné parsování výstupu, funguje i v testech s fake FS).

### 8.2 Atomická změna

**[MUST]** Sekvence pro každou změnu Apache konfigurace:

```
1. záloha stávajícího obsahu do paměti (pro Undo)
2. zápis nového souboru atomicky (temp + rename ve stejném adresáři)
3. apachectl configtest
4a. FAIL  → obnovit původní stav, znovu configtest, vrátit chybu i stderr configtestu
4b. OK    → systemctl reload apache2
5. ověřit `systemctl is-active apache2`
```

**[MUST]** Nikdy `systemctl restart apache2` automaticky. Restart pouze na explicitní `provctl apache restart`.

**Známé omezení, které musí implementace ošetřit (FAKT):** `apachectl configtest` testuje **celou** konfiguraci serveru. Pokud je v systému už dřív existující chyba mimo provctl, configtest selže i při korektní změně. Proto:

**[MUST]** Před první změnou v rámci operace se spustí *baseline* configtest. Pokud selže už baseline, provctl operaci **odmítne** s hláškou „Apache konfigurace je vadná ještě před touto změnou" a vypíše výstup. Nesmí to interpretovat jako chybu vlastní změny a nesmí rollbackovat cizí soubory.

### 8.3 Šablona: `php-fpm` vhost (HTTP)

```apache
<VirtualHost *:80>
    ServerName {{ .PrimaryDomain }}
{{- range .Aliases }}
    ServerAlias {{ . }}
{{- end }}

    DocumentRoot {{ .DocumentRoot }}

    Alias /.well-known/acme-challenge/ {{ .AcmeChallengeRoot }}/
    <Directory "{{ .AcmeChallengeRoot }}">
        Require all granted
        Options None
        AllowOverride None
    </Directory>

{{- if .ForceHTTPS }}
    RewriteEngine On
    RewriteCond %{REQUEST_URI} !^/\.well-known/acme-challenge/
    RewriteRule ^ https://%{SERVER_NAME}%{REQUEST_URI} [R=301,L]
{{- else }}
    <Directory "{{ .DocumentRoot }}">
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    <FilesMatch \.php$>
        SetHandler "proxy:unix:{{ .FpmSocket }}|fcgi://localhost"
    </FilesMatch>
    <Proxy "fcgi://localhost">
        ProxySet timeout={{ .ProxyTimeout }}
    </Proxy>
{{- end }}

    ErrorLog  {{ .LogDir }}/error.log
    CustomLog {{ .LogDir }}/access.log combined
</VirtualHost>
```

**[MUST]** Pozn.: `.well-known/acme-challenge` musí zůstat dostupné i při `ForceHTTPS`, jinak selže obnova certifikátu.

### 8.4 Šablona: SSL vhost

Generuje se **pouze pokud certifikát fyzicky existuje** (viz §11).

```apache
<VirtualHost *:443>
    ServerName {{ .PrimaryDomain }}
{{- range .Aliases }}
    ServerAlias {{ . }}
{{- end }}

    DocumentRoot {{ .DocumentRoot }}

    SSLEngine on
    SSLCertificateFile    {{ .CertPath }}/fullchain.pem
    SSLCertificateKeyFile {{ .CertPath }}/privkey.pem

{{- if .HSTS }}
    Header always set Strict-Transport-Security "max-age=31536000"
{{- end }}

    <Directory "{{ .DocumentRoot }}">
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    <FilesMatch \.php$>
        SetHandler "proxy:unix:{{ .FpmSocket }}|fcgi://localhost"
    </FilesMatch>

    ErrorLog  {{ .LogDir }}/error.log
    CustomLog {{ .LogDir }}/access.log combined
</VirtualHost>
```

### 8.5 Šablona: `proxy`

```apache
    ProxyPreserveHost On
    ProxyRequests Off
    ProxyPass        / {{ .Target }}/
    ProxyPassReverse / {{ .Target }}/
    RequestHeader set X-Forwarded-Proto "{{ .Scheme }}"
```

**[MUST]** `Target` musí projít validací: pouze schéma `http`, host `127.0.0.1` / `localhost` / `::1` (nebo hosty z allowlistu v configu), povinný port v rozsahu 1024–65535. Cílem je zabránit tomu, aby se z proxy stal otevřený relay do interní sítě (SSRF).

### 8.6 Šablona: `redirect`

```apache
    Redirect {{ .RedirectCode }} / {{ .Target }}
```

### 8.7 Default (catch-all) vhost — chybělo v původním návrhu

**Problém:** požadavek na neznámou doménu nebo přímo na IP servíruje Apache prvním vhostem v abecedním pořadí. To znamená nechtěné vystavení cizího webu.

**[MUST]** `provctl bootstrap` vytvoří `/etc/apache2/sites-available/provctl-000-default.conf` (symlink pojmenovaný tak, aby se načetl jako první), který na `*:80` i `*:443` vrací `403` a nemá žádný DocumentRoot v prostoru zákazníků. Pro `:443` použije self-signed certifikát vygenerovaný přes `openssl` do `/var/lib/provctl/default-ssl/`.

---

## 9. PHP-FPM

### 9.1 Pool na subscription

`/etc/php/<ver>/fpm/pool.d/provctl-<subscription>.conf`:

```ini
[{{ .Name }}]
user = {{ .Name }}
group = {{ .Name }}

listen = /run/php/provctl-{{ .Name }}.sock
listen.owner = www-data
listen.group = www-data
listen.mode = 0660

pm = dynamic
pm.max_children = {{ .MaxChildren }}
pm.start_servers = {{ .StartServers }}
pm.min_spare_servers = {{ .MinSpare }}
pm.max_spare_servers = {{ .MaxSpare }}
pm.max_requests = 500

php_admin_value[memory_limit] = {{ .MemoryLimit }}
php_admin_value[upload_max_filesize] = {{ .UploadMax }}
php_admin_value[post_max_size] = {{ .UploadMax }}
php_admin_value[max_execution_time] = {{ .MaxExecTime }}

; Izolace mezi subscriptions
php_admin_value[open_basedir] = {{ .Home }}:/usr/share/php:/tmp
php_admin_value[upload_tmp_dir] = {{ .Home }}/tmp
php_admin_value[sys_temp_dir] = {{ .Home }}/tmp
php_admin_value[session.save_path] = {{ .Home }}/tmp/sessions

php_admin_value[error_log] = {{ .PhpErrorLog }}
php_admin_flag[log_errors] = on
php_admin_flag[display_errors] = off
```

**[MUST]** `session.save_path` per subscription je povinné. Výchozí sdílená cesta znamená, že si subscriptions vidí navzájem do session souborů.

**[MUST]** `open_basedir` je povinné. Bez něj PHP jedné subscription přečte cizí soubory tam, kde to práva dovolí.

### 9.2 Atomická změna FPM

**[MUST]** Analogicky k Apache:

```
1. zápis pool souboru
2. php-fpm<ver> -t          (test konfigurace)
3. FAIL → rollback souboru, znovu test, vrátit chybu
4. OK   → systemctl reload php<ver>-fpm
5. ověřit is-active + existenci socketu
```

**[MUST]** Krok 2 v původním návrhu chyběl — bez něj vadný pool projde až do reloadu.

### 9.3 Změna PHP verze

**[MUST]** Změna verze subscription je **atomická operace zahrnující tři systémy**:

```
1. vygenerovat pool v novém /etc/php/<new>/fpm/pool.d/
2. php-fpm<new> -t
3. reload php<new>-fpm, ověřit socket
4. přegenerovat VŠECHNY vhosty subscription (mění se cesta socketu)
5. apachectl configtest + reload apache2
6. odstranit pool ze starého /etc/php/<old>/fpm/pool.d/
7. reload php<old>-fpm
```

Undo v opačném pořadí. Socket se jmenuje `provctl-<sub>.sock` (bez verze), aby se cesta neměnila — **ale [MUST] i tak se vhosty přegenerují**, protože jinak by konfigurace přestala odpovídat DB a `reconcile` by hlásil drift.

---

## 10. MariaDB

### 10.1 Připojení

**[MUST]** provctl se nikdy nepřipojuje s heslem na příkazové řádce (viditelné v `ps`). Používá:

- buď `unix_socket` autentizaci jako root (výchozí na Debianu, **PŘEDPOKLAD** — `doctor` to ověří),
- nebo defaults-file `/etc/provctl/mysql.cnf` s právy `0600`, jehož cesta je v configu.

**[MUST]** SQL se předává přes stdin (`RunWithStdin`), nikdy jako argument.

### 10.2 Operace

```sql
CREATE DATABASE `acme_main` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'acme_main'@'localhost' IDENTIFIED BY '<generated>';
GRANT ALL PRIVILEGES ON `acme_main`.* TO 'acme_main'@'localhost';
FLUSH PRIVILEGES;
```

**[MUST]** Identifikátory se nikdy nesestavují interpolací nevalidovaného vstupu. Jméno DB i uživatele projde whitelistem `^[a-z][a-z0-9_]{0,X}$` **před** sestavením SQL. Backtick escaping je druhá obrana, ne první.

**[MUST]** **PŘEDPOKLAD k ověření implementací:** maximální délka jména MySQL/MariaDB uživatele je omezená (v MySQL 32 znaků, v novějších MariaDB více). Implementace to **ověří dotazem na cílový server** (`SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='mysql' AND TABLE_NAME='global_priv' AND COLUMN_NAME='User'`) a validuje délku *před* vytvořením. Nesmí spoléhat na natvrdo zapsané číslo a nesmí nechat MariaDB jméno tiše useknout.

### 10.3 Hesla

**[MUST]** Heslo k databázi se generuje kryptograficky (`crypto/rand`, min. 24 znaků, znaková sada bez znaků problematických pro shell/URL), **zobrazí se právě jednou** a do SQLite provctl se neukládá ani hashované.

Volitelně (`--write-credentials <path>`) se zapíše do souboru vlastněného subscription s právy `0600`.

---

## 11. SSL / Certbot — stavový automat

**Problém v původním návrhu:** SSL vhost nelze vygenerovat dřív, než existuje certifikát — Apache neprojde configtestem a operace spadne.

**[MUST]** Závazné pořadí `provctl ssl enable <sub> <domain>`:

```
1. ověřit, že web existuje a je enabled
2. ověřit DNS: A/AAAA záznam domény i všech aliasů ukazuje na IP serveru
   (selhání = varování + vyžádání --force, ne tvrdá chyba — server může být za NAT)
3. zajistit HTTP vhost bez force_https a s aliasem na acme-challenge, reload
4. self-check: HTTP GET http://<domain>/.well-known/acme-challenge/<random>
   → očekává se 404 z Apache (ne 301, ne connection refused)
5. certbot certonly --webroot -w /var/lib/provctl/acme-challenge \
       -d <domain> [-d <alias> ...] \
       --non-interactive --agree-tos -m <email> \
       --cert-name provctl-<sub>-<domain>
6. ověřit existenci /etc/letsencrypt/live/provctl-<sub>-<domain>/fullchain.pem
7. zapsat certifikát do DB (lineage, SANs, not_after)
8. vygenerovat SSL vhost + (volitelně) force_https v HTTP vhostu
9. configtest → reload
```

**[MUST]** `--webroot` s **centrálním** challenge adresářem, ne apache plugin. Důvod: apache plugin by editoval Apache konfiguraci a kolidoval s generovanými soubory.

**[MUST]** `provctl ssl disable` odstraní SSL vhost a `force_https`, ale **certifikát nemaže** (aby se nespotřebovával rate limit Let's Encrypt při opakovaném zapínání).

### 11.1 Proč auto-renew funguje bez zásahu provctl

**FAKT:** certbot si při vydání certifikátu uloží použité parametry do `/etc/letsencrypt/renewal/<lineage>.conf` — včetně `authenticator = webroot` a `webroot_path`. `certbot renew` je pak při obnově použije znovu, bez opakování původních přepínačů.

Z toho plyne, že pokud provctl vydá certifikát s `--webroot -w /var/lib/provctl/acme-challenge`, obnova bude sahat na stejný centrální adresář, který je v každém HTTP vhostu vystavený přes `Alias` a je průchozí i při `force_https` (§8.3). **Auto-renew tedy funguje díky návrhu vhostu, ne díky žádné vlastní logice provctl.**

**[MUST]** provctl **neimplementuje vlastní renew timer ani cron**. Duplicitní mechanismus obnovy vede k zbytečným pokusům a v horším případě k vyčerpání rate limitu Let's Encrypt.

### 11.2 Existující obnova na serveru — povinná kontrola

**PŘEDPOKLAD:** balíček `certbot` v Debianu dodává vlastní automatickou obnovu (systemd timer `certbot.timer`, u starších instalací i `/etc/cron.d/certbot`). Autor navíc uvádí, že si obnovu **momentálně řeší vlastním cronem**.

**[MUST]** `provctl doctor` proto detekuje všechny tři možné mechanismy a chová se takto:

| Zjištěný stav | Výsledek |
|---|---|
| aktivní právě jeden mechanismus | `OK` |
| žádný mechanismus | `FAIL` + návod, jak zapnout `certbot.timer` |
| dva a více mechanismů | `WARN` + výpis konkrétních souborů a doporučení ponechat jen jeden |

**[MUST]** provctl **sám cizí cron nezruší.** Je to zásah do konfigurace serveru, který si má admin udělat vědomě. Doctor pouze pojmenuje konflikt a napíše přesně, který soubor odstranit.

### 11.3 Deploy hook — globální, ne per-certifikát

**[MUST]** provctl instaluje **jeden globální hook** (dělá to `provctl bootstrap`, ne `postinst`):

`/etc/letsencrypt/renewal-hooks/deploy/00-provctl.sh`

```sh
#!/bin/sh
# GENERATED BY PROVCTL — DO NOT EDIT
# certbot předává: RENEWED_LINEAGE, RENEWED_DOMAINS
/usr/bin/provctl ssl deploy-hook --lineage "$RENEWED_LINEAGE" >> /var/log/provctl/deploy-hook.log 2>&1
exit 0
```

**[MUST]** Zásady:

- Hook **vždy vrací 0**. Chyba v provctl nesmí shodit obnovu certifikátu.
- Hook je **globální**, ne per-certifikát přes `--deploy-hook`. Kdyby bylo obojí, certbot by spustil obě varianty a apache by se reloadoval dvakrát. Globální hook navíc pokryje i certifikáty vydané mimo provctl (např. ty stávající), což je žádoucí — reload po obnově je potřeba tak jako tak.
- `provctl ssl deploy-hook` je **idempotentní a rychlý**: přečte expiraci z `$RENEWED_LINEAGE/cert.pem`, aktualizuje `certificates.not_after` v DB (pokud lineage zná) a udělá `systemctl reload apache2`. Nic negeneruje, nic nemigruje.

**[MUST]** `provctl ssl status` čte skutečný stav z `/etc/letsencrypt/live/*` (`openssl x509 -noout -enddate`), ne z DB. DB je jen cache — pokud se obnova provede bez hooku, status je i tak správný.

### 11.4 Ověření obnovy — povinný akceptační krok

**[MUST]** Po `provctl ssl enable` se obnova neověřuje domněnkou, ale příkazem:

```
certbot renew --cert-name provctl-<sub>-<domain> --dry-run
```

**[MUST]** `provctl ssl enable` tento dry-run spustí automaticky jako poslední krok (lze vypnout `--no-renewal-check`) a jeho selhání hlásí jako `WARN` s plným výstupem certbota — certifikát už v tu chvíli existuje, takže to není důvod k rollbacku, ale je to důvod k upozornění.

**[MUST]** Totéž je součástí `provctl health` pro každý web s SSL, ale **jen na vyžádání** (`--check-renewal`), protože dry-run chodí na síť do Let's Encrypt.

### 11.5 Převzetí existujících certifikátů (`adopt`)

**Past:** stávající certifikáty na serveru mají v renewal configu vlastní `authenticator` (často apache plugin) a vlastní `webroot_path` ukazující na původní `/var/www/<domain>`. Po `provctl subscription adopt`, který data přesune, by **obnova tiše přestala fungovat** a projevilo by se to až za měsíce expirací.

**[MUST]** `subscription adopt` proto:

1. Najde v `/etc/letsencrypt/renewal/` všechny lineage, jejichž `domains` obsahují přesouvanou doménu.
2. Nahlásí je uživateli **před** přesunem.
3. Po přesunu přenastaví lineage na centrální webroot:
   `certbot certonly --webroot -w /var/lib/provctl/acme-challenge -d <domény> --cert-name <existující lineage> --keep-until-expiring --non-interactive`
4. **Ověří** výsledek přes `certbot renew --cert-name <lineage> --dry-run`.
5. Teprve po úspěšném dry-runu označí adopci za dokončenou. Při selhání dry-runu vypíše hlasité varování a nechá záznam v `operations` se stavem `inconsistent`.

**PŘEDPOKLAD k ověření implementací:** že opakované `certonly --keep-until-expiring` s existujícím `--cert-name` skutečně přepíše `webroot_path` v renewal configu, aniž by vydalo nový certifikát. Ověření je právě krok 4 — pokud dry-run projde, přenastavení fungovalo. Pokud ne, implementace použije fallback: editovat `webroot_path` v renewal configu přímo (se zálohou původního souboru) a dry-run zopakovat.

---

## 12. Logy a logrotate

`/etc/logrotate.d/provctl-<subscription>`:

```
{{ .LogDir }}/*/*.log {
    daily
    rotate {{ .Retention }}
    missingok
    notifempty
    compress
    delaycompress
    create 0640 root {{ .Name }}
    sharedscripts
    postrotate
        systemctl reload apache2 > /dev/null 2>&1 || true
    endscript
}
```

**[MUST]** `create 0640 root <sub>` je povinné — je to jediný mechanismus, kterým se uživateli otevře čtení logů, aniž by dostal zápis do adresáře (viz §6.3).

---

## 13. SSH a hesla

**[MUST]** Výchozí stav nové subscription: `ssh_access = none` (shell `/usr/sbin/nologin`, heslo zamčené `usermod -L`).

| Režim | Shell | Heslo | `authorized_keys` |
|---|---|---|---|
| `none` | `/usr/sbin/nologin` | locked | prázdný |
| `key` | `/bin/bash` | locked | spravovaný |
| `password` | `/bin/bash` | nastavené | prázdný |
| `key+password` | `/bin/bash` | nastavené | spravovaný |

**[MUST]** `~/.ssh` `0700`, `~/.ssh/authorized_keys` `0600`, vlastník subscription. Soubor je generovaný artefakt z tabulky `ssh_keys` — provctl ho přepisuje celý, s hlavičkou „DO NOT EDIT".

**[MUST]** Klíč se před uložením validuje (`ssh-keygen -l -f -` přes stdin) a ukládá se otisk. Nevalidní klíč = chyba.

**[MUST]** Hesla: `crypto/rand`, min. 20 znaků, nastavení přes `chpasswd` **na stdin**. Heslo se zobrazí jednou a nikam se neukládá.

**[MUST]** provctl v v0.1 needituje `/etc/ssh/sshd_config`. Pokud je potřeba chroot/`Match User` blok, je to **[LATER]** a musí to být samostatný generovaný soubor v `/etc/ssh/sshd_config.d/provctl.conf` s vlastním `sshd -t` testem před reloadem.

---

## 14. Cron

**[MUST]** Crontab uživatele je **generovaný artefakt** z tabulky `cron_jobs`, zapisovaný přes `crontab -u <user> -` (stdin). Nikdy přímý zápis do `/var/spool/cron/crontabs`.

**[MUST]** Validace `schedule` proti cron syntaxi před zápisem. `command` se **nevaliduje obsahově** (uživatel má právo spustit cokoli jako svůj vlastní uživatel), ale nesmí obsahovat znak nového řádku.

**[MUST]** Suspend subscription **musí odstranit crontab** (v DB zůstává, jen se negeneruje). V původním návrhu to chybělo — jinak by suspendovaná subscription dál běžela přes cron.

---

## 15. Stavy subscription

```
          create
             │
             ▼
       ┌──────────┐  suspend   ┌───────────┐
       │  ACTIVE  │───────────▶│ SUSPENDED │
       │          │◀───────────│           │
       └────┬─────┘  resume    └─────┬─────┘
            │                        │
            │ archive                │ archive
            ▼                        ▼
       ┌──────────┐
       │ ARCHIVED │──── delete ───▶ (pryč)
       └──────────┘
```

| Akce | Apache | PHP-FPM | Cron | SSH | DB | Data |
|---|---|---|---|---|---|---|
| **suspend** | vhosty disabled | pool odstraněn | crontab odstraněn | shell → nologin, heslo locked | zůstává | zůstávají |
| **resume** | obnoveno z DB | obnoveno | obnoveno | obnoveno dle `ssh_access` | — | — |
| **archive** | disabled | odstraněn | odstraněn | disabled | zůstává | zůstávají + povinná záloha |
| **delete** | odstraněno | odstraněno | odstraněno | uživatel smazán | **smazána** | **smazána** |

**[MUST]** `archive` **vždy** nejprve vytvoří zálohu. Když záloha selže, archivace se neprovede.

**[MUST]** `delete` vyžaduje:

1. subscription musí být ve stavu `ARCHIVED` (nelze mazat aktivní), pokud není `--force`,
2. interaktivní potvrzení opsáním jména subscription,
3. druhé potvrzení `--yes-i-am-sure` nebo interaktivní `yes`,
4. v neinteraktivním režimu je nutné `--yes-i-am-sure` **i** `--confirm-name=<name>`.

**[MUST]** `delete` nikdy nesmí mazat rekurzivně cestu, kterou si sám nesestavil z DB a neověřil, že leží uvnitř vhosts rootu a odpovídá existujícímu uživateli. Před `rm -rf` ekvivalentem se ověří: cesta ≠ `/`, cesta ≠ vhosts root, cesta má prefix vhosts rootu, cesta po `EvalSymlinks` má stále ten prefix.

---

## 16. Backup / Restore

### 16.1 Formát

```
/var/backups/provctl/acme/2026-08-28T03-00-00Z/
├── metadata.json
├── files.tar.zst
├── db/
│   ├── acme_main.sql.zst
│   └── acme_shop.sql.zst
└── SHA256SUMS
```

`metadata.json`:

```json
{
  "format_version": 1,
  "provctl_version": "0.1.0",
  "created_at": "2026-08-28T03:00:00Z",
  "subscription": { "name": "acme", "unix_user": "acme", "uid": 1005, "home": "/var/www/vhosts/acme",
                    "php_version": "8.4", "status": "active", "quotas": { } },
  "websites": [ ],
  "databases": [ { "name": "acme_main", "db_user": "acme_main", "db_host": "localhost" } ],
  "cron_jobs": [ ],
  "ssh_keys": [ ],
  "certificates": [ { "lineage": "provctl-acme-example.cz", "sans": ["example.cz","www.example.cz"] } ]
}
```

### 16.2 Pravidla

**[MUST]**

- Před zálohou zkontrolovat volné místo: odhad = `du -sb` home + součet velikostí DB × 1.2. Při nedostatku selhat **předem**, ne uprostřed.
- `mysqldump --single-transaction --quick --routines --triggers --events` (konzistentní dump InnoDB bez zamykání).
- `tar --numeric-owner --acls --xattrs -p` — zachovat vlastníky číselně (UID se při restore může lišit, řeší se remapem, viz níže).
- Vyloučit z `files.tar.zst`: `tmp/`, `*/storage/framework/cache/*`, socket a FIFO soubory.
- Po dokončení spočítat `SHA256SUMS` a zapsat řádek do `backups` se `status='complete'`. Nedokončená záloha zůstane `running`/`failed` a `backup list` ji označí.
- Záloha běží **bez zámku na celý provctl** (může trvat hodiny), ale drží *per-subscription* zámek, aby nekolidovala s jinou operací nad stejnou subscription. → **[MUST]** zámky jsou tedy dva: globální (`/run/provctl.lock`) pro systémové změny a per-subscription (`/run/provctl-<sub>.lock`).

### 16.3 Restore

**[MUST]** Řešení rozporu z původního zadání: hesla k databázím se neukládají, takže restore je **nemůže obnovit**. Chování:

- Restore vytvoří databáze a uživatele s **nově vygenerovanými hesly** a vypíše je.
- `metadata.json` obsahuje jména DB a uživatelů, **nikdy hesla**.
- Restore na konci vypíše výrazné upozornění: *„Hesla k databázím byla vygenerována znovu. Aktualizuj konfigurační soubory aplikací."*

**[MUST]** Postup restore:

```
1. backup inspect  — ověřit SHA256SUMS a format_version
2. rozhodnout režim:
   - subscription neexistuje → plný restore
   - existuje → vyžadovat --force, a i pak NEPŘEPISOVAT bez zálohy současného stavu
3. vytvořit uživatele (UID se nevynucuje; při rozdílu se provede rekurzivní chown na nové UID)
4. rozbalit files.tar.zst do dočasného adresáře na STEJNÉM filesystému
5. ověřit strukturu, chown, chmod dle §6.2
6. přesunout na cílové místo (rename, atomicky)
7. obnovit DB, vygenerovat hesla
8. obnovit záznamy do SQLite
9. reconcile (vygenerovat Apache/FPM/cron)
10. certifikáty NEobnovovat — vydat znovu přes `provctl ssl enable`
```

---

## 17. Health checks

**[MUST]** `provctl health [<subscription> [<domain>]]`, výstup i v `--json`.

| Check | Metoda |
|---|---|
| Apache running | `systemctl is-active apache2` |
| Apache config | `apachectl configtest` |
| FPM pool | `systemctl is-active php<ver>-fpm` + existence socketu + práva socketu |
| Vhost enabled | existence symlinku |
| DocumentRoot | existuje, je adresář, práva dle §6.2 |
| DNS | A/AAAA doména vs. IP serveru (`net.LookupHost`) |
| HTTP | GET `http://<domain>/` s `Host` hlavičkou, timeout 5 s, status |
| HTTPS | totéž přes TLS + verifikace řetězce |
| Certifikát | dny do expirace (`WARN` < 21 dní, `FAIL` < 7 dní nebo expirovaný) |
| Databáze | připojení + `SELECT 1` |
| Disk | `du -sb` home vs. `quota_disk_bytes` (`WARN` > 90 %) |

**[MUST]** Health check je **read-only**. Nikdy nic neopravuje.

---

## 18. Audit log

**[MUST]** `/var/log/provctl/audit.jsonl`, JSON Lines, jeden řádek na operaci:

```json
{"ts":"2026-08-28T10:15:00Z","actor":"petr","uid":0,"action":"subscription.create","target":"acme","status":"ok","duration_ms":842,"operation_id":42}
{"ts":"2026-08-28T10:17:11Z","actor":"petr","uid":0,"action":"ssl.enable","target":"acme/example.cz","status":"failed","error":"dns validation failed","operation_id":44}
```

**[MUST]**

- `actor` = `SUDO_USER`, fallback `USER`, fallback `root`.
- Logují se i **neúspěšné** operace.
- Do audit logu **nikdy** nesmí jít heslo, privátní klíč ani obsah SQL. Argumenty se logují po filtraci přes seznam citlivých parametrů.
- Vlastní logrotate konfigurace pro audit log (`/etc/logrotate.d/provctl`, `rotate 90`, `compress`, `create 0640 root adm`).

---

## 19. Kvóty

**[MUST]** V v0.1 jsou kvóty **měřené a kontrolované na úrovni provctl**, nikoli vynucené kernelem:

- `quota_websites`, `quota_databases`, `quota_backups` — **vynucené** (kontrola při create, odmítnutí při překročení).
- `quota_disk_bytes` — **pouze měřená** (`du -sb`, cache výsledku v paměti po dobu běhu příkazu, protože je to drahé). Health check hlásí překročení, ale nic neblokuje.

**[LATER]** Skutečné disk quoty (`quota`/`repquota`, XFS project quotas) — vyžadují podporu na filesystému a jsou mimo rozsah v0.1. Datový model už s tím počítá.

---

## 20. Validace vstupů

**[MUST]** Centralizováno v `internal/domain/validate.go`, s tabulkovými testy.

| Vstup | Pravidlo |
|---|---|
| Jméno subscription | `^[a-z][a-z0-9-]{1,30}$`, max 31 znaků (limit Linux uživatele), nesmí kolidovat s existujícím uživatelem, nesmí být v rezervovaném seznamu (`root`, `www-data`, `mysql`, `admin`, `daemon`, `backup`, `provctl`, …), UID přidělen automaticky z rozsahu v configu |
| Doména | převod na lowercase + punycode (`golang.org/x/net/idna`), max 253 znaků, každý label max 63, validace přes `idna.Lookup.ToASCII`, zákaz wildcardů v v0.1 |
| DocumentRoot | absolutní cesta, po `filepath.Clean` **a** `EvalSymlinks` musí mít prefix `<home>/`, nesmí obsahovat `..`, cílový adresář musí existovat a být adresář |
| Proxy target | `http://` + povolený host (default `127.0.0.1`, `localhost`, `::1`) + port 1024–65535 |
| Redirect target | absolutní URL, schéma `http`/`https` |
| Jméno DB | `^[a-z][a-z0-9_]{1,X}$`, prefix `<sub>_` vynucen, délka ověřená proti serveru (§10.2) |
| Cron schedule | validace 5 polí, standardní syntaxe, povoleny `@daily` apod. |
| SSH klíč | `ssh-keygen -l -f -` |
| PHP verze | musí být v detekovaném seznamu |

**[MUST]** Kontrola DocumentRootu proti traversalu se dělá **po** `EvalSymlinks` obou cest (home i cíle). Kontrola pouhého string prefixu je nedostatečná — uživatel může vytvořit symlink.

---

## 21. CLI

### 21.1 Globální konvence

**[MUST]**

- Globální přepínače: `--config`, `--json`, `--dry-run`, `--yes`, `--verbose`, `--quiet`, `--no-color`.
- `--json` na *každém* příkazu (i chybovém výstupu) — nutné pro skriptování a pro integrační testy.
- Chybový výstup jde na stderr, data na stdout. V `--json` režimu i chyby jako JSON na stderr.

Exit kódy:

| Kód | Význam |
|---|---|
| 0 | úspěch |
| 1 | obecná chyba |
| 2 | chyba validace vstupu / drift nalezen (`reconcile --dry-run`) |
| 3 | objekt nenalezen |
| 4 | konflikt (už existuje, kvóta překročena) |
| 5 | selhání systémové operace (Apache, FPM, DB) |
| 6 | proveden rollback (změna se nekonala) |
| 7 | **nekonzistentní stav** — rollback selhal, vyžaduje ruční zásah |
| 10 | není root / chybí zámek |

### 21.2 Příkazy

```
provctl doctor [--json]
provctl bootstrap [--install-missing] [--yes] [--dry-run] [--skip <check>]
provctl config migrate [--dry-run]
provctl migrate                                # pouze DB schéma, volá postinst
provctl reconcile [--dry-run] [--subscription <name>]
provctl operation list|inspect <id>

provctl subscription create <name> [--php <ver>] [--ssh key|password|none] [--quota-disk 20G]
provctl subscription list|show|suspend|resume|archive <name>
provctl subscription delete <name> --confirm-name <name> --yes-i-am-sure
provctl subscription adopt <name> --from /var/www/example.cz --domain example.cz   # migrace

provctl website create <sub> <domain> --type php-fpm|static|proxy|redirect
                                      [--docroot <path>] [--target <url>] [--alias <domain>]
provctl website list <sub>
provctl website show|enable|disable|delete <sub> <domain>
provctl website alias add|remove <sub> <domain> <alias>
provctl website logs <sub> <domain> [--error] [--follow] [--lines N]

provctl php list-versions
provctl php set <sub> --version <ver> [--max-children N] [--memory-limit 256M]

provctl database create <sub> <name> [--write-credentials <path>]
provctl database list <sub>
provctl database password <sub> <name>        # regeneruje heslo
provctl database delete <sub> <name> --yes

provctl ssl enable|renew|disable|status <sub> <domain>
provctl ssl deploy-hook                        # volá certbot

provctl ssh set <sub> --access key|password|none
provctl ssh key add <sub> --file <path> | --stdin
provctl ssh key list|remove <sub> [<fingerprint>]

provctl cron list|add|remove <sub> ...
provctl backup create|list|inspect <sub> [...]
provctl backup restore <sub> <backup-id> [--force]
provctl health [<sub> [<domain>]]
provctl apache reload|restart|configtest

provctl                                        # bez argumentů → TUI
```

**[MUST]** `subscription adopt` je povinný — bez něj nelze splnit deklarovaný cíl postupné migrace stávajícího `/var/www/<domain>`. Chování: ověří, že cesta existuje, vytvoří subscription a uživatele, **přesune** (ne zkopíruje, ale s `--copy` volitelně) data do `<vhosts>/<name>/sites/<domain>/public`, nastaví práva, vytvoří website záznam, přegeneruje konfiguraci. Před přesunem vytvoří zálohu, pokud `--backup` (default zapnuto).

### 21.3 Čistý server: `doctor` hlásí, `bootstrap` opravuje

**[MUST]** Rozdělení odpovědnosti je striktní:

| | `doctor` | `bootstrap` |
|---|---|---|
| Mění systém | **ne, nikdy** | ano |
| Vyžaduje potvrzení | ne | ano (mimo `--yes`) |
| Idempotentní | triviálně | **musí být** |
| Exit kód | 1 při jakémkoli `FAIL` | 0 / 5 |

**[MUST]** `provctl bootstrap` provede — v tomto pořadí, jako plán s rollbackem (§7):

1. Vytvoří adresáře `/etc/provctl`, `/var/lib/provctl`, `/var/lib/provctl/acme-challenge`, `/var/log/provctl`, vhosts root — se správnými právy dle §6.1.
2. Povolí chybějící Apache moduly: `proxy`, `proxy_fcgi`, `proxy_http`, `ssl`, `rewrite`, `headers` (symlinky v `mods-enabled`, stejně jako u vhostů — bez `a2enmod`).
3. Vytvoří catch-all vhost `provctl-000-default.conf` + self-signed certifikát (§8.7).
4. Nainstaluje deploy hook pro certbot (§11.3).
5. Nainstaluje logrotate konfiguraci pro audit log.
6. `apachectl configtest` → `reload apache2`.
7. Znovu spustí `doctor` a vypíše výsledný stav.

**[MUST]** `bootstrap` je opakovaně spustitelný. Druhý běh na už připraveném serveru **nesmí nic změnit** a musí to nahlásit (`nothing to do`). Toto je povinný test.

### 21.4 Doinstalace chybějících balíčků

**[MUST]** `bootstrap --install-missing` doinstaluje chybějící závislosti, ale za těchto podmínek:

1. **Pouze balíčky z oficiálních repozitářů Debianu.** Seznam je pevný a v kódu: `apache2`, `certbot`, `logrotate`, `mariadb-server` / `mariadb-client`, `php-fpm` (metabalíček verze z Debianu).
2. **Nikdy nepřidává repozitáře třetích stran.** Pokud je potřeba PHP verze, kterou Debian nemá (Sury), provctl **pouze vypíše přesné příkazy** k ručnímu přidání a skončí. Přidání cizího APT klíče je bezpečnostní rozhodnutí admina, ne nástroje.
3. Před instalací vypíše **přesný seznam** balíčků a vyžádá potvrzení (mimo `--yes`).
4. Ověří, že není držen dpkg lock (`/var/lib/dpkg/lock-frontend`). Pokud je, **selže s jasnou chybou** — nečeká, neopakuje.
5. Spouští `apt-get install -y --no-install-recommends <pkgs>` s `DEBIAN_FRONTEND=noninteractive`.
6. **Nikdy se nespouští z `postinst`** — vnořený `apt` uvnitř probíhající dpkg transakce skončí deadlockem na zámku.

**[MUST]** Bez `--install-missing` bootstrap chybějící balíčky **neinstaluje**, jen selže s výpisem toho, co chybí, a s hotovým `apt install` příkazem k ručnímu spuštění. Instalace balíčků musí být vždy vědomá volba.

Typický průběh na čistém serveru:

```bash
sudo apt install provctl
sudo provctl doctor                      # co chybí
sudo provctl bootstrap --install-missing # doinstaluj a připrav
sudo provctl doctor                      # ověření, vše OK
sudo provctl subscription create acme
```

---

## 22. TUI

**[MUST]** TUI je pouze prezentační vrstva nad `service`. Žádný TUI kód nesmí volat `system` ani `repository`.

**[MUST]** Rozsah v0.1 — záměrně malý:

- seznam subscriptions (stav, počty webů/DB, disk)
- detail subscription (weby, DB, PHP, SSL, disk)
- detail webu (typ, docroot, SSL, health)
- prohlížeč logů
- spouštění health checku
- **read-mostly**: jediné mutující akce v TUI v0.1 jsou `enable`/`disable` webu a `suspend`/`resume`. Vytváření a mazání jde přes CLI.

Odůvodnění: mutující operace v TUI vyžadují stejné potvrzovací a chybové toky jako CLI; udělat je pořádně stojí víc času než celý zbytek TUI. **[LATER]**

**[MUST]** Každá dlouhá operace v TUI běží mimo hlavní smyčku Bubble Tea (`tea.Cmd`), s viditelným průběhem a možností zrušení.

---

## 23. Konfigurační soubor

`/etc/provctl/config.toml`:

```toml
# Verze schématu konfigurace. NEUPRAVOVAT ručně — mění ji `provctl config migrate`.
[meta]
config_version = 1

[paths]
vhosts          = "/var/www/vhosts"
backups         = "/var/backups/provctl"
acme_challenge  = "/var/lib/provctl/acme-challenge"

[apache]
service         = "apache2"
sites_available = "/etc/apache2/sites-available"
sites_enabled   = "/etc/apache2/sites-enabled"
proxy_timeout   = 60
allowed_proxy_hosts = ["127.0.0.1", "localhost", "::1"]

[php]
default_version = ""        # prázdné = nejvyšší detekovaná
max_children    = 10
memory_limit    = "256M"
upload_max      = "64M"
max_exec_time   = 60

[mariadb]
enabled       = true
host          = "localhost"
defaults_file = ""          # prázdné = unix_socket auth jako root

[users]
uid_min = 5000
uid_max = 59999
shell   = "/bin/bash"

[ssl]
email   = ""                # povinné, pokud se má používat certbot
staging = false             # true = Let's Encrypt staging (pro testy!)

[logs]
retention_days = 14
compress       = true

[limits]
lock_timeout_seconds = 30
```

**[MUST]** Config se validuje při startu. Chybějící povinná hodnota (např. `ssl.email` při použití SSL příkazu) = jasná chyba s cestou k souboru a názvem klíče.

**[MUST]** Žádná cesta se v kódu nesmí objevit natvrdo — vždy z configu nebo z `meta`.

### 23.1 Verzování a migrace při upgradu nástroje

V systému existují **čtyři nezávisle verzované artefakty**. Každý musí mít verzi zapsanou přímo v sobě, jinak nelze při upgradu bezpečně rozhodnout, co migrovat.

| Artefakt | Kde je verze | Migrace |
|---|---|---|
| SQLite databáze | tabulka `schema_migrations` | ano, číslované SQL migrace |
| `config.toml` | `[meta] config_version` | ano, viz níže |
| Zálohy | `metadata.json` → `format_version` | ano, při restore |
| Generované konfigurace (Apache, FPM, cron, logrotate) | hlavička souboru | **ne — přegenerují se** |

**[MUST]** Generované konfigurace se **nikdy nemigrují**. Jsou to odvozená data; při změně šablon je správná odpověď `provctl reconcile`, ne migrace. Proto hlavička obsahuje `provctl-version` a `template-hash` — reconcile podle nich pozná, co je zastaralé.

**[MUST] Migrace konfiguračního souboru.** `config.toml` je conffile s `noreplace`, takže ho `apt upgrade` **nikdy** nepřepíše — nová verze nástroje tedy najde starý soubor. Chování:

1. Při startu se načte `config_version`. Chybějící hodnota se interpretuje jako `1`.
2. `config_version` > verze podporovaná binárkou ⇒ **odmítnout běh** s chybou (starší nástroj nesmí pracovat s novějším configem).
3. `config_version` < aktuální ⇒ migrace se aplikuje **v paměti** (doplní se výchozí hodnoty nových klíčů) a vypíše se jednorázové upozornění:
   `config.toml je verze 1, aktuální je 2. Spusť 'provctl config migrate'.`
4. `provctl config migrate` zapíše aktualizovaný soubor, **zachová komentáře a existující hodnoty**, doplní nové klíče s výchozími hodnotami a komentářem, a před zápisem uloží `config.toml.bak-<timestamp>`.
5. Neznámé klíče = `WARN`, ne chyba (kompatibilita se starším i novějším configem).
6. Odstraněné klíče = `WARN` s informací, čím byly nahrazeny.

**[MUST]** Migrace configu se **nikdy** nespouští z `postinst`. Volá ji admin, nebo se aplikuje v paměti při prvním běhu.

**[MUST]** Do DB se při každém spuštění zapisuje poslední použitá verze nástroje (tabulka `meta_kv`, klíč `last_run_version`) — slouží k diagnostice, když se něco rozbije po upgradu, a k rozhodnutí, zda po upgradu doporučit `reconcile`.

**[MUST]** `provctl migrate` (volané z `postinst`) migruje **pouze databázové schéma**, je idempotentní, nesahá na systémové služby a při selhání nesmí shodit instalaci balíčku — jen vypíše instrukci ke spuštění ručně.

---

## 24. Testovací strategie

Toto je podmínka toho, aby šlo tvrdit „ověřeno". Bez ní je jakékoli tvrzení o funkčnosti nepodložené.

### 24.1 Úrovně

| Úroveň | Co pokrývá | Běží v CI |
|---|---|---|
| **Unit** | `domain` validace, výpočty cest, stavové přechody | ano |
| **Golden files** | všechny šablony (Apache, FPM, logrotate) proti fixturám | ano |
| **Service s fake systémem** | celé operace přes `system/fake` — včetně rollbacku | ano |
| **Repository** | SQLite operace nad dočasným souborem, migrace | ano |
| **Integrační** | skutečný Debian 13 kontejner/VM s Apache + PHP-FPM + MariaDB | **ne v základním CI** |

**[MUST]** Golden-file testy: každá šablona má pro každý typ webu fixture vstupu a očekávaný výstup v `testdata/`. Aktualizace přes `go test ./... -update`. Toto je nejlevnější reálná verifikace, kterou projekt má.

**[MUST]** Test rollbacku je povinný: fake `Commander` nakonfigurovaný tak, aby N-tý příkaz selhal, a test ověří, že po chybě neexistují žádné vytvořené soubory, uživatel ani DB záznam.

**[MUST]** Test „bez rootu": celá testovací sada musí projít pod běžným uživatelem. Pokud test vyžaduje root, patří do integrační sady označené build tagem `//go:build integration`.

### 24.2 Co CI ověřit nemůže

**[MUST]** Implementační agent v reportu **explicitně uvede**, že tyto věci nebyly ověřeny, pokud nebyly ověřeny na skutečném serveru:

- že vygenerovaná Apache konfigurace projde skutečným `apachectl configtest`,
- že FPM pool skutečně nastartuje,
- že certbot skutečně vydá certifikát,
- že práva na filesystému skutečně izolují subscriptions,
- chování při souběžném běhu více instancí.

**[MUST]** Pro SSL testy vždy `ssl.staging = true`, jinak dojde k vyčerpání rate limitu Let's Encrypt.

---

## 25. Distribuce: `.deb` balíček

### 25.1 Rozhodnutí

**[MUST]** V v0.1 **jeden** balíček `provctl`, ne trojice `provctl-core`/`provctl-cli`/`provctl-apache`.

Odůvodnění: rozdělení dává smysl až tehdy, když existuje víc než jeden frontend nebo víc backendů (např. nginx). Do té doby přidává jen složitost v CI a v závislostech. Rozdělit později lze bez breaking change (`provctl` se stane meta-balíčkem s `Depends`).

**[MUST]** Balíček se staví přes **nfpm** (`packaging/nfpm.yaml`), ne ručním `dpkg-deb`. Důvod: funguje v CI bez Debian build toolchainu a je deklarativní.

### 25.2 `packaging/nfpm.yaml`

```yaml
name: provctl
arch: ${ARCH}
platform: linux
version: ${VERSION}
section: admin
priority: optional
maintainer: "<jméno> <email>"
description: |
  Minimal hosting control-plane for Debian (Apache, PHP-FPM, MariaDB, SSL).
homepage: https://github.com/<org>/provctl
license: MIT

depends:
  - adduser
  - apache2
recommends:
  - certbot
  - logrotate
  - mariadb-client
suggests:
  - php-fpm

contents:
  - src: ./dist/provctl_${ARCH}
    dst: /usr/bin/provctl
    file_info: { mode: 0755 }

  - src: ./templates/
    dst: /usr/share/provctl/templates/

  - src: ./packaging/config.toml.default
    dst: /etc/provctl/config.toml
    type: config|noreplace          # NIKDY nepřepsat při upgradu

  - src: ./packaging/logrotate.provctl
    dst: /etc/logrotate.d/provctl
    type: config|noreplace

  - dst: /var/lib/provctl
    type: dir
    file_info: { mode: 0700 }
  - dst: /var/lib/provctl/acme-challenge
    type: dir
    file_info: { mode: 0755 }
  - dst: /var/log/provctl
    type: dir
    file_info: { mode: 0750 }

scripts:
  postinstall: ./packaging/debian/postinst
  preremove:   ./packaging/debian/prerm
  postremove:  ./packaging/debian/postrm
```

**[MUST]** `config.toml` je conffile s `noreplace` — upgrade nesmí přepsat konfiguraci serveru.
**[MUST]** Šablony v `/usr/share/provctl/templates` conffile **nejsou** — upgrade je má přepsat. Admin override patří do `/etc/provctl/templates/` (načítá se přednostně).

### 25.3 Maintainer skripty

**[MUST]** `postinst` dělá **jen tohle**:

```sh
#!/bin/sh
set -e
case "$1" in
  configure)
    # adresáře a práva (nfpm je vytvoří, tady jen pojistka a idempotence)
    install -d -m 0755 -o root -g root /etc/provctl
    install -d -m 0700 -o root -g root /var/lib/provctl
    install -d -m 0755 -o root -g root /var/lib/provctl/acme-challenge
    install -d -m 0750 -o root -g adm  /var/log/provctl

    # migrace schématu databáze (idempotentní, bez síťových a systémových operací)
    /usr/bin/provctl migrate --quiet || {
        echo "provctl: database migration failed, run 'provctl migrate' manually" >&2
    }

    echo "provctl installed. Run 'provctl doctor' to verify the environment,"
    echo "then 'provctl bootstrap' to set up Apache integration."
    ;;
esac
exit 0
```

**[MUST]** `postinst` **nikdy**:

- nezakládá subscription ani uživatele hostingu,
- nereloaduje Apache,
- nepovoluje Apache moduly,
- nevytváří vhosty,
- nemění nic v `/var/www`.

Toto je klíčové: `apt upgrade` musí být bezpečný na produkčním serveru se stovkami webů. Systémovou integraci dělá explicitní `provctl bootstrap`.

**[MUST]** `prerm` / `postrm`:

- `remove`: nic nemaže z `/var/www`, nemaže DB, nemaže vygenerované vhosty (server musí fungovat dál i bez nástroje).
- `purge`: smaže `/etc/provctl` a `/var/lib/provctl`, ale **nikdy** `/var/www/vhosts`, `/var/log/provctl` ani zákaznická data. Vypíše, co zůstalo.

**[MUST]** Verze balíčku: `1.2.0`, předběžné verze `1.5.0~rc1` — **tilda, ne pomlčka**. `~` řadí *před* finální verzi, `-rc1` by se řadilo *za* ni a server by z `testing` nikdy nepřešel na stable release.

---

## 26. APT repozitář na GitHub Pages

### 26.1 Podmínka zdarma — ověřeno

**FAKT (ověřeno srpen 2026):** GitHub Actions na standardních hostovaných runnerech je pro **veřejné repozitáře** zdarma a bez limitu minut. Privátní repozitáře mají na Free plánu 2 000 minut měsíčně.

**[MUST] Důsledek:** aby bylo řešení skutečně zdarma, musí být repozitář **veřejný**. Tím se veřejným stává i APT repozitář — což je pro tenhle typ nástroje v pořádku, ale je to vědomé rozhodnutí. Pokud by měl být privátní, GitHub Pages ani Actions zdarma nestačí a je nutná jiná varianta (Cloudflare R2).

### 26.2 Struktura

```
gh-pages (nebo Pages artifact)
└── debian/
    ├── dists/
    │   ├── stable/
    │   │   ├── InRelease            ← podepsaný inline
    │   │   ├── Release
    │   │   ├── Release.gpg
    │   │   └── main/
    │   │       ├── binary-amd64/{Packages,Packages.gz}
    │   │       └── binary-arm64/{Packages,Packages.gz}
    │   └── testing/  …
    ├── pool/main/p/provctl/*.deb
    └── provctl.asc                  ← veřejný GPG klíč
```

### 26.3 Klíčové rozhodnutí: repozitář se staví bez stavu

**[MUST]** Repozitář se **při každém releasu buduje znovu ze všech `.deb` v GitHub Releases**, ne inkrementálně z předchozího stavu.

Důvod: GitHub Pages deployment nahrazuje celý obsah stránky. Inkrementální `reprepro` databáze v `gh-pages` větvi je zdroj driftu a konfliktů. Stateless build má jediný zdroj pravdy (Releases) a je vždy reprodukovatelný.

### 26.4 GitHub Actions workflow

`.github/workflows/release.yml` — spouštěno na tag `v*`:

```yaml
name: release
on:
  push:
    tags: ['v*']

permissions:
  contents: write
  pages: write
  id-token: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: 'stable' }

      - name: Test
        run: |
          go vet ./...
          go test ./...

      - name: Build binaries
        env:
          CGO_ENABLED: 0
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          for arch in amd64 arm64; do
            GOOS=linux GOARCH=$arch go build \
              -trimpath \
              -ldflags "-s -w -X provctl/internal/meta.Version=${VERSION}" \
              -o dist/provctl_${arch} ./cmd/provctl
          done

      - name: Build .deb packages
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
          for arch in amd64 arm64; do
            ARCH=$arch VERSION=$VERSION nfpm package \
              -f packaging/nfpm.yaml -p deb -t dist/
          done

      - name: Publish GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: dist/*.deb
          prerelease: ${{ contains(github.ref_name, 'rc') }}

  publish-apt:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install tooling
        run: sudo apt-get update && sudo apt-get install -y reprepro

      - name: Import signing key
        env:
          GPG_PRIVATE_KEY: ${{ secrets.APT_GPG_PRIVATE_KEY }}
          GPG_PASSPHRASE:  ${{ secrets.APT_GPG_PASSPHRASE }}
        run: |
          export GNUPGHOME="$(mktemp -d)"
          echo "GNUPGHOME=$GNUPGHOME" >> $GITHUB_ENV
          echo "$GPG_PRIVATE_KEY" | gpg --batch --import

      - name: Download all release assets
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          mkdir -p incoming
          gh release list --limit 200 --json tagName -q '.[].tagName' \
            | while read -r tag; do
                gh release download "$tag" -D incoming -p '*.deb' --clobber || true
              done

      - name: Build repository
        run: |
          mkdir -p out/debian/conf
          cp packaging/apt/distributions out/debian/conf/distributions
          cp packaging/apt/provctl.asc out/debian/provctl.asc
          for deb in incoming/*.deb; do
            case "$deb" in
              *rc*) suite=testing ;;
              *)    suite=stable ;;
            esac
            reprepro -b out/debian includedeb "$suite" "$deb" || true
          done
          # RC verze patří i do testing, stable pouze finální

      - uses: actions/upload-pages-artifact@v3
        with: { path: out }
      - uses: actions/deploy-pages@v4
```

**[MUST]** `packaging/apt/distributions`:

```
Origin: provctl
Label: provctl
Codename: stable
Architectures: amd64 arm64
Components: main
Description: provctl stable
SignWith: <KEY_ID>

Origin: provctl
Label: provctl
Codename: testing
Architectures: amd64 arm64
Components: main
Description: provctl testing
SignWith: <KEY_ID>
```

**[MUST] Bezpečnost klíče:** privátní GPG klíč je **výhradně** v GitHub Secrets (`APT_GPG_PRIVATE_KEY`, `APT_GPG_PASSPHRASE`), nikdy v repozitáři, nikdy v logu workflow. Použij podklíč určený pouze k podpisu, ne hlavní klíč.

### 26.5 Instalace na serveru

Dokumentovaný postup (do README, **[MUST]** bez `curl | sh`):

```bash
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://<org>.github.io/provctl/debian/provctl.asc \
  | sudo gpg --dearmor -o /etc/apt/keyrings/provctl.gpg

echo "deb [signed-by=/etc/apt/keyrings/provctl.gpg] \
https://<org>.github.io/provctl/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/provctl.list

sudo apt update
sudo apt install provctl

sudo provctl doctor
sudo provctl bootstrap
```

### 26.6 Testování balíčku před publikací

**Zásada:** suite `testing` je určená pro *cizí* servery a pro ověření celého distribučního řetězce, ne pro první pokus, jestli se balíček vůbec nainstaluje. Než se cokoli publikuje, projde balíček třemi lokálními úrovněmi.

#### Úroveň 1 — statická kontrola balíčku (v CI, sekundy)

**[MUST]** Na každý push, ne jen na tag:

```bash
nfpm package -f packaging/nfpm.yaml -p deb -t dist/
lintian --no-tag-display-limit dist/provctl_*.deb
dpkg-deb --contents dist/provctl_*.deb      # kontrola cest a práv
dpkg-deb --info dist/provctl_*.deb          # kontrola control, závislostí, conffiles
```

**[MUST]** Workflow ukládá `.deb` jako artefakt buildu i pro netagované commity, aby šlo stáhnout a testovat bez vydání releasu.

#### Úroveň 2 — install / upgrade / purge v chrootu (v CI, minuty)

**[MUST]** `piuparts` je nástroj přesně na tohle — instaluje balíček v čistém chrootu, znovu ho odinstaluje a purguje, a hlásí každý soubor, který po sobě balíček nechal:

```bash
sudo piuparts -d trixie --warn-on-leftover-files dist/provctl_1.0.0_amd64.deb
```

**[MUST]** Test upgradu (potřebuje dvě verze):

```bash
sudo piuparts -d trixie --warn-on-leftover-files \
     dist/provctl_0.9.0_amd64.deb dist/provctl_1.0.0_amd64.deb
```

Toto je jediný spolehlivý způsob, jak ověřit, že upgrade **nepřepsal `/etc/provctl/config.toml`** a že `purge` nesmazal nic ze zákaznických dat.

**[MUST]** Akceptační kritérium: po `purge` nesmí zbýt nic v `/etc/provctl` a `/var/lib/provctl`, a naopak **musí** zůstat `/var/www/vhosts` a `/var/log/provctl`.

#### Úroveň 3 — reálný Debian 13 s běžícími službami (lokálně, ne v CI)

Chroot neumí systemd, Apache ani MariaDB. Skutečné ověření vyžaduje VM nebo systémový kontejner. Doporučené pořadí podle náročnosti:

1. **Systémový kontejner** — `incus`/`lxc` (**PŘEDPOKLAD:** `incus` je v Debianu 13 dostupný; pokud ne, platí varianta 2):

```bash
incus launch images:debian/13 provctl-test
incus file push dist/provctl_1.0.0_amd64.deb provctl-test/root/
incus exec provctl-test -- apt install -y /root/provctl_1.0.0_amd64.deb
incus exec provctl-test -- provctl doctor
incus exec provctl-test -- provctl bootstrap --install-missing --yes
incus exec provctl-test -- provctl subscription create acme
# po testu:
incus delete -f provctl-test
```

Výhoda: start pod pět sekund, plný systemd, snapshoty (`incus snapshot`) pro opakované testy ze stejného výchozího stavu.

2. **VM** (QEMU/libvirt nebo Vagrant s Debian 13 boxem) — nutné, pokud se testuje cokoli závislé na kernelu (disk quoty, síť, firewall).

**[MUST]** `apt install ./cesta/k/balicku.deb` (s `./` nebo absolutní cestou) závislosti dořeší z repozitářů — na rozdíl od `dpkg -i`. V testovacích skriptech se používá výhradně tato forma.

#### Úroveň 4 — celý APT řetězec lokálně, bez publikace

Repozitářovou mechaniku (struktura `dists/`, `Packages`, podpis, `apt update`) lze ověřit bez GitHubu — proti lokálnímu adresáři:

```bash
# 1. postav repo přesně stejným skriptem jako CI
./scripts/build-apt-repo.sh out/debian

# 2. v testovacím kontejneru:
incus file push -r out/debian provctl-test/srv/
incus exec provctl-test -- sh -c \
  'echo "deb [trusted=yes] file:///srv/debian stable main" > /etc/apt/sources.list.d/provctl.list'
incus exec provctl-test -- apt update
incus exec provctl-test -- apt install -y provctl
```

**[MUST]** Skript `scripts/build-apt-repo.sh` je **stejný**, jaký volá GitHub Actions. CI ho jen spouští — nesmí mít vlastní inline verzi kroků, jinak testuješ něco jiného, než se publikuje.

`[trusted=yes]` obchází ověření podpisu, což je pro lokální test v pořádku. **[MUST]** Podpis se ověřuje zvlášť, jednou, proti reálnému publikovanému repu — importem klíče přesně podle §26.5.

#### Kdy teprve publikovat

```
push        → lintian + piuparts        (CI, každý commit)
lokálně     → incus/VM smoke test       (před tagem)
lokálně     → file:// APT repo test     (při změně packagingu nebo CI)
tag vX~rc1  → suite testing             (ověření řetězce a upgradu na cizím stroji)
tag vX      → suite stable
```

**[MUST]** Do `stable` se nikdy nepublikuje verze, která neprošla alespoň jedním `apt upgrade` z předchozí stable verze na reálném Debianu.

---

## 27. Milníky a akceptační kritéria

Implementace probíhá v tomto pořadí. **[MUST]** Každý milník končí funkčním, otestovaným a commitnutým stavem — žádné „dodělám později".

| M | Obsah | Akceptační kritérium |
|---|---|---|
| **M0** | skeleton, `meta`, `config`, `system` rozhraní + fake, SQLite + migrace, `doctor` | `provctl doctor` běží; unit testy zelené; kontrola vrstvení v CI |
| **M1** | `plan` executor + rollback + zámek + `operations` journal | test, kde N-tý krok selže, ověří úplný rollback |
| **M2** | subscription create/list/show/delete + Linux user + adresáře + práva | golden test práv; rollback test; `--dry-run` |
| **M3** | website + Apache render + configtest + reload + default vhost + `reconcile` | golden testy všech 4 typů vhostů; `reconcile --dry-run` hlásí drift |
| **M4** | PHP-FPM pool, detekce verzí, změna verze | golden test poolu; test změny verze včetně rollbacku |
| **M5** | MariaDB, hesla, cron, SSH klíče | validace identifikátorů; test, že heslo není v `ps` ani v audit logu |
| **M6** | SSL stavový automat + deploy-hook | test pořadí kroků s fake commanderem; ruční ověření na staging LE |
| **M7** | backup/restore, health, audit log, kvóty | round-trip test backup → restore v kontejneru |
| **M8** | TUI (read-mostly) | |
| **M9** | packaging, nfpm, postinst, GitHub Actions, APT repo, `config migrate` | lintian + piuparts install/upgrade/purge zelené (§26.6); `apt install` z lokálního `file://` repa na čistém Debianu 13; `apt upgrade` nepřepíše `config.toml` ani nesáhne na `/var/www`; opakovaný `bootstrap` nahlásí `nothing to do` |
| **M10** | `subscription adopt` (migrace stávajících webů) | test na kopii reálné struktury |

---

## 28. Zakázané chování (shrnutí pro implementačního agenta)

**[MUST] Nikdy:**

1. Nespouštět shell (`sh -c`) s čímkoli, co pochází z uživatelských dat.
2. Nepředávat hesla jako argumenty procesu (viditelné v `ps`) — vždy stdin.
3. Nezapisovat hesla, klíče ani SQL do audit logu.
4. Nepřepisovat a nemazat systémové soubory bez prefixu `provctl-`.
5. Nedělat automatický `systemctl restart apache2`.
6. Nezapisovat do SQLite dřív, než systémová operace uspěla.
7. Nehardcodovat PHP verzi ani cesty.
8. Nepřidávat `www-data` do skupiny subscription.
9. Neumisťovat logy do adresáře zapisovatelného subscription uživatelem.
10. Nedělat v `postinst` cokoli, co se dotýká zákaznických dat nebo běžících služeb.
11. Netvrdit, že něco funguje, pokud to nebylo spuštěno. Rozlišovat „napsáno", „unit-testováno", „ověřeno na Debianu".
12. Nezvětšovat rozsah — co je označeno **[LATER]**, se v v0.1 neimplementuje.

---

## 29. Otevřené otázky / předpoklady k ověření na cílovém serveru

Tyto body **nebyly ověřeny** a implementace je musí ošetřit defenzivně (detekce + jasná chyba), ne předpokládat:

1. Které PHP verze jsou na serveru skutečně nainstalované a zda je použit Sury.
2. Zda MariaDB umožňuje `unix_socket` autentizaci roota (jinak je nutný `defaults_file`).
3. Maximální délka jména MariaDB uživatele na dané verzi serveru.
4. Zda je Apache jediný frontend, nebo je před ním reverse proxy / CDN (ovlivňuje ACME validaci a `X-Forwarded-For`).
5. Zda server má veřejnou IPv6 (ovlivňuje DNS check).
6. Zda filesystém podporuje disk quoty (relevantní až pro **[LATER]**).
7. E-mail pro Let's Encrypt registraci.
8. GitHub org / název repozitáře a doména pro APT repo.
9. Jakým mechanismem se **dnes** na serveru obnovují certifikáty (vlastní cron vs. `certbot.timer` vs. `/etc/cron.d/certbot`) a které z nich zůstane po nasazení provctl. Řeší `doctor` (§11.2), ale rozhodnutí je na adminovi.
10. Jaký `authenticator` a `webroot_path` mají stávající certifikáty v `/etc/letsencrypt/renewal/` — určuje náročnost `adopt` (§11.5).
11. Zda je `incus` dostupný v Debianu 13 pro lokální testovací kontejnery; jinak VM (§26.6).

---

## 30. Formát reportu po každém milníku

**[MUST]** Implementační agent po každém milníku dodá:

```
## Milník MX

### Co bylo implementováno
### Co bylo ověřeno a jak
  - unit testy: <které>
  - golden testy: <které>
  - ručně/integračně: <co, kde>
### Co ověřeno NEBYLO
### Odchylky od zadání a jejich důvod
### Zbývající omezení a známé problémy
### Otevřené otázky pro autora
```

Bez sekce „Co ověřeno NEBYLO" se milník nepovažuje za dokončený.

