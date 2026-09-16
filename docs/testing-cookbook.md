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

Pro opakovatelnou kontrolu bez instalace balíčku použij:

```bash
./scripts/tests/t04-package.sh dist/provctl_0.0.0+git.<sha>_amd64.deb
```

Skript ověří control metadata, conffile, povinné cesty a práva, absenci
zákaznických dat pod `/var/www` a velikost balíčku. Nevyžaduje root ani Incus.

Automatizace:

```bash
dpkg-deb --contents dist/provctl_*_amd64.deb \
  | awk '{print $1, $2, $6}' > dist/contents.actual
diff -u testdata/packaging/contents.expected dist/contents.actual
```

### T05 — install / remove / purge

```bash
sudo piuparts -d trixie dist/provctl_1.0.0_amd64.deb
```

**Očekávané:**
- instalace projde bez interakce
- `postinst` neselže, ani když není Apache nakonfigurovaný
- po `purge` nezůstane `/etc/provctl` ani `/var/lib/provctl`
- piuparts nehlásí neznámé zbylé soubory

Opakovatelný E2 běh, který instaluje `piuparts` pouze do `pv` a po úspěchu i
selhání obnoví snapshot, je:

```bash
./scripts/tests/t05-piuparts.sh dist/provctl_0.0.0+git.<sha>_amd64.deb
```

Debian 13 `piuparts 1.6.0` nepodporuje historický přepínač
`--warn-on-leftover-files`; upozornění na zbylé soubory kontroluje standardně.

### T06 — upgrade a zachování conffile

Potřebuješ dvě verze. Postav starší z tagu nebo jen s jiným `VERSION`:

```bash
VERSION=0.9.0 ./scripts/build-deb.sh
VERSION=1.0.0 ./scripts/build-deb.sh

sudo apt install ./dist/provctl_0.9.0_amd64.deb
sudo apt install ./dist/provctl_1.0.0_amd64.deb
```

Pro stejný upgrade v izolovaném `pv` bez E1 nástrojů na hostu:

```bash
./scripts/tests/t06-piuparts-upgrade.sh \
  dist/provctl_0.9.0_amd64.deb dist/provctl_1.0.0_amd64.deb
```

**[MUST] Kontrola conffile** (piuparts sám nezkontroluje obsah upraveného configu) — v E2:

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

`t06-piuparts-upgrade.sh` provádí skutečný `dpkg -i` upgrade a stejnou
aserci automaticky, s vlastním jednoznačným komentářem a cestou
`/data/web/vhosts`. `piuparts` nekontroluje upgrade dvou lokálních `.deb`,
protože je nemá v APT cache; pro něj zůstává T05 install/purge kontrolou.

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

### Lokální E2 helper

Scénáře níže používají `scripts/e2.sh`, aby se žádný příkaz určený pro test
omylem nespustil na hostu. Helper je záměrně omezený na instanci `pv` a její
snapshot `clean`; `sh` spustí zadaný příkaz jako root **jen uvnitř** tohoto
kontejneru. Po restore helper čeká nejvýše 60 sekund na `running` nebo
`degraded` systemd stav, aby pomalejší start kontejneru nezpůsobil falešné
selhání integračního testu.

```bash
chmod +x scripts/e2.sh
./scripts/e2.sh status
./scripts/e2.sh reset
./scripts/e2.sh push dist/provctl_1.0.0_amd64.deb
./scripts/e2.sh sh 'dpkg -i /root/provctl_1.0.0_amd64.deb'
```

Vyžaduje to aktivní členství ve skupině `incus-admin` (po `usermod` se nově
přihlas nebo použij `sg incus-admin -c '…'`). `reset` čeká až 30 sekund na
systemd a vrátí úspěch také pro očekávaný stav `degraded`; před každým
mutujícím scénářem jej spusť znovu.

### Ruční TUI smoke test jedním příkazem

`scripts/dev/run-tui-test.sh` je určený výhradně pro ruční kontrolu v reálném
terminálu. Standardně obnoví izolovaný kontejner `pv-tls-debug-20260913` na
fixture `tui-ssh-key-picker`, sestaví právě checkoutnutý balíček, nainstaluje
jej do kontejneru a bez mezikroku otevře TUI. Vybraný kontejner se obnovuje,
proto jej nikdy nenastavuj na pracovní `pv`.

```bash
./scripts/dev/run-tui-test.sh
```

Volitelně lze předat již sestavený `.deb`, například pro reprodukci konkrétní
verze. `q` ukončí TUI a vrátí se do hostitelského shellu:

```bash
./scripts/dev/run-tui-test.sh dist/provctl_0.0.0+git.bfc8d57_amd64.deb
```

Pokud ne, funguje stejně `lxd` (snap) nebo přejdi na E4 (VM). Docker se pro tohle **nedoporučuje** — bez systemd nemá `systemctl` co dělat a testoval bys jinou cestu kódem než produkční.

### Vytvoření a zlatý snapshot

```bash
incus launch images:debian/13 pv --ephemeral=false
incus exec pv -- apt update
incus exec pv -- apt install -y ca-certificates curl cron zstd

# zlatý stav = čistý Debian 13 PŘED instalací provctl
incus snapshot create pv clean
```

Reset před každým testem (sekundy):

```bash
incus snapshot restore pv clean
```

Helper skript `scripts/e2.sh`:

```bash
#!/bin/sh
set -e
CT=pv
case "$1" in
  reset) incus snapshot restore $CT clean ;;
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
./scripts/e2.sh sh 'grep session.save_path /etc/php/*/fpm/pool.d/provctl-alfa-a.test.conf'
./scripts/e2.sh sh 'sudo -u beta ls /var/www/vhosts/alfa/tmp/sessions 2>&1'
```
**Očekávané:** cesta je uvnitř home alfy; beta ji nepřečte.

**Test 5 — symlink útok na logy (privilege escalation):**

```bash
./scripts/e2.sh sh 'sudo -u alfa ln -s /etc/shadow /var/log/provctl/alfa/a.test/shadow-link 2>&1'
./scripts/e2.sh sh 'stat -c "%U:%G %a" /var/log/provctl/alfa/a.test'
```
**Očekávané:** symlink **selže** (`Permission denied`) a adresář je `root:alfa 750`. Pokud symlink projde, je to kritická chyba — Apache otevírá logy jako root.

**Test 6 — logy jsou čitelné vlastníkem, ale ne cizím:**

```bash
./scripts/e2.sh sh 'sudo -u alfa head -1 /var/log/provctl/alfa/a.test/access.log >/dev/null && echo "OK: alfa čte"'
./scripts/e2.sh sh 'sudo -u beta  head -1 /var/log/provctl/alfa/a.test/access.log 2>&1'
```

**[MUST]** Celý T10 zapiš jako skript `scripts/tests/t10-isolation.sh` s návratovým kódem. Je to test, který se musí pouštět po každé změně práv nebo šablon.

Skript je implementovaný; přijímá přesnou cestu k `.deb`, vždy obnoví `pv`
po úspěchu i selhání a nepouští žádný testovací příkaz na hostu:

```bash
./scripts/tests/t10-isolation.sh dist/provctl_0.0.0+git.<sha>_amd64.deb
```

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

### T13 — změna PHP verze per doménu

Vyžaduje dvě nainstalované verze. Pokud standardní Debian 13 obsahuje jen jednu,
dočasně přidej v testovacím kontejneru repozitář Sury a druhou FPM verzi; po
testu vždy obnov `clean`. Pro ověření M4 byl použit PHP 8.3 ze Sury vedle
distribučního PHP 8.4. Produkční rozhodnutí o Sury je samostatné a tato
testovací závislost se nesmí promítnout do balíčku `provctl`.

```bash
./scripts/e2.sh sh 'provctl php list-versions'
./scripts/e2.sh sh 'provctl website create acme api.example.test --type php-fpm'
./scripts/e2.sh sh 'provctl php set acme example.test --version <verze-A>'
./scripts/e2.sh sh 'provctl php set acme api.example.test --version <verze-B>'
./scripts/e2.sh sh 'test -S /run/php/provctl-acme-example.test.sock'
./scripts/e2.sh sh 'test -S /run/php/provctl-acme-api.example.test.sock'
./scripts/e2.sh sh 'grep -F provctl-acme-example.test.sock /etc/apache2/sites-available/provctl-acme-example.test.conf'
./scripts/e2.sh sh 'grep -F provctl-acme-api.example.test.sock /etc/apache2/sites-available/provctl-acme-api.example.test.conf'
./scripts/e2.sh sh 'apache2ctl configtest'
./scripts/e2.sh sh 'provctl reconcile --dry-run; echo "exit=$?"'            # čekáme 0
```

**Očekávané:** každá doména používá nezávislý pool a socket odpovídající
vybrané verzi; změna jedné domény nepřepíše druhou. Oba vhosty míří na své
sockety a `apache2ctl configtest` projde.

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

Přepis existující subscription se testuje odděleně až po čistém round-tripu:

```bash
# Nech obnovenu subscription existovat, uprav marker a obnov původní backup.
./scripts/e2.sh sh 'echo CURRENT > /var/www/vhosts/acme/sites/example.test/public/marker.txt'
./scripts/e2.sh sh 'provctl backup restore acme <puvodni-id> --force --confirm-name acme --yes-i-am-sure'
./scripts/e2.sh sh 'cat /var/www/vhosts/acme/sites/example.test/public/marker.txt' # MARKER
./scripts/e2.sh sh 'provctl backup list acme' # obsahuje i current-state backup
```

**[MUST]** `--force` nesmí mazat existující subscription, pokud vytvoření
current-state zálohy selže; při chybě mazání musí výstup obsahovat ID této
zálohy.

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

TLS větev vyžaduje samostatný Pebble obraz: před adopcí připrav platný legacy
lineage pokrývající `stary.test` (a případné SAN aliasy), pak ověř `curl -k
--resolve stary.test:443:127.0.0.1 https://stary.test/`, `provctl ssl status
stary stary.test` a `certbot renew --cert-name <legacy-lineage>
--force-renewal --no-random-sleep-on-renew`. Pro produkční Certbot konfiguraci
bez `ssl.server` zůstává správnou bezpečnou kontrolou `--dry-run`; Pebble by
jej ale přesměroval na veřejný staging endpoint.
Po smazání adopovaného webu ověř, že jeho legacy lineage v
`/etc/letsencrypt/live/` zůstal zachovaný. Běžný snapshot `clean` Pebble
neobsahuje, proto je tento follow-up oddělený od standardního E2 průchodu.

### T17b — adopt s existujícím TLS lineage

T17b vytvoří na izolované instanci funkční legacy vhost, vydá jeho certifikát
přes Pebble a teprve pak provede adopci. Ověřuje přechod původního webrootu na
centrální ACME webroot, TLS vhost, forced renewal a zachování cizího lineage po
smazání provctl website. Nezasahuje do interaktivního `pv`, pokud použiješ
debug instanci a její snapshot:

```bash
PATH=/home/linuxbrew/.linuxbrew/bin:$PATH \
PROVCTL_E2_INSTANCE=pv-tls-debug-20260913 \
PROVCTL_E2_SNAPSHOT=isolated-clean \
PROVCTL_E2_REGENERATE_NIC=true \
  ./scripts/tests/t17-adopt-tls.sh dist/provctl_*.deb /tmp/provctl-pebble
```

Skript vždy obnoví zvolený snapshot. Vyžaduje lokální checkout upstream
Pebble stejně jako T16 a není součástí `run-all.sh`.

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

**PŘEDPOKLAD:** Pebble není v Debianu jako balíček; distribuuje se jako Go binárka / kontejner. Ověř aktuální způsob instalace v jeho repozitáři. Automatizovaný scénář T16 používá jeho upstream checkout, z něhož na hostu sestaví binárku a spustí ji až uvnitř `pv`; ACME, DNS a HTTP-01 tedy neopustí testovací kontejner.

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
git clone --depth 1 https://github.com/letsencrypt/pebble.git /tmp/provctl-pebble
PATH=/home/linuxbrew/.linuxbrew/bin:$PATH \
  sg incus-admin -c './scripts/tests/t16-ssl.sh dist/provctl_*.deb /tmp/provctl-pebble'
```

T16 obnoví `pv` na `clean` i po selhání. Není součástí `run-all.sh`, protože
vyžaduje síť při prvním stažení Pebble zdrojů a lokální Go toolchain.

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
./scripts/e2.sh sh 'certbot renew --cert-name <lineage> --dry-run; echo "exit=$?"'
```
**Očekávané:** `exit=0`.

Pro lokální Pebble endpoint použij místo `--dry-run`
`--force-renewal --no-random-sleep-on-renew`.
Certbot totiž při `--dry-run` vědomě přepíná ACME server na veřejný Let’s
Encrypt staging endpoint; `provctl ssl enable` proto pro explicitní
`ssl.server` ověřuje obnovu pomocí `--force-renewal`, zatímco pro běžnou
produkční konfiguraci zůstává bezpečný `--dry-run`.

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
    ├── run-all.sh          # spustí implementované T04/T05/[T06]/T10/T17, nenulový kód při selhání
    ├── t04-package.sh
    ├── t05-piuparts.sh
    ├── t06-piuparts-upgrade.sh
    ├── t10-isolation.sh    # POVINNÝ po každé změně práv nebo šablon
    ├── t16-ssl.sh
    ├── t17-adopt.sh
    └── t17-adopt-tls.sh  # Pebble TLS lineage adoption; separate from run-all
```

`run-all.sh` přijímá aktuální `.deb` a volitelně starší `.deb` pro T06:

```bash
./scripts/tests/run-all.sh dist/provctl_1.0.0_amd64.deb \
  dist/provctl_0.9.0_amd64.deb
```

Spouští T04, T05, volitelné T06, T10 a T17 v tomto pořadí. Při prvním selhání
vypíše `FAIL: Tnn (exit N)` a předá návratový kód dané etapy; `PASS` na konci
proto znamená úspěch celého implementovaného rozsahu, nikoli jen posledního
skriptu.

**[MUST]** Každý E2 skript:
- začíná `set -eu`
- resetuje kontejner na snapshot `clean`
- na konci vypíše `PASS` / `FAIL: <důvod>` a vrací odpovídající exit kód
- **nezávisí na pořadí spuštění** ostatních skriptů

**[MUST]** `scripts/build-deb.sh` a `scripts/build-apt-repo.sh` používá CI i lokální testování. Žádné inline kroky ve workflow — jinak testuješ jiný proces, než jaký publikuje.

GitHub Actions sestavuje balíček a kontroluje `lintian` v hostitelském jobu.
`piuparts` běží v samostatném jobu jako privilegovaný Docker kontejner
`debian:trixie`, který stahuje přesně artefakt vytvořený package jobem.
Privilegovaný kontejner je nutný pouze pro vlastní dočasný piuparts chroot,
který připojuje `/proc`; neprovádí žádnou operaci nad hostitelským projektem.

---

## 8. Postup před vydáním (checklist)

```
[ ] make test                              (E0)
[ ] scripts/tests/t04-package.sh           (E1)
[ ] piuparts install/purge                 (E1)
[ ] piuparts upgrade z předchozí verze     (E1)
[ ] scripts/tests/run-all.sh               (E2)
[ ] t10-isolation.sh zvlášť a pozorně      (E2)
[ ] t17-adopt.sh                           (E2)
[ ] t17-adopt-tls.sh proti Pebble          (E3)
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
