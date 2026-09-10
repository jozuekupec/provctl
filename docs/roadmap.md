# provctl roadmap

Tento dokument je živý přehled implementace vázaný na
[projektovou specifikaci](project-specification.md). Každý dokončený krok musí
být označen, otestován, commitnut a odeslán před zahájením dalšího milníku.

Legenda: `[x]` hotovo a ověřeno v uvedeném rozsahu; `[~]` rozpracováno;
`[ ]` nezačato.

## Základ a dosavadní postup

- [x] **M0 — základ projektu:** Go CLI, konfigurace, systémové abstrakce a
  fake, SQLite migrace, `doctor` a architektonické testy.
- [x] **M1 — operační jádro:** plán, journal operací, zámek a rollback.
- [x] **M2 — subscriptions:** vytvoření, výpis, detail a bezpečné smazání;
  uživatel, skupina a adresáře s izolačními právy.
- [x] **M3 — websites a Apache:** PHP-FPM website create, ukládání domény,
  render a atomická aplikace HTTP vhostu, povolení modulů a defaultní catch-all
  vhost jsou hotové. Renderery pro static/proxy/redirect jsou připravené a
  proxy cíl je omezen na loopback či allowlist s povinným neprivilegovaným
  portem. Static lifecycle (`website create --type static`) je napojený a
  unit-testovaný i integračně ověřený v `pv` lokálním HTTP požadavkem; zbývá
  správa webů a `reconcile`. Proxy a redirect lifecycle včetně CLI jsou
  napojené, perzistují cíl/redirect kód, mají cílené unit testy a byly ověřeny
  v `pv` skutečným HTTP proxy požadavkem i odpovědí `302` s `Location`.
  Read-only `website list <subscription>` a `website show <subscription> <domain>`
  jsou dostupné přes service vrstvu a ověřené v `pv`. `website enable` a
  `website disable` atomicky přepínají symlink Apache i hodnotu SQLite s
  rollbackem; oba směry jsou ověřené v `pv`. `website delete` vyžaduje dvojí
  potvrzení, odstraní generovaný vhost a SQLite záznam, ale záměrně zachová
  obsah webu a logy; je ověřený v `pv`. `website logs` bezpečně čte omezený
  konec access/error logu (1–1000 řádků) a je ověřený v `pv`; `--follow`
  zatím chybí. Před integračním během po restore je nutné počkat na aktivní
  Apache, jinak může jeho runtime adresář krátce chybět. `website alias
  add|remove` atomicky přerenderuje Apache vhost a upraví SQLite; obě cesty
  jsou ověřené v `pv`. `reconcile` nyní z SQLite obnoví obsah všech
  spravovaných HTTP vhostů i jejich enabled symlinky; `--dry-run` vypíše
  line-oriented unified diff a při driftu končí kódem 2. Skutečný běh vytváří
  žurnálovanou rollbackovatelnou operaci. V `pv` byl ověřen úmyslně změněný
  vhost i smazaný symlink, následný `apachectl configtest` a druhý dry-run bez
  driftu. Golden testy nyní pokrývají všechny čtyři typy HTTP vhostu.
- [x] **M4 — PHP-FPM:** automatická detekce a výběr verze, render a atomické
  vytvoření poolu včetně ověření socketu jsou hotové. `php list-versions` a
  žurnálované `php set <sub> --version <ver>` nyní vytvářejí nový pool,
  přerenderují všechny vhosty subscription, odstraní starý pool a nakonec
  atomicky zapíší verzi i nastavení poolu do SQLite; každý krok má rollback.
  Změna limitů ve stejné verzi aktualizuje existující pool, aniž by jej
  odstranila. Sdílený socket vyžaduje bezpečné předání: starý pool se odstraní,
  systém čeká nejvýše 10 sekund na uvolnění socketu a teprve poté aktivuje nový
  pool; timeout vrátí starý pool žurnálovaným rollbackem. V `pv` byl ověřen
  dry-run i změna `max_children`, configtest Apache, zapsaná verze a obousměrné
  HTTP přepnutí 8.4 → 8.3 → 8.4 s dodatečně instalovaným PHP 8.3 ze Sury.

## Bezprostřední práce

- [x] `bootstrap`: vytvoření systémových adresářů a audit logu s právy ze
  specifikace, moduly, výchozí certifikát, vhost, deploy-hook, logrotate i
  skutečně prázdný druhý běh (`nothing to do`) jsou hotové. Mutující běh nyní
  vyžaduje potvrzení, nebo explicitní `--yes`; `--skip` přijímá jen pevně
  pojmenované volitelné artefakty a nikdy bezpečnostní adresáře. `--install-missing`
  kontroluje pevný allowlist oficiálních Debian balíčků, dpkg lock a spouští
  explicitní noninteractive `apt-get`; bez něj vypíše příkaz pro ruční instalaci.
  Po běhu se vypíše výsledný `doctor`. Offline testy pokrývají allowlist,
  lock, apt argumenty a skip validaci; `make test` prošel.
- [x] Unit testy bootstrapu pokrývají prázdný plán, chybějící systémové cesty,
  odmítnutí změny práv existujícího adresáře i rollback nově vytvořené cesty po
  neúspěšném Apache configtestu.
- [x] Projít end-to-end HTTP požadavek přes Apache v Incus kontejneru `pv`; po
  každém integračním testu obnovit snapshot `clean`. PHP-FPM, static, proxy i
  redirect tok jsou ověřeny přes lokální HTTP požadavky s `--resolve`.
  Poslední běh proxy přenesl tělo z `127.0.0.1:8080`; redirect vrátil `302` a
  očekávaný `Location`. Apache configtest uspěl a kontejner byl obnoven na
  `clean`.

## Následující milníky

- [x] **M5 — MariaDB, SSH a cron:** databázový lifecycle je žurnálovaný a
  dostupný přes `database create|list|password|delete`. Jméno se skládá jako
  `<subscription>_<name>`, před vytvořením se dynamicky ověřuje limit uživatele
  na cílovém MariaDB serveru a SQL jde výhradně přes stdin. Hesla jsou
  kryptografická, nezapisují se do SQLite a CLI je vypíše pouze po úspěchu.
  Create má rollback databáze i metadata; delete nejdříve odstraňuje metadata,
  aby je při selhání serverového dropu vrátil. V `pv` byly ověřeny create, list,
  změna hesla i delete proti skutečné MariaDB přes unix socket, včetně existence
  a následného odstranění databáze a uživatele; kontejner byl obnoven na `clean`.
  `--write-credentials` bezpečně odmítá existující nebo mimodomovský soubor a
  v `pv` vytvořil nový soubor `0600` vlastněný subscription; kontejner byl opět
  obnoven na `clean`. SSH klíče mají datový model, SQLite store a žurnálované
  `ssh key add|list|remove`: klíč se validuje přes `ssh-keygen` na stdin a
  `authorized_keys` se celý přegeneruje s vlastnictvím subscription a právy
  `0700/0600`. Nová subscription vzniká jako `nologin` se zamčeným heslem;
  `ssh set <sub> --access none|key|password|key+password` žurnálovaně nastaví
  shell, generovaný soubor, SQLite stav a případně jednorázové kryptografické
  heslo přes stdin. Klíčový režim bez uloženého klíče je odmítnut. V `pv`
  proběhlo add/list/remove se skutečným ed25519 klíčem i přepnutí
  `none → key → password → none`, včetně ověření shellu a bez vypsání hesla;
  kontejner byl obnoven na `clean`. Cron má nyní `cron list|add|remove`,
  persistenci `cron_jobs`, validaci pětifieldové syntaxe i standardních maker
  a jednorázově přegeneruje artefakt pouze přes `crontab -u <user> -` na stdin;
  command ani comment nemohou obsahovat nový řádek. Jednotkové, SQLite a
  rollback testy prošly v `make test`; v `pv` bylo ověřeno add, list i remove
  nad skutečným `crontab` uživatele subscription, včetně výsledného
  generovaného obsahu. Kontejner byl následně obnoven na `clean`.
- [x] **M6 — SSL:** příprava pro stavový automat Certbotu obsahuje
  konfigurovatelný HTTPS ACME endpoint (`[ssl].server`) a nyní i `ssl status`
  a `ssl deploy-hook`. Status čte expiraci z živého lineage přes `openssl`,
  zatímco hook bezpečně přijímá jen přímý podadresář Certbot live dir,
  aktualizuje známý záznam v SQLite a reloaduje Apache. Je připraven i TLS
  renderer pro PHP-FPM vhost a perzistence `ssl_enabled`/`force_https`.
  TLS rendering a bezpečné HTTP→HTTPS přesměrování s výjimkou ACME nyní platí
  pro PHP-FPM, static, proxy i redirect weby; rendery jsou kryté jednotkovými
  testy. Jednotkové a SQLite testy včetně architektonické kontroly prošly v
  `make test`. Stavový automat `ssl enable`/`disable` je nyní dostupný přes
  CLI: kontroluje enabled web, DNS (s vědomým `--force` pro NAT), ACME HTTP
  404, explicitně sestavené Certbot argumenty, živý certificate lineage a
  následné přepnutí vhostu; `disable` lineage nemaže. DNS a HTTP mají vlastní
  testovací seam. V `pv` byl nyní ověřen celý tok proti lokálnímu Pebble:
  Certbot vydal certifikát pro `ssl.test`, `ssl status` četl živou expiraci,
  HTTPS odpověď prošla, `ssl disable` odstranil TLS konfiguraci a Apache
  configtest zůstal zelený. Test zároveň opravil webroot pod privátním state
  directory, strukturu Aliasu pro skutečný Certbot webroot a proxy nezávislý
  self-check. Převzetí existujících webů a jejich certifikátů patří do
  samostatného M10 `subscription adopt`.
  Prioritní revize pro model „jeden certifikát na projekt“ je zaznamenána v
  [ssl-project-issuance-review.md](ssl-project-issuance-review.md). Před
  automatickým vydáváním při `website create` vyžaduje rozhodnutí o DNS
  readiness; současný bezpečný tok zůstává explicitní `ssl enable`. Preflight
  nyní provádí HTTP ACME self-check pro každý požadovaný SAN hostname, nejen
  pro primární doménu; nefunkční alias proto Certbot nikdy nedostane.
  **Follow-up E3:** po migraci na stabilní project lineage a po atomickém SAN
  reconcile aliasů zopakovat v `pv` Pebble issuance, alias add/remove,
  `certbot renew --dry-run`, deploy hook a Apache reload. Současný `clean`
  snapshot Pebble neobsahuje; test se proto neprovádí při běžném E2 round-tripu.
  Review nyní konkretizuje nutný jednotný resolver lineage pro issuance,
  rendering, status, deploy hook, aliasy, delete a adopt; pořadí datové
  migrace a nevratných Certbot kroků je v `ssl-project-issuance-review.md`.
  `ssl status` nyní otevírá SQLite výhradně read-only a používá stejnou
  stabilní lineage webu jako issuance a rendering; živý soubor Certbotu
  zůstává autoritou pro datum expirace. TLS website alias nyní sestavuje celý
  výsledný SAN seznam, nejdřív aplikuje dočasný HTTP-only candidate vhost a po
  DNS/HTTP preflightu certifikát přegeneruje s jeho stabilním názvem; až pak
  zapíše alias a cache SAN metadata. HTTP website alias zůstává žurnálovaný
  původní cestou. Zbývá Pebble E3 ověření obou směrů včetně skutečné podpory
  odebrání SAN v použité verzi Certbotu.
  Při `website delete` se nyní nejprve odstraní a reloaduje Apache vhost, pak
  se podle per-website metadata odstraní Certbot lineage a jeho cache a až
  nakonec SQLite web; web bez vydaného certifikátu tuto větev nepoužije.
  Permanentní `subscription delete` stejnou posloupnost rozšiřuje na všechny
  vhosty a certificate lineages: každý vhost je odstraněn přes Apache applier
  s configtestem a reloadem, Certbot lineages následují před smazáním SQLite
  certificate metadata.
- [x] **M7 — provoz:** první read-only část `health` je dostupná jako
  `provctl health [<subscription> [<domain>]]` v textu i přes `--json`.
  Kontroluje aktivní Apache, `apachectl configtest`, read-only SQLite spojení,
  enabled vhost, DocumentRoot, PHP-FPM pool a socket, DNS vůči IP serveru a
  HTTP/HTTPS odpověď a živou expiraci certifikátu (`WARN` pod 21 dní, `FAIL`
  pod 7 dní nebo po expiraci); síť a čtení certifikátu mají testovací seam a
  síť nepřebírá systémový proxy server. Úspěšná i chybová cesta jsou pokryty
  offline testy a celý `make test` prošel. Měřenou diskovou kvótu lze nyní při
  vytvoření subscription nastavit přes `--quota-disk 20G`; ukládá se do
  SQLite, `health` ji změří přes `du -sb` a hlásí `WARN` nad 90 % a `FAIL` po
  překročení. Kvóty počtu objektů lze volitelně nastavit přes
  `--quota-websites`, `--quota-databases` a `--quota-backups`; jsou uložené ve
  SQLite a create webu/databáze je před změnou systému vynucuje (nula znamená
  bez limitu). Kvóta záloh bude vynucená spolu s připravovanou implementací
  záloh. Audit JSONL je nyní centrálně připojený k executorům všech
  žurnálovaných produkčních operací; zapisuje pouze aktéra, akci, cíl, stav,
  délku a operation ID, nikdy argumenty, SQL, hesla ani chyby s potenciálně
  citlivým obsahem. Záznam má samostatný test formátu; audit nyní pokrývá i
  přímé `ssl enable|disable` a Certbot deploy-hook. Backup základ nyní
  obsahuje doménový model, SQLite čtení historie (včetně `running` a
  `failed`) a read-only `provctl backup list <subscription>`; zbývá bezpečné
  vytvoření archivu a restore. `provctl backup inspect <subscription> <id>`
  nyní navíc bezpečně ověřuje cestu, `metadata.json`, formát verze 1 a
  SHA256SUMS přes filesystem seam bez spouštění shellu.
  SQLite nyní také atomicky eviduje životní cyklus budoucí archivace
  `running → complete|failed`, včetně výsledné velikosti a času dokončení;
  `backup create <subscription>` nyní vytváří per-subscription zamčený,
  číselně vlastněný `tar.zst` archiv souborů, bezpečný manifest a kontrolní
  součty a dokončuje či označí selhání evidence; kvóta počtu záloh se
  kontroluje před systémovou změnou. Zálohu doplňují konzistentní databázové
  dumpy (`mysqldump` → zstd) bez shellu, zahrnuté v metadatech i checksumách.
  Manifest nyní zaznamenává celý obnovitelný stav subscription — websites,
  databáze, cron jobs a SSH keys — a certificate lineage pouze jako referenci
  pro následné nové vydání; certifikáty se nearchivují ani neobnovují.
  Stále zbývá restore.
  `backup restore <subscription> <id> --dry-run` nyní bezpečně ověří
  manifest, identitu subscription a SHA256SUMS bez jakékoli změny systému;
  mutující fáze obnovy stále zbývá.
  Pro její atomický přesun stagingu existuje testovaný volitelný `FileMover`
  seam, který nerozšiřuje základní FS kontrakt ani nekomplikuje stávající fake
  implementace.
  Evidence backupu umí archiv vyhledat samostatně podle ID, takže clean-server
  restore nebude falešně závislý na existenci původního subscription záznamu.
  SQLite schema v2 nyní při smazání subscription zachová backup záznam s
  uvolněnou vazbou na zdroj; migrační test ověřuje upgrade z v1 i následné
  dohledání archivu. Tím je odstraněna překážka skutečného clean-server
  round-tripu.
  `backup inspect` a `backup restore --dry-run` nyní záměrně vyhledávají
  ověřený archiv přímo podle ID, takže fungují i poté, co zdrojový SQLite
  subscription záznam již neexistuje.
  Lifecycle nyní obsahuje také žurnálovaný `subscription archive <name>`;
  permanentní delete před odstraněním subscription vyčistí navázané vhost
  záznamy a soubory, databázové identity a certificate metadata, aby byly
  cizí klíče konzistentní a clean restore neměl konflikt se starým MariaDB
  stavem.
  Clean-server `backup restore <subscription> <id>` nyní provádí ověřenou
  souborovou obnovu jako jednu žurnálovanou operaci: najde volné UID, vytvoří
  locknutý Unix účet, rozbalí archiv do stagingu na stejném filesystému a
  atomicky jej povýší do nového home před zápisem subscription do SQLite.
  Přepis existující subscription a následná obnova databází či runtime
  artefaktů zatím zůstávají záměrně odmítnuté, dokud nejsou pokryté stejnou
  rollback a round-trip garancí.
  Souborový payload se nyní umí rozbalit explicitním allowlisted `tar` během
  restore do stagingu; test zajišťuje absenci shellu a úklid při selhání.
  Staging lze povýšit jen atomickým `FileMover` přesunem do dosud neexistujícího
  home; test kryje odmítnutí existujícího cíle.
  Databázová část clean-server restore nyní pro každý dump vytvoří novou
  MariaDB identitu s čerstvým heslem, bezpečně rozbalí `.sql.zst` do dočasného
  souboru `0600` a importuje jej výhradně přes stdin `mysql`; do SQLite zapíše
  nový subscription ID. Hesla se zobrazí pouze po úspěchu celé operace, jsou
  řazená deterministicky a nikde se neukládají. Obnova nyní vytvoří a ověří
  také PHP-FPM pool podle uložené verze, obnoví enabled i disabled vhosty,
  cron a veřejné SSH klíče. TLS se záměrně nepřenáší: obnovené vhosty jsou
  HTTP-only bez redirectu a certifikáty je nutné znovu vydat. Jednotkový test
  ověřuje nové SQLite vazby, aliases, cron, SSH přístup i odstranění TLS.
  E2 round-trip nyní prošel v `pv`: po archivaci a permanentním smazání byly
  ověřeny marker soubor, PHP odpověď, databázový řádek, generovaný crontab,
  `authorized_keys`, vlastnictví a `reconcile --dry-run`. Test přitom odhalil
  a opravil pořadí mazání PHP-FPM poolu před `userdel`. Pro zálohy patří mezi
  runtime závislosti `zstd` a pro cron artefakty balíček `cron`; oba jsou
  uvedeny v cookbooku. Zbývá nový lifecycle certifikátů a bezpečný scénář
  přepisu pomocí `--force`; ten je nyní implementovaný a E2 ověřený níže.
  `backup restore --force` nyní před smazáním existující subscription vždy
  vytvoří novou current-state zálohu; teprve po jejím úspěchu spustí stejný
  bezpečný delete lifecycle a clean restore. Když delete selže, chyba vypíše
  ID bezpečně vytvořené zálohy pro obnovu a původní data se nepřepisují.
  Cílené unit testy ověřují pořadí backup → delete, odmítnutí delete při
  selhání backupu a dohledatelné ID při selhání delete. E2 overwrite
  round-trip nyní prošel: static web obnovil původní marker, `backup list`
  po restore ukázal původní i current-state zálohu a `apache2ctl configtest`
  skončil úspěšně; `pv` byl vrácen na `clean`.
  Adresář každé nové zálohy nyní zahrnuje nanosekundy UTC, takže běžná záloha
  a bezprostřední current-state záloha při `restore --force` nemohou sdílet
  stejnou cestu. Regresní test kryje tuto kolizi.
  Jelikož `restore --force` nahrazuje existující subscription, CLI nyní
  vyžaduje i přesný `--confirm-name` a `--yes-i-am-sure`; žádná konfigurace
  ani záloha se bez této dvojité brány neotevře. Cílený CLI test pokrývá oba
  odmítavé stavy.
  E2 přepis static-only subscription odhalil a opravil ještě lifecycle chybu:
  subscription s uloženou výchozí PHP verzí, ale bez PHP-FPM website, nemá
  žádný pool k odstranění. Delete jej nyní odstraňuje jen když alespoň jeden
  web skutečně používá PHP-FPM; unit test kryje PHP-FPM i static-only větev.
  Původní E2 průchod potvrdil obnovení původního markeru a `apache2ctl
  configtest`, ale odhalil, že clean restore zanechával historické backup
  záznamy bez vazby na nový subscription ID. Restore je nyní po zápisu nového
  subscription bezpečně znovu přiřadí pouze z jeho přesného backup rootu;
  sousední cesty zůstávají orphaned. SQLite regresní test kryje obě větve.
  Opakovaný E2 průchod pak reattachment ověřil: `backup list` po forced
  restore vypisuje oba záznamy, marker z původní zálohy je obnoven a Apache
  configtest zůstává zelený. Kontejner byl vrácen na `clean` a běží.
- [x] **M8 — TUI:** návrh je zaznamenán v [tui-design.md](tui-design.md) a
  cíleně přebírá konzistentní Bubble Tea vzor z projektu `depo`: hodnotový
  model, `Deps`, samostatné routing/render/keys/theme a I/O jen přes `tea.Cmd`.
  První read-only subscriptions obrazovka je funkční (`provctl` bez argumentů):
  načítá přes `tea.Cmd`, umí pohyb a refresh a má modelový test. Websites se
  nyní načtou pro vybranou subscription přes service vrstvu. Detail a omezený
  copy-on-write output panel jsou hotové a testované; následují potvrzované
  mutace a vizuální terminálové ověření. První omezená mutace je dostupná:
  vybraný website lze přes `e` potvrdit `y` a asynchronně enable/disable přes
  `tea.Cmd` a service seam; modelový test ověřuje cíl i potvrzovací bránu.
  Service i CLI nyní mají žurnálované `subscription suspend|resume`, které
  TUI používá jako druhou povolenou mutaci: `s` s potvrzením `y` přepíná
  active/suspended přes `tea.Cmd`; modelový test pokrývá správný cíl i stav.
  Klávesa `h` spouští health ve `tea.Cmd` pro vybranou subscription a výsledné
  read-only kontroly posílá do Output panelu; modelový test ověřuje scope i
  zobrazení výsledku. Klávesa `b` nyní přes service seam načte databáze
  vybrané subscription a zobrazí jejich jména v Detailu; tok je krytý
  modelovým testem. Pro vybraný website klávesy `l` a `L` asynchronně načtou
  posledních 100 řádků access, resp. error logu přes read-only service seam a
  zobrazí je v Output panelu; website detail nyní vypisuje také SSL, force
  HTTPS a HSTS stav.
  Srovnání dostupných referencí `examples/branchctl`, `examples/dbctl` a
  aktuálního `depo` je uloženo v [tui-pattern-comparison.md](tui-pattern-comparison.md);
  závěr konkrétně určuje další společné základy (key binding source, themed
  panely, minimální rozměr, async operation slots) bez přenášení nesouvisejících
  vault a deployment funkcí. První společná UX vrstva je nyní hotová:
  `theme.go` drží paletu, View používá přesný čtyřpanelový layout se striktním
  minimem `80×20`, seznamy drží kurzor ve viditelném okně a Detail/Output se
  samostatně scrollují. Detail domény ukazuje subscription, PHP-FPM verzi,
  home, document root, aliases, TLS, force HTTPS, HSTS a případný target.
  Potvrzené mutace mají podle vzoru `branchctl` streamovaný checklist
  skutečných UI kroků („apply generated configuration/state“) s omezeným
  contextem; nevydává se za vnitřní systémový plán, který služba neposkytuje.
  Modelové testy ověřují detail, minimum, čtyři panely a celý progress stream;
  `make test` prošel. Porovnání s `branchctl`, `depo` a `dbctl` vedlo k
  převzetí malého `opSlot` vzoru: každé čtení nyní používá 15sekundový,
  zrušitelný context a generační guard, takže opožděná odpověď nemůže přepsat
  nový výběr. Změna subscription ruší a vymaže závislé weby, databáze, health
  a logy; Output trvale zachová i chyby čtení. Potvrzená mutace okamžitě
  zahodí confirmation snapshot, během běhu ignoruje další akční klávesy a
  `Esc` ruší její context. Testy pokrývají stale odpověď, nahrazený refresh,
  změnu subscription i duplicitní `y`; `make test` znovu prošel. V `pv` byla
  binárka nasazena, bootstrap provedl skutečná data a TUI zahájilo terminálový
  handshake v pseudoterminálu; kontejner byl následně obnoven na `clean`.
  **Follow-up:** před vydáním má člověk provést krátkou vizuální kontrolu přes
  skutečný interaktivní terminál; automatizační Incus TTY zde nepřenáší obraz
  Bubble Tea, pouze handshake sekvence. Následná nezávislá revize vůči
  `branchctl`, `dbctl` a `depo` vedla ještě k ANSI-safe ořezu (včetně Unicode),
  rozlišitelnému rámečku aktivního panelu, zachování historie Outputu při čtení
  logů a zahození závislých dat, když refresh nahradí vybranou subscription;
  nové modelové testy i `make test` prošly.
- [~] **M9 — distribuce:** je přidána deklarace `packaging/nfpm.yaml` pro
  jediný `provctl` `.deb`, config je `noreplace`, šablony jsou běžný obsah a
  balíček deklaruje pouze potřebné Debian závislosti. `scripts/build-deb.sh`
  staví CGO-free binárku s verzí vloženou přes `-ldflags` a předává ji nfpm.
  Maintainer skripty vytvoří pouze cesty vlastněné provctl, spustí explicitní
  `provctl migrate --quiet` a při remove/purge ponechají `/var/www/vhosts` i
  `/var/log/provctl`; nový příkaz `migrate` je krytý CLI testem. `templates/embed.go`
  se do balíčku nekopíruje. CI na každý push instaluje nfpm a lintian, sestaví
  amd64 `.deb`, zkontroluje obsah i control metadata a uloží jej jako artefakt.
  Lokální build s nfpm nyní prošel; manifest používá standardní `dist/provctl`
  a výslovně nastavuje práva binárky, konfigurace a šablon. V `pv` prošel
  `dpkg -i` včetně `postinst` a následný purge zachoval `/var/www/vhosts`.
  Integrace backupu následně odhalila, že balíčku chyběly runtime závislosti
  `zstd` a `cron`; obě jsou nyní explicitní Debian `Depends`, takže instalace
  nemůže úspěšně skončit bez binárek potřebných pro backup a cron lifecycle.
  Lokální fallback verze balíčku také nyní normalizuje netagovaný commit na
  validní Debian tvar `0.0.0+git.<sha>` (a release tagy dál používají verzi
  tagu). Kontrola rozlišuje exact tag od hashe i když hash začíná číslicí;
  `dpkg-deb --field Version` tento tvar ověřil.
  CI nyní spouští `lintian` i `piuparts` proti Debianu trixie; první vzdálený
  běh ještě musí potvrdit. Tag `vX.Y.Z` spouští release workflow, který vytvoří
  `.deb` s verzí `X.Y.Z` a připojí jej ke GitHub Release. V `pv` úspěšně
  proběhl upgrade `0.0.1~local → 0.0.2~local` přes `dpkg -i`; kontejner byl
  obnoven na `clean`. Zbývá stateless APT repozitář.
  Uživatel potvrdil vytvoření GitHub signing secrets; veřejný export je v
  `packaging/apt/provctl.asc`. Replikovatelný postup vytvoření klíčů, nastavení
  secrets, zálohy do trezoru a ověření obnovy je v
  [apt-signing-keys.md](apt-signing-keys.md). Skutečný podpis v CI a obnova
  ze zálohy zatím nejsou ověřeny.
  Release build nyní používá matici `amd64`/`arm64` a publikuje společný
  release až po sestavení obou balíčků. Opraveno předávání `GOOS=linux` a
  `GOARCH` do kompilátoru (dříve `ARCH` měnilo jen metadata balíčku).
  Lokálně sestaveny oba `.deb` verze `0.0.3~local`; `file` potvrdil AArch64
  a x86-64 binárky. Doplněna konfigurace APT kanálů se skutečným signing
  fingerprintem a podrobnější anglický popis balíčku. Samotné sestavení
  a publikace APT indexů zůstávají rozpracované.
  Skript `scripts/build-apt-repo.sh` vytváří oba kanály a architekturní
  indexy, podepisuje `InRelease` i `Release.gpg` a ověřuje je přes `gpgv`
  distribuovaným veřejným klíčem. Izolovaný lokální test s dočasným klíčem
  prošel pro amd64/arm64 i prázdný testing kanál. Oproti ukázce ve specifikaci
  používá `apt-ftparchive`, aby indexoval všechny historické verze, nejen
  nejnovější verzi evidovanou standardním nastavením `reprepro`.
  Zbývá napojení na úplné stažení GitHub Releases, Pages workflow a test
  skutečného APT klienta; produkční podpis dosud nebyl ověřen.
  Workflow nyní navazuje na release stateless stažením všech publikovaných
  `.deb`, podpisem a Pages deploymentem. Downloader stránkuje releases i
  assets, vynechává drafts, řadí prereleases do testing a selže při kolizi
  názvu či neúplném stažení. Tři offline Python testy prošly (stránkování,
  routování a nebezpečné názvy). Vzdálené spuštění a APT klient zatím zbývají.
  Následný lokální `apt-get update` s oddělenými seznamy přijal podepsané
  indexy obou kanálů a `apt-cache policy provctl` našel kandidáta
  `0.0.3~local`. Instalace přes APT v `pv` zatím zbývá. Review doplnilo
  testovací gate před release build a sjednotilo Go ve workflow podle
  `go.mod` (1.24 místo zastaralého 1.22).
  V `pv` následně prošlo `apt-get update` i instalace `provctl=0.0.3~local`
  z lokálního podepsaného repozitáře s explicitním `Signed-By`. Ověřena
  verze binárky a čtení databáze přes `subscription list`; kontejner obnoven
  na `clean` a potvrzen stav RUNNING. Jde o lokální testovací klíč a file
  transport, nikoli důkaz produkčního podpisu nebo dostupnosti GitHub Pages.
  Read-only kontrola GitHub API nyní potvrzuje, že Pages používají workflow
  deployment na `https://jozuekupec.github.io/provctl/`, oba potřebné Actions
  secrets (`APT_GPG_PRIVATE_KEY`, `APT_GPG_PASSPHRASE`) jsou nastavené a
  dosavadní CI běhy jsou zelené. Repo ale zatím nemá žádný GitHub Release;
  produkční podpis, Pages deployment APT obsahu a instalace skutečným APT
  klientem proto čekají na push aktuální větve a první záměrný release tag.
  E1 install/purge nyní prošel také přes `piuparts 1.6.0` v čerstvém Debian
  13 chrootu uvnitř `pv`; log končil `PASS: All tests`. Reprodukovatelný
  `scripts/tests/t05-piuparts.sh` instaluje E1 nástroje pouze do `pv`, testuje
  konkrétní `.deb` a přes `trap` vrací `clean` snapshot. Cookbook již
  nepoužívá přepínač `--warn-on-leftover-files`, který aktuální piuparts
  nepodporuje.
  T06 je obdobně opakovatelný přes `scripts/tests/t06-piuparts-upgrade.sh`;
  upgrade `0.0.3~local → 0.0.4~local` proběhl v čistém trixie chrootu bez
  chyby a instance se následně vrátila na `clean`. Ručně upravený conffile
  zůstává samostatnou explicitní E2 kontrolou, protože ji piuparts nepokrývá.
- [~] **M10 — migrace:**
  První integrační běh v `pv` odhalil rozpor: Debian balíček instaloval
  `/etc/logrotate.d/provctl`, ale bootstrap očekával jiný obsah a odmítal jej.
  Bootstrap nyní používá totožný balíčkový obsah; jeho no-op a změnové testy
  prošly. Následný čistý HTTP běh v `pv` ověřil instalaci balíčku, bootstrap,
  atomický přesun legacy document rootu, PHP-FPM pool, odpověď Apache
  s očekávaným obsahem a `apache2ctl configtest`; kontejner byl vrácen na
  `clean`.
  Před adopcí se kontroluje aktivní konfigurace přes `apache2ctl -S` včetně
  aliasů a wildcardů. Kolize vyžaduje explicitní vypnutí původního vhostu;
  cizí konfiguraci nástroj automaticky nepřepisuje. Parser pokrývá jednotlivý
  vhost i name-based výpis; reálný výpis je nutné ověřit v integračním testu.
  Malformed wildcard pattern se nyní odmítá fail-closed, aby parser nikdy
  neprohlásil nečitelnou legacy konfiguraci za bezpečnou; cílený unit test
  pokrývá tento guard.
  SAN převzatého certifikátu se přenášejí jako aliasy webu do Apache i DB.
  Před změnami se ověřují doménová pravidla a konflikt s již spravovanými
  doménami; wildcard není v této HTTP-01 adopci podporován. Service race
  test ověřuje zachování aliasu. Reálný `pv` test s aktivním legacy vhostem
  pro stejnou doménu potvrdil odmítnutí před přesunem dat a bez vytvoření
  subscription či provctl vhostu; kontejner byl vrácen na `clean`. TLS/Pebble
  větev stále zbývá.
  Schválené zachování původního Certbot lineage má podporu v repository:
  `CreateWebsite` zachová validovaný explicitní `CertificateName`, pro nové
  weby zůstává default `provctl-site-<id>`. Test pokrývá round-trip přes
  resolver i seznam, konflikt sdíleného lineage a odmítnutí nebezpečné cesty.
  Napojení adopce na zachování TLS zůstává rozpracované.
  Doménová validace nyní rozlišuje bezpečný původní název od existujícího
  guardu pro mazání `provctl-*` certifikátů. Testy domain, SQLite i service
  s race detektorem prošly. Zachování názvu samo o sobě ještě neaktivuje TLS;
  zbývá certifikát ověřit, převzít metadata a vyřešit jeho životní cyklus.
  Vyhledávání certifikátů nově čte SAN přímo z `live/<lineage>/cert.pem`
  a nezávisí na poli `domains` v renewal konfiguraci. Test se skutečným
  lokálně vytvořeným X.509 certifikátem ověřil alias, cizí doménu a chybný
  PEM. Nejde zatím o kontrolu platnosti, klíče či kompletní TLS adopci.
  Následně doplněna kontrola časové platnosti, shody `cert.pem` s listovým
  certifikátem ve `fullchain.pem` a párování privátního klíče přes
  `tls.X509KeyPair`. Testy service s race detektorem prošly včetně odmítnutí
  vadného privátního klíče. Důvěra vydavatele ani úplná TLS adopce tím nejsou
  ověřeny; metadata platnosti a vydavatele jsou připravena pro převzetí.
  Adopční plán nyní přebírá jediný jednoznačný lineage do webu, generuje
  HTTPS vhost a zapisuje metadata certifikátu s vazbou na website ID.
  Více nalezených certifikátů odmítá před změnami. Service test ověřuje
  zachování názvu a vazby metadat; zbývá review obnovy/mazání převzatého
  certifikátu a reálný kontejnerový test TLS adopce.
  Review mazání je nyní zapracované: migrace SQLite `0006` ukládá explicitní
  vlastnictví certifikátu. Nově vydané lineage jsou `managed`, ale převzaté
  jsou vždy cizí bez ohledu na svůj historický název. Mazání webu nebo
  subscription proto odstraní Certbot lineage jen pro `managed` záznam;
  převzatý certifikát a jeho živé soubory zůstanou zachované, odstraní se jen
  provctl metadata. Cílené service a SQLite race testy prošly.
  Obnovovací větev má nyní v journalu explicitní recovery hranici až po
  úspěšném vytvoření systémových artefaktů a SQLite záznamů. Selže-li až
  následný `certbot renew --dry-run`, plán obnoví zachycenou renewal konfiguraci,
  ale data adopce záměrně ponechá a operaci označí `inconsistent` pro ruční
  dokončení. Selhání před hranicí se dál standardně vrací rollbackem. Nový
  unit test executoru a service race test ověřují obě vlastnosti.
  Review změnilo obnovu při adopci na Certbot `reconfigure` místo `certonly
  --keep-until-expiring`, které mohlo vydat nový živý certifikát. Regresní
  test ověřuje přesné argumenty bez změny SAN; service race testy prošly.
  Reálná ACME testovací obnova a rollback renewal konfigurace ještě zbývají.
  Rollback nyní zachycuje původní renewal soubor a jeho oprávnění před
  reconfigure. Obnovuje jej při selhání příkazu i při rollbacku po selhání
  následného dry-run. Test ověřuje obnovu obsahu/oprávnění a zapojení do
  chybové cesty adopce. Záloha je zatím v paměti probíhající operace,
  ne trvalá ochrana proti pádu procesu; tato část recovery zbývá.
  Následně doplněna trvalá záloha před změnou renewal konfigurace v
  `/var/lib/provctl/renewal-backups/<lineage>/<timestamp>-<nonce>/`, včetně
  původní cesty a oprávnění pro ruční obnovu. Test ověřuje původní obsah
  na disku. Automatická obnova po pádu a retence záloh nejsou implementovány.
  Návrh je v
  [subscription-adopt-design.md](subscription-adopt-design.md). Určuje jednu
  žurnálovanou operaci, přesný cíl document rootu, defaultní atomický přesun,
  volitelnou kopii, rollback hranice a povinné převzetí renewal lineage.
  Implementace nyní přidává `provctl subscription adopt <name> --from <path>
  --domain <domain>` v jednom žurnálovaném plánu: výchozí atomický přesun,
  opt-in `--copy`, výchozí archivní záloha, PHP-FPM/Apache artefakty a zápisy
  SQLite až po konfiguraci systému. Certbot renewal inspector je samostatný
  seam; selhání jeho finálního dry-run označí operaci `inconsistent`. Jednotkové
  testy pokrývají cíl, přesun, vlastnictví, SQLite pořadí a renewal selhání.
  Převzatá lineage je bezpečně propojena s TLS vhostem, aliasy a metadaty;
  reconfigure zachovává živý certifikát. Zbývá pouze reálné Pebble ověření
  konfigurace renewal a TLS větve.
  Incus E2 ověření s kopií legacy webrootu prošlo: uživatel, atomický přesun,
  archivní záloha, PHP-FPM pool, Apache `configtest` a HTTP odpověď (`200`) byly
  ověřeny a `pv` byl vrácen na `clean`. Zbývá Pebble větev pro skutečný Certbot
  renewal config a TLS-lineage sjednocení.

## Pravidla ověřování

Unit a golden testy běží neprivilegovaně přes `make test`. Integrační ověření
probíhá jen v Debian 13 kontejneru `pv`, nikdy na hostiteli; návrat do čistého
stavu provede `incus snapshot restore pv clean`. Před uzavřením každého milníku
se do tohoto souboru doplní rozsah ověření a případná odchylka od specifikace.
Pro opakovatelné ruční E2 scénáře je nyní k dispozici omezený helper
`scripts/e2.sh`: umí zobrazit stav `pv`, vrátit pouze jeho `clean` snapshot,
nahrát jeden fixture do `/root` a spustit výslovný testovací příkaz jen uvnitř
kontejneru. Syntaxe helperu i celý bezpečný tok `reset → push → sh → reset`
pro aktuální `.deb` proti běžícímu `pv` byly ověřeny; postup je v
[testing-cookbook.md](testing-cookbook.md).

T10 je nyní zapsaný jako opakovatelný `scripts/tests/t10-isolation.sh`. Test
vytvoří dvě subscriptions a PHP-FPM weby a ověří oddělení shellu, PHP
`open_basedir`, session, privátních cest HTTP i Apache logů; `trap` vždy
obnoví `pv clean`. Při jeho prvním reálném běhu se ukázal nesoulad v průchodu
kořenovým logovým adresářem. Bootstrap proto migruje známý starší režim
`/var/log/provctl` z `0750` na `0751` (`root:adm`): uživatel může projít jen
ke známé vlastní cestě, ale obsah kořene nevylistuje. Unit test pokrývá
aplikaci i rollback této úzké migrace. Balíček
`provctl_0.0.0+git.78689e3_amd64.deb` a celý T10 scénář proti `pv` prošly;
po běhu byl ověřen návrat instance na snapshot `clean`.

T04 má nyní neprivilegovaný `scripts/tests/t04-package.sh`: kontroluje control
metadata, conffile, zákaznická data mimo balíček, požadované cesty a jejich
práva i velikost artefaktu. Pro `provctl_0.0.0+git.78689e3_amd64.deb` prošel
bez instalace; tím je ruční package-checklist z cookbooku opakovatelný.

Poslední integrační ověření (2026-08-30): bootstrap vytvořil požadované cesty
včetně práv, `apachectl configtest` a reload uspěly a druhý běh byl beze změn.
Následný `doctor` potvrdil provctl, Apache, PHP-FPM a Certbot; testovací obraz
zatím nemá funkční MariaDB socket autentizaci a obsahuje dva certbot renewal
mechanismy. Nejde o změnu provctl a kontejner byl následně obnoven na `clean`.
Ve stejný den proxy website úspěšně předala odpověď z lokálního upstreamu a
redirect website vrátila očekávané `302` a `Location`; i po tomto testu byl
kontejner obnoven na `clean`.
