# provctl — testovací cookbook

Doplněk k implementačnímu zadání. Popisuje **jak postavit jednotlivá testovací prostředí** a **jaké konkrétní scénáře v nich spustit**, včetně očekávaných výsledků.

**FAKT / PŘEDPOKLAD:** stejná konvence jako v zadání. Kde je uvedeno PŘEDPOKLAD, ověř to prvním spuštěním, nespoléhej na to.

---

## 0. Přehled prostředí

| ID | Prostředí | Root? | systemd? | Rychlost | Co ověřuje |
|---|---|---|---|---|---|
| **E0** | Lokální dev (tvůj stroj) | ne | ne | sekundy | Go testy, golden files, logika, rollback (fake systém) |
| **E1** | lintian + piuparts | ano | ne | 1–3 min | struktura balíčku, install/upgrade/purge, conffiles |
| **E2** | Systémový kontejner (incus/LXC) | ano | ano | 10 s start | reálný Apache, PHP-FPM, MariaDB, práva, lifecycle |
| **E3** | Lokální ACME (Pebble) v E2 | ano | ano | +1 min | SSL flow, deploy hook, renew — bez rate limitů |
| **E4** | VM (Vagrant/QEMU) | ano | ano | minuty | kernel-level věci: quoty, síť, firewall |
| **E5** | Reálný VPS + veřejná doména | ano | ano | — | Let's Encrypt staging, DNS, skutečné auto-renew |

**Pravidlo:** nic se nepublikuje do APT `testing`, dokud neprojde E0 → E1 → E2 → E3.

## E-1 — vývojové prostředí a izolace

Na hostu ponech pouze nástroje bez systémových služeb: Go, Git, editor, `staticcheck` a `nfpm`. E0 a sestavení balíčku mohou běžet bez rootu. `piuparts`, `debootstrap`, `lintian` a `reprepro` přenech CI nebo Debian VM.

Pro E2/E3 vytvoř Debian 13 VM (4 vCPU, 8 GB RAM, 60–80 GB disk) přes virt-manager/QEMU nebo Vagrant s libvirt. VM snapshotni hned po instalaci nástrojů jako `tooled`; zdrojový kód připoj přes virtiofs nebo do VM doručuj přes Git.

Uvnitř VM nainstaluj a inicializuj pouze Incus; jeho síť, firewall a storage pool tak zůstávají mimo host:

```bash
sudo apt update
sudo apt install -y incus
sudo incus admin init --minimal
sudo usermod -aG incus-admin "$USER"  # potom se znovu přihlásit
incus launch images:debian/13 pv
incus snapshot create pv clean
```

Pokud Docker nastaví politiku `FORWARD DROP` a kontejner nemá odchozí síť, z rootu repozitáře nainstaluj úzce omezenou perzistentní službu:

```bash
sudo ./scripts/dev/install-incus-docker-forwarding.sh
```

Skript zjistí aktivní uplink, nastaví IPv4 forwarding a povolí pouze odchozí provoz `incusbr0` a související odpovědi. Službu odebereš přes `sudo systemctl disable --now incus-docker-forward.service`; pak smaž její soubor a `/etc/sysctl.d/90-incus-forwarding.conf`.

E2 musí být **systémový kontejner** s běžícím systemd, ne Dockerový aplikační kontejner. V něm je bezpečné spouštět `provctl` jako root: uživatelé, `/etc/apache2`, databáze i služby jsou izolované v kontejneru. Neověřuje však věci závislé na kernelu, firewallu, diskových kvótách ani veřejném DNS/Let's Encrypt; ty patří do E4/E5. Před každým mutujícím testem obnov `clean` snapshot.

### Mapa: test → prostředí

| Test | E0 | E1 | E2 | E3 | E4 | E5 |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| T01 unit + golden | ✔ | | | | | |
| T02 rollback (fake) | ✔ | | | | | |
| T03 vrstvení a zakázané vzory | ✔ | | | | | |
| T04 struktura balíčku | | ✔ | | | | |
| T05 install/purge | | ✔ | ✔ | | | |
| T06 upgrade + conffile | | ✔ | ✔ | | | |
| T07 doctor na čistém serveru | | | ✔ | | | |
| T08 bootstrap + idempotence | | | ✔ | | | |
| T09 lifecycle subscription | | | ✔ | | | |
| T10 **izolace práv** | | | ✔ | | | |
| T11 rollback s reálným Apache | | | ✔ | | | |
| T12 reconcile / drift | | | ✔ | | | |
| T13 změna PHP verze | | | ✔ | | | |
| T14 zámek / souběh | | | ✔ | | | |
| T15 backup / restore | | | ✔ | | | |
| T16 SSL + deploy hook + renew | | | | ✔ | | ✔ |
| T17 adopt (migrace) | | | ✔ | | | |
| T18 lokální APT repo | | | ✔ | | | |
| T19 config migrace | ✔ | | ✔ | | | |
| T20 disk quoty | | | | ✔ | | |

---

## 1. E0 — lokální dev

### Setup

```bash
# jednorázově
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/google/go-licenses@latest   # volitelné
```

Nic víc. **[MUST]** Celá sada musí běžet pod běžným uživatelem, bez rootu, bez Debianu, bez sítě.

### Makefile

```makefile
.PHONY: test lint golden-update build deb

test:
	go vet ./...
	staticcheck ./...
	go test ./... -race -count=1

golden-update:
	go test ./internal/render/... -update

build:
	CGO_ENABLED=0 go build -trimpath -o dist/provctl ./cmd/provctl

deb:
	./scripts/build-deb.sh
```

### T01 — unit a golden testy

```bash
make test
```

Očekávané: vše zelené. Golden testy pokrývají **všechny čtyři typy vhostu** (static, php-fpm, proxy, redirect) × {HTTP, HTTP+forceHTTPS, HTTPS} + FPM pool + logrotate + crontab.

Kontrola, že golden soubory nejsou prázdné nebo zapomenuté:

```bash
find . -path '*/testdata/*.golden' -size -10c   # nesmí nic vypsat
```

### T02 — rollback s fake systémem

Fake `Commander` umí selhat na N-tém volání. Test pro každou mutující operaci:

```
pro N = 1..počet_kroků:
    spusť operaci s fake selhávajícím na kroku N
    ověř: žádný soubor nevznikl
    ověř: žádný uživatel nevznikl
    ověř: v SQLite není záznam
    ověř: operations má status 'rolled_back'
    ověř: exit kód 6
```

**[MUST]** Tenhle test je tabulkový a generovaný, ne ručně napsaný pro jeden krok.

### T03 — architektonické invarianty

```bash
go test ./internal/arch/...
```

Ověřuje:
- `tui/` ani `internal/cli/` neimportují `internal/system` nebo `internal/repository`
- `internal/domain` neimportuje nic z projektu
- v repozitáři není `"sh", "-c"` ani `"bash", "-c"` (mimo `testdata/`)
- v repozitáři není hardcodovaná PHP verze: `grep -rn "php8\.\|8\.4\|8\.5" --include='*.go' internal/ | grep -v _test.go` nesmí najít verzi v cestě

### T19a — migrace configu (unit)

`testdata/config/v1.toml` → načíst novou binárkou → ověřit, že se doplní nové klíče, `config_version` se povýší až po `config migrate`, a že `config migrate --dry-run` nic nezapíše.

---

## 2. E1 — lintian + piuparts

### Setup

Potřebuje Debian/Ubuntu host (nebo kontejner) s rootem a sítí.

```bash
sudo apt update
sudo apt install -y lintian piuparts debootstrap
```

**PŘEDPOKLAD:** `piuparts` umí `-d trixie`, pokud má host odpovídající `debootstrap` skripty. Pokud selže, použij `--distribution` s explicitním mirrorem:

```bash
sudo piuparts -d trixie --mirror http://deb.debian.org/debian dist/provctl_*.deb
```

### T04 — struktura balíčku

```bash
./scripts/build-deb.sh                      # postaví do dist/

lintian --no-tag-display-limit --info dist/provctl_*_amd64.deb
dpkg-deb --info     dist/provctl_*_amd64.deb
dpkg-deb --contents dist/provctl_*_amd64.deb
```

**Kontrolní seznam (ručně projít poprvé, pak zautomatizovat skriptem):**

- [ ] `/usr/bin/provctl` je `0755 root/root`
- [ ] `/etc/provctl/config.toml` je uveden v `conffiles`
- [ ] `/usr/share/provctl/templates/*` **není** v `conffiles`
- [ ] `/var/lib/provctl` má `0700`
- [ ] `/var/log/provctl` má `0750`
- [ ] `Depends` obsahuje `apache2`, ne `php*` (PHP je `Suggests`)
- [ ] balíček neobsahuje nic pod `/var/www`
- [ ] velikost binárky je rozumná (< 30 MB)

Automatizace:

```bash
dpkg-deb --contents dist/provctl_*_amd64.deb \
  | awk '{print $1, $2, $6}' > dist/contents.actual
diff -u testdata/packaging/contents.expected dist/contents.actual
```

### T05 — install / remove / purge

```bash
sudo piuparts -d trixie --warn-on-leftover-files dist/provctl_1.0.0_amd64.deb
```

**Očekávané:**
- instalace projde bez interakce
- `postinst` neselže, ani když není Apache nakonfigurovaný
- po `purge` nezůstane `/etc/provctl` ani `/var/lib/provctl`
- piuparts nehlásí neznámé zbylé soubory

### T06 — upgrade a zachování conffile

Potřebuješ dvě verze. Postav starší z tagu nebo jen s jiným `VERSION`:

```bash
VERSION=0.9.0 ./scripts/build-deb.sh
VERSION=1.0.0 ./scripts/build-deb.sh

sudo piuparts -d trixie --warn-on-leftover-files \
  dist/provctl_0.9.0_amd64.deb dist/provctl_1.0.0_amd64.deb
```

**[MUST] Ruční doplněk** (piuparts sám nezkontroluje obsah upraveného configu) — v E2:

```bash
apt install -y ./provctl_0.9.0_amd64.deb
echo '# MOJE POZNAMKA' >> /etc/provctl/config.toml
sed -i 's|^vhosts.*|vhosts = "/data/web/vhosts"|' /etc/provctl/config.toml

apt install -y ./provctl_1.0.0_amd64.deb

grep -q 'MOJE POZNAMKA' /etc/provctl/config.toml && echo "OK: config zachován"
grep -q '/data/web/vhosts'  /etc/provctl/config.toml && echo "OK: hodnota zachována"
test -f /etc/provctl/config.toml.dpkg-dist && echo "CHYBA: dpkg nabídl náhradu"
```

**Očekávané:** obě `OK`, žádná `CHYBA`, a **žádný interaktivní dotaz dpkg** na konfiguraci.

---

## 3. E2 — systémový kontejner (hlavní pracovní prostředí)

### Setup

**PŘEDPOKLAD:** `incus` je dostupný v Debianu 13. Ověř:

```bash
apt-cache policy incus
```

Pokud ano:

```bash
sudo apt install -y incus
sudo incus admin init --minimal
sudo usermod -aG incus-admin "$USER"   # odhlásit/přihlásit
```

Pokud ne, funguje stejně `lxd` (snap) nebo přejdi na E4 (VM). Docker se pro tohle **nedoporučuje** — bez systemd nemá `systemctl` co dělat a testoval bys jinou cestu kódem než produkční.

### Vytvoření a zlatý snapshot

```bash
incus launch images:debian/13 pv --ephemeral=false
incus exec pv -- apt update
incus exec pv -- apt install -y ca-certificates curl

# zlatý stav = čistý Debian 13 PŘED instalací provctl
incus snapshot create pv clean
```

Reset před každým testem (sekundy):

```bash
incus restore pv clean
```

Helper skript `scripts/e2.sh`:

```bash
#!/bin/sh
set -e
CT=pv
case "$1" in
  reset) incus restore $CT clean ;;
  push)  incus file push "$2" $CT/root/ ;;
  sh)    shift; incus exec $CT -- sh -c "$*" ;;
esac
```

### T07 — doctor na čistém serveru

```bash
./scripts/e2.sh reset
./scripts/e2.sh push dist/provctl_1.0.0_amd64.deb
./scripts/e2.sh sh 'apt install -y /root/provctl_1.0.0_amd64.deb'
./scripts/e2.sh sh 'provctl doctor; echo "exit=$?"'
```

**Očekávané:**
- `apt install` dotáhne `apache2` jako závislost
- `doctor` vypíše `FAIL` u chybějícího PHP-FPM, MariaDB, certbota a u nepovolených Apache modulů
- `exit=1`
- **nic** se neopravilo — `ls /etc/apache2/sites-enabled/` neobsahuje `provctl-*`

```bash
./scripts/e2.sh sh 'ls /etc/apache2/sites-enabled/ | grep provctl && echo CHYBA || echo "OK: doctor nic nezměnil"'
```

### T08 — bootstrap a idempotence

```bash
./scripts/e2.sh sh 'provctl bootstrap --install-missing --yes'
./scripts/e2.sh sh 'provctl doctor; echo "exit=$?"'
```

**Očekávané:** `exit=0`, všechny kontroly `OK`.

Idempotence — **povinný test**:

```bash
./scripts/e2.sh sh 'provctl bootstrap --dry-run' > run2.txt
grep -q 'nothing to do' run2.txt && echo "OK: idempotentní"

# a ještě ostrý druhý běh
./scripts/e2.sh sh 'md5sum /etc/apache2/sites-available/provctl-000-default.conf' > a.txt
./scripts/e2.sh sh 'provctl bootstrap --yes'
./scripts/e2.sh sh 'md5sum /etc/apache2/sites-available/provctl-000-default.conf' > b.txt
diff a.txt b.txt && echo "OK: druhý bootstrap nic nezměnil"
```

Test catch-all vhostu:

```bash
./scripts/e2.sh sh 'curl -s -o /dev/null -w "%{http_code}\n" -H "Host: neexistuje.test" http://127.0.0.1/'
```

**Očekávané:** `403`. Ne 200, ne obsah cizího webu.

### T09 — lifecycle subscription

```bash
./scripts/e2.sh sh 'provctl subscription create acme --php-max-children 5'
./scripts/e2.sh sh 'id acme && ls -la /var/www/vhosts/'
./scripts/e2.sh sh 'provctl website create acme example.test --type php-fpm'
./scripts/e2.sh sh 'echo "<?php echo \"HELLO-\".PHP_VERSION;" > /var/www/vhosts/acme/sites/example.test/public/index.php'
./scripts/e2.sh sh 'chown acme:acme /var/www/vhosts/acme/sites/example.test/public/index.php'
./scripts/e2.sh sh 'curl -s -H "Host: example.test" http://127.0.0.1/'
```

**Očekávané:** výstup začíná `HELLO-`. Pokud přijde 403, jsou špatně práva; pokud se stáhne zdrojový kód, není napojený FPM handler.

Ověření běžícího uživatele PHP:

```bash
./scripts/e2.sh sh 'echo "<?php echo posix_getpwuid(posix_geteuid())[\"name\"];" > /var/www/vhosts/acme/sites/example.test/public/whoami.php; chown acme:acme /var/www/vhosts/acme/sites/example.test/public/whoami.php'
./scripts/e2.sh sh 'curl -s -H "Host: example.test" http://127.0.0.1/whoami.php'
```

**Očekávané:** `acme`. Pokud vrátí `www-data`, PHP neběží v poolu subscription a celá izolace je fikce.

Suspend / resume:

```bash
./scripts/e2.sh sh 'provctl subscription suspend acme'
./scripts/e2.sh sh 'curl -s -o /dev/null -w "%{http_code}\n" -H "Host: example.test" http://127.0.0.1/'   # čekáme 403
./scripts/e2.sh sh 'crontab -u acme -l 2>&1'                                                              # čekáme "no crontab"
./scripts/e2.sh sh 'ls /etc/php/*/fpm/pool.d/ | grep provctl-acme && echo CHYBA || echo "OK: pool pryč"'
./scripts/e2.sh sh 'getent passwd acme | grep nologin && echo "OK: shell zamčen"'

./scripts/e2.sh sh 'provctl subscription resume acme'
./scripts/e2.sh sh 'curl -s -H "Host: example.test" http://127.0.0.1/ | head -c 20'                       # čekáme HELLO-
```

### T10 — izolace práv (nejdůležitější bezpečnostní test)

```bash
./scripts/e2.sh sh 'provctl subscription create alfa'
./scripts/e2.sh sh 'provctl subscription create beta'
./scripts/e2.sh sh 'provctl website create alfa a.test --type php-fpm'
./scripts/e2.sh sh 'provctl website create beta b.test --type php-fpm'
./scripts/e2.sh sh 'echo TAJEMSTVI > /var/www/vhosts/alfa/sites/a.test/app/secret.txt; chown alfa:alfa /var/www/vhosts/alfa/sites/a.test/app/secret.txt; chmod 600 /var/www/vhosts/alfa/sites/a.test/app/secret.txt'
```

**Test 1 — čtení cizích souborů shellem:**

```bash
./scripts/e2.sh sh 'sudo -u beta cat /var/www/vhosts/alfa/sites/a.test/app/secret.txt 2>&1'
./scripts/e2.sh sh 'sudo -u beta ls /var/www/vhosts/alfa/ 2>&1'
```
**Očekávané:** obojí `Permission denied`. Výpis obsahu adresáře alfa **nesmí** projít (proto `0751`, ne `0755`).

**Test 2 — čtení cizích souborů přes PHP (`open_basedir`):**

```bash
cat > /tmp/evil.php <<'EOF'
<?php
$p = '/var/www/vhosts/alfa/sites/a.test/app/secret.txt';
var_dump(@file_get_contents($p));
var_dump(@scandir('/var/www/vhosts/alfa'));
EOF
incus file push /tmp/evil.php pv/var/www/vhosts/beta/sites/b.test/public/evil.php
./scripts/e2.sh sh 'chown beta:beta /var/www/vhosts/beta/sites/b.test/public/evil.php'
./scripts/e2.sh sh 'curl -s -H "Host: b.test" http://127.0.0.1/evil.php'
```
**Očekávané:** dvakrát `bool(false)`. Cokoli jiného = díra v `open_basedir`.

**Test 3 — Apache nesmí servírovat neveřejné adresáře:**

```bash
./scripts/e2.sh sh 'curl -s -o /dev/null -w "%{http_code}\n" -H "Host: a.test" http://127.0.0.1/../app/secret.txt'
./scripts/e2.sh sh 'sudo -u www-data cat /var/www/vhosts/alfa/sites/a.test/app/secret.txt 2>&1'
```
**Očekávané:** 400/403/404 a `Permission denied`.

**Test 4 — session isolation:**

```bash
./scripts/e2.sh sh 'grep session.save_path /etc/php/*/fpm/pool.d/provctl-alfa.conf'
./scripts/e2.sh sh 'sudo -u beta ls /var/www/vhosts/alfa/tmp/sessions 2>&1'
```
**Očekávané:** cesta je uvnitř home alfy; beta ji nepřečte.

**Test 5 — symlink útok na logy (privilege escalation):**

```bash
./scripts/e2.sh sh 'sudo -u alfa ln -s /etc/shadow /var/log/provctl/alfa/a.test/access.log 2>&1'
./scripts/e2.sh sh 'stat -c "%U:%G %a" /var/log/provctl/alfa/a.test'
```
**Očekávané:** symlink **selže** (`Permission denied`) a adresář je `root:alfa 750`. Pokud symlink projde, je to kritická chyba — Apache otevírá logy jako root.

**Test 6 — logy jsou čitelné vlastníkem, ale ne cizím:**

```bash
./scripts/e2.sh sh 'sudo -u alfa head -1 /var/log/provctl/alfa/a.test/access.log >/dev/null && echo "OK: alfa čte"'
./scripts/e2.sh sh 'sudo -u beta  head -1 /var/log/provctl/alfa/a.test/access.log 2>&1'
```

**[MUST]** Celý T10 zapiš jako skript `scripts/tests/t10-isolation.sh` s návratovým kódem. Je to test, který se musí pouštět po každé změně práv nebo šablon.

### T11 — rollback s reálným Apache

Trik, jak vynutit selhání bez zásahu do kódu: podstrč vadnou šablonu do override adresáře.

```bash
./scripts/e2.sh sh 'mkdir -p /etc/provctl/templates/apache'
./scripts/e2.sh sh 'cp /usr/share/provctl/templates/apache/php-fpm.conf.tmpl /etc/provctl/templates/apache/'
./scripts/e2.sh sh 'echo "ThisDirectiveDoesNotExist on" >> /etc/provctl/templates/apache/php-fpm.conf.tmpl'

./scripts/e2.sh sh 'provctl website create acme broken.test --type php-fpm; echo "exit=$?"'
```

**Očekávané:**
- `exit=6` (rollback proveden)
- chybová hláška obsahuje výstup `apachectl configtest`
- `ls /etc/apache2/sites-available/ | grep broken` → prázdné
- `provctl website list acme` → `broken.test` tam **není**
- `apachectl configtest` → `Syntax OK`
- původní weby dál fungují

```bash
./scripts/e2.sh sh 'apachectl configtest'
./scripts/e2.sh sh 'curl -s -H "Host: example.test" http://127.0.0.1/ | head -c 6'
./scripts/e2.sh sh 'rm -rf /etc/provctl/templates/apache'   # úklid
```

**Varianta B — baseline configtest:** rozbij konfiguraci *mimo* provctl a ověř, že provctl operaci odmítne a nesnaží se rollbackovat cizí soubor:

```bash
./scripts/e2.sh sh 'echo "GarbageDirective" > /etc/apache2/conf-enabled/zzz-cizi.conf'
./scripts/e2.sh sh 'provctl website create acme other.test --type static; echo "exit=$?"'
```
**Očekávané:** jasná hláška „Apache konfigurace je vadná ještě před touto změnou", **soubor `zzz-cizi.conf` zůstane nedotčen**.

### T12 — reconcile a drift

```bash
./scripts/e2.sh sh 'provctl reconcile --dry-run; echo "exit=$?"'          # čekáme exit=0
./scripts/e2.sh sh 'echo "# rucni zmena" >> /etc/apache2/sites-available/provctl-acme-example.test.conf'
./scripts/e2.sh sh 'provctl reconcile --dry-run; echo "exit=$?"'          # čekáme exit=2 + diff
./scripts/e2.sh sh 'provctl reconcile'
./scripts/e2.sh sh 'provctl reconcile --dry-run; echo "exit=$?"'          # zpět exit=0
```

**Test ochrany cizích souborů:**

```bash
./scripts/e2.sh sh 'echo "# neni provctl" > /etc/apache2/sites-available/mujweb.conf'
./scripts/e2.sh sh 'provctl reconcile'
./scripts/e2.sh sh 'test -f /etc/apache2/sites-available/mujweb.conf && echo "OK: cizí soubor nedotčen"'
```

### T13 — změna PHP verze

Vyžaduje dvě nainstalované verze. Pokud jsou v Debianu 13 dostupné pouze jedna, tento test se přesouvá na E5 (server se Sury) — **a musí to být v reportu uvedeno jako neověřeno**.

```bash
./scripts/e2.sh sh 'provctl php list-versions'
./scripts/e2.sh sh 'provctl php set acme --version <druhá_verze>'
./scripts/e2.sh sh 'curl -s -H "Host: example.test" http://127.0.0.1/'      # HELLO-<nová verze>
./scripts/e2.sh sh 'ls /etc/php/*/fpm/pool.d/ | grep provctl-acme'          # pool jen na jednom místě
./scripts/e2.sh sh 'provctl reconcile --dry-run; echo "exit=$?"'            # čekáme 0
```

### T14 — zámek a souběh

```bash
./scripts/e2.sh sh '(provctl subscription create lock1 & provctl subscription create lock2 & wait) 2>&1'
```

**Očekávané:** obě operace doběhnou korektně (jedna počká na zámek), oba uživatelé existují, žádné poškozené konfigurace, `provctl reconcile --dry-run` vrací 0.

Test timeoutu:

```bash
./scripts/e2.sh sh 'flock -x /run/provctl.lock -c "sleep 40" & sleep 1; provctl subscription create lock3; echo "exit=$?"'
```
**Očekávané:** `exit=10`, hláška uvádí PID držitele zámku.

### T15 — backup a restore (round-trip)

```bash
./scripts/e2.sh sh 'provctl database create acme main --write-credentials /root/db.txt'
./scripts/e2.sh sh 'MYSQL_PWD=$(grep -oP "password=\K.*" /root/db.txt) mysql -u acme_main acme_main -e "CREATE TABLE t(id INT); INSERT INTO t VALUES (42);"'
./scripts/e2.sh sh 'echo MARKER > /var/www/vhosts/acme/sites/example.test/public/marker.txt'

./scripts/e2.sh sh 'provctl backup create acme'
./scripts/e2.sh sh 'provctl backup list acme'
./scripts/e2.sh sh 'provctl backup inspect acme <id>'      # ověří SHA256SUMS

./scripts/e2.sh sh 'provctl subscription archive acme'
./scripts/e2.sh sh 'provctl subscription delete acme --confirm-name acme --yes-i-am-sure'
./scripts/e2.sh sh 'id acme 2>&1; ls /var/www/vhosts/'     # uživatel i data pryč

./scripts/e2.sh sh 'provctl backup restore acme <id>'
./scripts/e2.sh sh 'cat /var/www/vhosts/acme/sites/example.test/public/marker.txt'   # MARKER
./scripts/e2.sh sh 'curl -s -H "Host: example.test" http://127.0.0.1/ | head -c 6'   # HELLO-
```

**[MUST]** Ověř explicitně:
- restore vypsal **nová** hesla k DB a upozornění na nutnost aktualizace aplikací
- `mysql -e "SELECT * FROM t"` vrací 42
- práva po restore odpovídají §6.2 (spusť znovu T10)
- `provctl reconcile --dry-run` vrací 0

### T17 — adopt (migrace existujícího webu)

```bash
./scripts/e2.sh reset
# simulace tvého současného stavu
./scripts/e2.sh sh 'mkdir -p /var/www/stary.test && echo "<?php echo \"STARY\";" > /var/www/stary.test/index.php'
./scripts/e2.sh sh 'apt install -y /root/provctl_1.0.0_amd64.deb && provctl bootstrap --install-missing --yes'

./scripts/e2.sh sh 'provctl subscription adopt stary --from /var/www/stary.test --domain stary.test --dry-run'
./scripts/e2.sh sh 'provctl subscription adopt stary --from /var/www/stary.test --domain stary.test'
./scripts/e2.sh sh 'curl -s -H "Host: stary.test" http://127.0.0.1/'      # STARY
./scripts/e2.sh sh 'ls -la /var/www/vhosts/stary/sites/stary.test/public/'
```

**Očekávané:** `--dry-run` vypíše plán a **nic nepřesune**; ostrý běh přesune data, nastaví práva, vytvoří uživatele a web funguje. Záloha před přesunem existuje.

### T18 — lokální APT repozitář

```bash
./scripts/build-apt-repo.sh out/debian          # STEJNÝ skript, jaký volá CI
incus file push -r out/debian pv/srv/
./scripts/e2.sh sh 'echo "deb [trusted=yes] file:///srv/debian stable main" > /etc/apt/sources.list.d/provctl.list'
./scripts/e2.sh sh 'apt update && apt-cache policy provctl'
./scripts/e2.sh sh 'apt install -y provctl && provctl --version'
```

**Očekávané:** `apt-cache policy` ukazuje správnou verzi z lokálního repa; instalace projde závislostmi.

Test povýšení verze:

```bash
VERSION=1.1.0 ./scripts/build-deb.sh && ./scripts/build-apt-repo.sh out/debian
incus file push -r out/debian pv/srv/
./scripts/e2.sh sh 'apt update && apt list --upgradable | grep provctl'
./scripts/e2.sh sh 'apt upgrade -y && provctl --version'
```

**Ověř také řazení RC verzí:**

```bash
dpkg --compare-versions 1.1.0~rc1 lt 1.1.0 && echo "OK: ~rc1 je starší"
dpkg --compare-versions 1.1.0-rc1 lt 1.1.0 || echo "POZOR: -rc1 by se řadilo NAD 1.1.0"
```

### T19b — migrace configu na serveru

```bash
./scripts/e2.sh sh 'sed -i "/config_version/d" /etc/provctl/config.toml'   # simulace starého configu
./scripts/e2.sh sh 'provctl doctor 2>&1 | grep -i "config"'                # čekáme upozornění
./scripts/e2.sh sh 'provctl config migrate --dry-run'                      # ukáže diff, nezapíše
./scripts/e2.sh sh 'provctl config migrate'
./scripts/e2.sh sh 'ls /etc/provctl/config.toml.bak-*'                     # záloha existuje
./scripts/e2.sh sh 'grep config_version /etc/provctl/config.toml'
```

---

## 4. E3 — SSL lokálně přes Pebble (bez rate limitů)

Testovat SSL proti ostrému Let's Encrypt je špatný nápad: potřebuje veřejnou DNS, veřejnou IP a má přísné limity. Pro vývoj slouží **Pebble** — testovací ACME server od Let's Encrypt.

**PŘEDPOKLAD:** Pebble není v Debianu jako balíček; distribuuje se jako Go binárka / kontejner. Ověř aktuální způsob instalace v jeho repozitáři.

### Setup uvnitř E2

```bash
# 1. Pebble běží v kontejneru na portu 14000 (ACME) a 15000 (management)
#    Konfigurace musí mít vypnutou validaci na náhodném portu:
#    "httpPort": 80, "tlsPort": 443
# 2. Certbot musí věřit Pebble CA:
./scripts/e2.sh sh 'curl -sk https://localhost:15000/roots/0 > /usr/local/share/ca-certificates/pebble.crt'
./scripts/e2.sh sh 'update-ca-certificates'
# 3. DNS: doména musí ukazovat na kontejner
./scripts/e2.sh sh 'echo "127.0.0.1 ssl.test" >> /etc/hosts'
```

V configu provctl:

```toml
[ssl]
email  = "test@example.test"
server = "https://localhost:14000/dir"     # override ACME serveru
```

**[MUST]** Zadání musí počítat s konfigurovatelnou `ssl.server` URL — jinak nejde SSL flow testovat vůbec. (Doplnit do §23 zadání.)

### T16 — SSL flow

```bash
./scripts/e2.sh sh 'provctl website create acme ssl.test --type php-fpm'
./scripts/e2.sh sh 'provctl ssl enable acme ssl.test; echo "exit=$?"'
```

**Kontrolní seznam:**

- [ ] před vydáním existoval HTTP vhost a `.well-known` vracelo 404, ne 301
- [ ] `/etc/letsencrypt/live/provctl-acme-ssl.test/fullchain.pem` existuje
- [ ] `:443` vhost vznikl **až po** vydání certifikátu
- [ ] `apachectl configtest` → `Syntax OK`
- [ ] `curl -sk https://ssl.test/` funguje
- [ ] v DB je `certificates` řádek se správným `not_after`

```bash
./scripts/e2.sh sh 'grep -c "443" /etc/apache2/sites-available/provctl-acme-ssl.test.conf'
./scripts/e2.sh sh 'openssl x509 -in /etc/letsencrypt/live/provctl-acme-ssl.test/cert.pem -noout -enddate'
./scripts/e2.sh sh 'provctl ssl status acme ssl.test --json'
```

**Test force-HTTPS a průchodnosti ACME:**

```bash
./scripts/e2.sh sh 'provctl website set acme ssl.test --force-https'
./scripts/e2.sh sh 'curl -s -o /dev/null -w "%{http_code}\n" http://ssl.test/'                          # 301
./scripts/e2.sh sh 'curl -s -o /dev/null -w "%{http_code}\n" http://ssl.test/.well-known/acme-challenge/x'  # 404, NE 301
```

**Toto je nejdůležitější kontrola celého SSL — pokud `.well-known` vrací 301, obnova certifikátu za tři měsíce tiše selže.**

### T16b — deploy hook a renew

```bash
./scripts/e2.sh sh 'cat /etc/letsencrypt/renewal/provctl-acme-ssl.test.conf | grep -E "authenticator|webroot_path"'
```
**Očekávané:** `authenticator = webroot`, `webroot_path = /var/lib/provctl/acme-challenge`.

```bash
./scripts/e2.sh sh 'certbot renew --cert-name provctl-acme-ssl.test --dry-run; echo "exit=$?"'
```
**Očekávané:** `exit=0`.

Vynucená obnova a ověření hooku:

```bash
./scripts/e2.sh sh 'certbot renew --cert-name provctl-acme-ssl.test --force-renewal'
./scripts/e2.sh sh 'tail -5 /var/log/provctl/deploy-hook.log'
./scripts/e2.sh sh 'systemctl show apache2 -p ActiveEnterTimestamp'    # reload proběhl
./scripts/e2.sh sh 'provctl ssl status acme ssl.test --json | grep not_after'
```

**Očekávané:** hook se spustil, `not_after` v DB se aktualizovalo, Apache byl reloadnut, **hook vrátil 0 i kdyby provctl selhal** (ověř tak, že dočasně přejmenuješ binárku a spustíš renew znovu — obnova musí přesto proběhnout).

### T16c — konfliktní mechanismy obnovy

```bash
./scripts/e2.sh sh 'echo "0 3 * * * root certbot renew -q" > /etc/cron.d/muj-certbot'
./scripts/e2.sh sh 'provctl doctor 2>&1 | grep -i renew'
```
**Očekávané:** `WARN` s výpisem obou mechanismů (`certbot.timer` i `/etc/cron.d/muj-certbot`) a doporučením nechat jen jeden. provctl **nic nesmaže**.

---

## 5. E5 — reálný server, Let's Encrypt staging

Až po zeleném E3. Potřebuje veřejnou doménu s A záznamem na testovací VPS.

```toml
[ssl]
staging = true
```

```bash
provctl ssl enable acme test.tvojedomena.cz
certbot certificates | grep -i "test cert"      # potvrzení, že je ze staging
```

**[MUST]** Nikdy nepřepínat na ostrý LE, dokud staging neprošel. Rate limity ostrého LE jsou tvrdé a čekání je v řádu dnů.

**Zde a jen zde se ověřuje:** skutečné DNS ověření, chování za NAT/CDN, reálný auto-renew po měsících (nebo simulovaný `--force-renewal`).

---

## 6. E4 — VM, jen pro kernel-level

Potřeba až pro **[LATER]** disk quoty, testy s odlišným filesystémem a síťové/firewall scénáře.

```bash
# Vagrant
vagrant init debian/trixie64
vagrant up
vagrant ssh
```

Nebo QEMU s cloud image. Do v0.1 tohle prostředí nepotřebuješ.

---

## 7. Struktura testovacích skriptů

```
scripts/
├── build-deb.sh            # volá nfpm, používá CI i lokál
├── build-apt-repo.sh       # volá reprepro, používá CI i lokál
├── e2.sh                   # helper nad incus
└── tests/
    ├── run-all.sh          # spustí T05–T19 v E2, vrací nenulový kód při selhání
    ├── t04-package.sh
    ├── t07-doctor.sh
    ├── t08-bootstrap.sh
    ├── t09-lifecycle.sh
    ├── t10-isolation.sh    # POVINNÝ po každé změně práv nebo šablon
    ├── t11-rollback.sh
    ├── t12-reconcile.sh
    ├── t15-backup.sh
    ├── t16-ssl.sh
    └── t17-adopt.sh
```

**[MUST]** Každý skript:
- začíná `set -eu`
- resetuje kontejner na snapshot `clean`
- na konci vypíše `PASS` / `FAIL: <důvod>` a vrací odpovídající exit kód
- **nezávisí na pořadí spuštění** ostatních skriptů

**[MUST]** `scripts/build-deb.sh` a `scripts/build-apt-repo.sh` používá CI i lokální testování. Žádné inline kroky ve workflow — jinak testuješ jiný proces, než jaký publikuje.

---

## 8. Postup před vydáním (checklist)

```
[ ] make test                              (E0)
[ ] scripts/tests/t04-package.sh           (E1)
[ ] piuparts install/purge                 (E1)
[ ] piuparts upgrade z předchozí verze     (E1)
[ ] scripts/tests/run-all.sh               (E2)
[ ] t10-isolation.sh zvlášť a pozorně      (E2)
[ ] t16-ssl.sh proti Pebble                (E3)
[ ] lokální file:// APT repo               (E2)
[ ] git tag vX.Y.Z~rc1  → suite testing
[ ] instalace z testing na druhém stroji
[ ] apt upgrade z předchozí stable
[ ] git tag vX.Y.Z      → suite stable
```

---

## 9. Co tímto ověřeno NENÍ

**[MUST]** Uvádět v každém reportu, dokud to neplatí:

- chování při stovkách subscriptions (výkon `du`, počet FPM procesů, doba `reconcile`)
- reálné auto-renew v horizontu měsíců
- chování při plném disku uprostřed zálohy
- chování při pádu serveru uprostřed operace (test lze simulovat `kill -9` během operace a následným `provctl operation list`)
- souběh s ruční úpravou Apache konfigurace administrátorem
- restore zálohy mezi různými servery s odlišnými UID
- chování za CDN/reverse proxy před Apache
