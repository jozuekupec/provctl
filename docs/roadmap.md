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
  Reprodukovatelný T16 (`scripts/tests/t16-ssl.sh`) nyní sestaví upstream Pebble
  na hostu, spustí jej pouze uvnitř `pv` a ověří skutečné HTTP-01, renew,
  deploy-hook, force-HTTPS výjimku pro ACME a disable. Harness má explicitní
  kontrolu PID, síťové timeouty a `--noproxy '*'` pro čistě lokální management
  endpoint; úplný opakovatelný běh zůstává E3 follow-upem před uzavřením SAN
  reconcile větve.
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
  enabled vhost, DocumentRoot pouze pro static/PHP-FPM web, PHP-FPM pool a socket, DNS vůči IP serveru a
  HTTP/HTTPS odpověď a živou expiraci certifikátu (`WARN` pod 21 dní, `FAIL`
  pod 7 dní nebo po expiraci); síť a čtení certifikátu mají testovací seam a
  síť nepřebírá systémový proxy server. Úspěšná i chybová cesta jsou pokryty
  offline testy a celý `make test` prošel. Release revalidace opravila falešný
  DocumentRoot `FAIL` pro proxy a redirect weby; cílený race test pokrývá oba
  bez-root typy. Měřenou diskovou kvótu lze nyní při
  vytvoření subscription nastavit přes `--quota-disk 20G`; ukládá se do
  SQLite, `health` ji změří přes `du -sb` a hlásí `WARN` nad 90 % a `FAIL` po
  překročení. Kvóty počtu objektů lze volitelně nastavit přes
  `--quota-websites`, `--quota-databases` a `--quota-backups`; jsou uložené ve
  SQLite a create webu/databáze je před změnou systému vynucuje (nula znamená
  bez limitu). TUI nyní tyto čtyři kvóty přijímá již při vytvoření subscription
  (disk ve formátu `20G`, `500M` nebo bytech; prázdné pole znamená bez limitu)
  a zobrazuje je v detailu subscription; úprava existující kvóty zůstává
  záměrně mimo aktuální rozsah. Kvóta záloh bude vynucená spolu s připravovanou implementací
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
- [x] **M8 — TUI:** původní čtyřpanelový prototyp je funkční, ale po ruční
  kontrole se ukázal jako UX nedostatečný: layout může oříznout horní řádek,
  navigace, nápověda, filtry a potvrzení neodpovídají referenčnímu `branchctl`.
  Schválený nástupnický návrh je v
  [tui-redesign-proposal.md](tui-redesign-proposal.md): fullscreen subscription
  picker, workspace jedné subscription, přesný layout, šipková navigace,
  modální formuláře a ANSI-safe confirm overlay. Následující text popisuje
  dosavadní implementovaný základ, který redesign nahradí nebo znovu použije.
  **Redesign block 1 is complete:** startup now renders an exact full-screen
  subscription picker; `Enter` opens a subscription workspace with metadata
  and domains on the left and Detail, Logs, and Output on the right. The panel
  renderer owns its outer dimensions instead of relying on Lipgloss height
  composition, preventing the observed clipped top row. Workspace geometry is
  covered at 101x28, including exact width for every row and exact terminal
  height; the minimum terminal is now 80x24. `make test` passed after this
  change. Filtering, help, modal confirmation, forms, and new mutations remain
  in the next redesign blocks.
  **Redesign block 2 complete:** subscription and domain filters use Bubble Tea
  text input and derive selection, rendering, and service targets from the
  same filtered list. `Esc` clears an applied domain filter before leaving the
  workspace, and active inputs now return their Bubble Tea command and blur on
  completion. A model test proves that an enable/disable action targets the
  filtered domain without mutating the source list.
  **Redesign block 3 complete:** `?` opens a centered, ANSI-safe Help overlay
  with scroll and shortcut filtering; help owns keyboard input until closed.
  Confirming a domain toggle or subscription state change is now a centered
  modal dialog rather than a fragile status-line prompt. Its modal key routing
  prevents background actions, has an explicit `y` confirmation rule, and any
  other key cancels. The concise one-row keybar only advertises currently
  implemented operations. Full `make test` (vet, staticcheck, race tests)
  passed after this block.
  **Redesign block 4 complete:** the picker and Subscription panel now expose
  the implemented subscription lifecycle rather than advertising placeholder
  actions. `a` archives with a modal confirmation; `d` only permits permanent
  deletion of an archived subscription and requires typing its exact name.
  The action remains service-backed and uses the existing journalled deletion
  path. Left/right now traverse Subscription → Domains → Detail → Logs →
  Output and back, so subscription actions are reachable from the workspace.
  Model tests cover archive, typed deletion, and panel navigation; `make test`
  passed.
  **Redesign block 5 complete:** selected domains can now toggle TLS through
  `t`. The confirmation dialog clearly states that certificate issuance needs
  public DNS and HTTP reachability; the actual operation calls the existing
  SSL service with its DNS/preflight, Certbot and renewal safeguards rather
  than duplicating them in the UI. Disabling TLS retains the certificate as the
  CLI does. The production dependency is wired only through `Deps` and runs
  through a Bubble Tea command; the model test verifies its target and enable
  state. `make test` passed. A real public-domain issuance remains an E3/E5
  follow-up, not a test to perform against the local `pv` container.
  **Redesign block 6 complete:** Help now uses a fixed, bounded, ANSI-safe
  popup with scroll overflow markers and a footer that remains visible on the
  minimum 80×24 terminal. Its filter lives inside the dialog, hides sections
  without matches, and follows a three-step Escape flow (finish typing, clear
  an applied filter, close). Subscription and domain filters now have an
  accent-coloured input row while typing, `visible/total` match counts, a
  persistent applied-filter indicator, and the same count in the panel title.
  Filtering subscriptions from the workspace and clearing either focused
  filter with Escape are covered by model tests. This deliberately adopts the
  current popup and filtering guidance pulled from `branchctl` and the Go TUI
  cookbook. `make test` and `make build` passed.
  **Redesign block 7 complete:** a selected PHP-FPM domain now exposes `p` for
  a bounded PHP-FPM version picker. It discovers only versions installed on
  the host via a cancellable `tea.Cmd`, labels their service state, and never
  accepts a free-text version. Selecting a new version opens the standard
  confirmation dialog; confirming calls the existing journalled PHP service
  through `Deps`, replaces only that domain's pool and vhost, then refreshes
  the domain list. Choosing the current version performs
  no mutation. PHP-FPM artifacts are now domain-scoped everywhere: creation,
  adoption, restore, health checks, deletion and the CLI use an independent
  pool, socket and error log. The pool name is deliberately separate from the
  subscription Unix user, so a domain name can never be mistaken for an OS
  account. In `pv`, Sury PHP-FPM 8.1, 8.2 and 8.3 were installed beside 8.4;
  `demo.test` was switched to 8.3 and `api.demo.test` to 8.2. Both sockets,
  pool files, Apache handlers and `apache2ctl configtest` were verified. The
  saved `php-per-domain` snapshot preserves that test state; `pv` was restored
  to `clean` after the integration run. The health command correctly reports
  the per-domain FPM check as OK; its existing HTTP 403/DNS warning for the
  fixture is unrelated to the PHP transition. Model tests cover discovery,
  selection, confirmation and the no-op guard; focused renderer, service,
  SQLite, TUI and CLI tests passed; later complete `make test` runs also
  verified the race phase before release.
  **Redesign block 8 in progress:** subscription-level metadata and list rows
  no longer present PHP-FPM as a subscription property; it is shown only on a
  selected domain. The PHP version picker marks and initially selects the
  current version for that domain, independently of whether each installed
  PHP-FPM service is active. Running mutations now render their checklist in
  a centred progress popup; Output remains the persistent operation log.
  The workspace now follows the focused-panel sizing pattern used by
  `branchctl`: Detail sits below the auto-sized Domains panel and has half the
  left column at rest; Logs and Output split the right column equally. Focusing
  Domains, Detail, Logs, or Output expands that panel while retaining a useful
  minimum height for its sibling. Geometry tests preserve the exact terminal
  frame and these sizing invariants.
  Progress overlays now preserve the originating panel's focus through both
  completion and failure instead of moving focus to Output; Output remains a
  passive operation history. TLS progress also correctly marks a failed SSL
  service call as failed in the checklist.
  The TUI now has a `,` Settings popup, modelled after branchctl's focused
  configuration editor. It exposes every supported config key in section tabs,
  saves atomically while preserving the rest of `config.toml` (including
  comments and unknown administrator keys), and keeps TLS settings available
  immediately; other service runtimes reload on the next TUI start. The popup
  uses the shared class/fit model: Settings is a large fixed surface with a
  permanent footer, while short confirmations retain automatic sizing. Model,
  configuration round-trip, and fixed-popup geometry tests cover the contract.
  The settings scopes now use the connected bordered tabs from `branchctl`:
  the active scope has an open bottom edge and visually joins the form below;
  compact `DB` keeps the complete tab strip inside the 80-column minimum.
  A renderer test protects its width and border height.
  **Redesign block 9 complete:** editable Settings path fields now share an
  asynchronous file explorer derived from the cookbook/dbctl reference. It
  keeps direct typing available while `Enter` opens a large bounded picker;
  directories are opened with Enter, `Space`/`Alt+Enter` opens a separate
  confirmation, and accepted values are absolute. The browser lists through a
  dependency seam, so the value model performs no filesystem I/O, and model
  tests cover navigation, selection and terminal-frame geometry. No editable
  domain document-root form exists yet; when the planned domain create/edit
  form is added, it must reuse this picker and retain service-layer boundary
  validation rather than treating the UI picker as authorization.
  **Redesign block 10 in progress:** the required service foundation for the
  future domain form is now present. `website docroot set` builds a journalled
  operation which rejects traversal, missing paths, non-directories and any
  path escaping the resolved subscription home. It explicitly does not move
  data; it renders and applies the Apache replacement before writing SQLite,
  with rollback to the previous vhost/root. Focused service, SQLite and CLI
  tests cover the path validation and persisted state. The next part is the
  TUI domain create/edit form, including reuse of the picker for its root.
  The first edit capability is complete: capital `E` over a static or PHP-FPM
  domain opens a bounded document-root form; Enter opens the shared picker,
  Ctrl+S follows the standard confirmation, and the existing progress popup
  reports Apache apply plus refresh without changing focus. Its model tests
  cover modal flow, service target, refreshed result and exact terminal width.
  Domain creation is now available with `n` in the Domains panel: its modal
  selects PHP-FPM, static, proxy or redirect and conditionally exposes a
  target only for proxy/redirect. It uses the established confirmation and
  progress pipeline and refreshes domains in place. Static/PHP-FPM creation
  deliberately retains the safe default root; an administrator can then use
  the explicit `E` root operation. Full editing of aliases, target and
  redirect policy still follows. Alias editing is now available: `a` adds and
  `A` removes an alias through a small confirmation form. Non-TLS websites use
  the journalled WebsiteService operation; TLS websites deliberately route to
  Certbot's complete-SAN reconciliation, preventing a database-only alias
  change from leaving a certificate stale. A model test covers the service
  target and refreshed aliases.
  The service foundation for proxy and redirect editing is now journalled and
  tested: it renders and applies the replacement Apache vhost before persisting
  `target`/redirect code, with the renderer retaining host, port, URL and
  redirect-code validation. The TUI form is now available via `T` for selected
  proxy/redirect domains, with redirect code toggled between 301 and 302 and
  the shared confirmation/progress flow.
  A selected domain can now be deleted with capital `D`. The TUI requires the
  exact domain acknowledgement used by the CLI and explicitly describes that
  it removes generated configuration (and managed TLS data) while preserving
  site data and logs. The service remains the only mutation boundary and the
  domain list is refreshed after the progress popup completes.
  **Redesign block 11 in progress:** the workspace no longer treats the
  Subscription metadata panel as focusable; the fullscreen picker is the sole
  selection boundary and `s` returns there from the normal workspace. Detail
  is again read-only, so it no longer presents database, SSH, cron, or backup
  rows as if they were directly editable. Those controls will move into the
  planned subscription administration surface. Picker rows now asynchronously
  combine persisted quotas with read-only live usage: active and disabled
  website counts plus `du -sb` disk usage. A failed disk measurement leaves
  the subscription usable and displays an unknown value. Domain rows carry a
  visible type tag (`[php-fpm]`, `[static]`, `[proxy]`, `[redirect]`). Focused
  model/service tests and full `make test` passed.
  `Enter` over a selected domain now opens the next safe editor slice: a
  full-screen surface with connected Overview, Content, Runtime, TLS, Routing
  and Logs tabs. Each tab exposes only its related existing operation, while
  the shared confirmation/progress flow returns to the same editor. `Esc`
  returns to the workspace and `s` to the picker. The historical Detail-based
  administration text below is superseded: database, SSH, cron and backup
  history stay read-only until each has a separately designed safe modal
  workflow; they do not turn Detail back into an editable control surface.
  The editor tab strip uses branchctl's ANSI-aware horizontal joining instead
  of concatenating multi-line styled strings. `Shift+Left` and `Shift+Right`
  now change tabs; ordinary arrows do not. A minimum-terminal geometry test
  covers the tab width and every rendered frame row.
  Shortcut declarations are now centralized in `internal/ui/bindings.go`.
  Each row binds an action to contexts and supplies both its Help and keybar
  forms; picker, workspace, and domain-editor routing consume the same action
  identity. Regression tests ensure every advertised key resolves and
  keybar/help remain derived from the shared registry. Modal forms retain only
  their widget-local input handling.
  The first separate fullscreen **Manage subscription** surface is now present
  behind `m` in the Domains panel, so the read-only Detail panel is not used as
  a hidden administration UI. Its connected tabs follow the domain editor's
  `Shift+Left`/`Shift+Right` convention. The Databases tab asynchronously loads
  its list, supports `n` creation, `p` one-time password rotation, and typed
  `D` deletion through the existing journalled services; all actions use the
  shared confirm/progress pipeline and refresh the list where needed. Its
  bindings, keybar and context help are entries in the same shortcut registry.
  The SSH tab now follows the same boundary: it asynchronously lists persisted
  keys, `n` opens the existing direct-path/file-picker add form, and typed `D`
  rewrites `authorized_keys` through the journalled SSH service. Its list keeps
  its own cursor and the key's public fingerprint is never read from the
  account file by the UI. The Cron tab now loads persisted jobs with its own
  cursor. `n` presents a bounded schedule/command/comment form, and typed `D`
  removes the selected numeric job ID; both paths use the existing journalled
  crontab service and the same progress popup. The model regression covers
  exact propagation of the optional comment, so it cannot be silently lost.
  The Backups tab now asynchronously shows persisted archive history and `n`
  confirms creation through the existing journalled backup service before
  refreshing the list. Restore deliberately remains CLI-only: its force mode
  can replace a subscription and may reveal newly generated database passwords,
  so it needs a dedicated double-confirmation and one-time-secret design rather
  than a shortcut. Model tests cover loading, rotation, secret non-persistence,
  typed deletion and confirmed backup creation; full `make test` passed.
  A follow-up shortcut review caught an administration-only regression: shared
  `,` Settings and `r` Refresh bindings were advertised by its keybar but had
  no router cases. Both now work in every administration tab; the model test
  executes the advertised actions rather than only checking registry lookup.
  The subscription workspace also exposes `R` for a confirmed, service-backed
  reconciliation of generated Apache configuration. It keeps the scope to the
  selected subscription, reports the no-drift result without implying a write,
  and refreshes its domain list after a real operation.
  The existing `b` database inspection now opens a real `Databases` detail
  view rather than invisibly loading data behind the selected-domain detail.
  It renders each persisted database's name, user, host and charset; moving to
  another domain restores the domain detail, so stale database output cannot
  be mistaken for that domain's state.
  Database creation is now also available from that detail with `n`: a modal
  accepts the safe local suffix and optional subscription-owned credentials
  file, uses the journalled database service, shows the generated password in
  the existing one-time secret popup, and refreshes the list. Password rotation
  and deletion remain CLI-only until the detail list gains an explicit selected
  database cursor and a destructive typed-name confirmation.
  Help is now context-sensitive: opening `?` from a database, SSH-key, cron,
  or backup detail displays only the shortcuts meaningful to that panel, rather
  than the prior global list that advertised unavailable actions.
  Detail-derived lists now have their own selected row (`↑/↓`), rather than
  borrowing scroll-only behavior; SSH key creation follows the common `n`
  convention instead of the former special `+` key. This is the shared base
  for selected-item deletion and inspection in databases, SSH, cron, and
  backups.
  Read-only SSH key inspection is now available through `K`, using the SSH
  service's persisted metadata rather than reading `authorized_keys` directly.
  Its dedicated detail view exposes the public fingerprint and comment, and is
  protected by the same cancellable load slot as the other derived views.
  Read-only cron inspection follows the same pattern through `c`: the detail
  view shows each persisted job ID, schedule, command and optional comment.
  This remains a database-derived view; the TUI does not parse or manipulate a
  user's generated crontab directly.
  Backup history is now available through `V`. It loads only the persisted
  subscription archives and shows ID, completion status, size and start time;
  inspection and restore remain explicit CLI workflows until their destructive
  confirmation and archive-verification UX has a dedicated design.
  A follow-up UI review reset every derived detail view on subscription change,
  preventing an empty `SSH keys`, `Cron jobs` or `Backups` title from leaking
  into a newly selected subscription. The Help popup also now groups the
  domain-scoped PHP action with the other domain actions.
  Subscription editing now includes `u` for SSH access (`none`, `key`,
  `password`, `key+password`) through the existing journalled SSH service.
  Password-bearing modes use a dedicated one-time acknowledgement popup and
  never append the generated password to Output; a TUI model test verifies
  that non-leakage contract. Key modes retain service validation requiring a
  registered public key.
  The same shared file explorer now drives `+` in the SSH-key detail: the
  administrator can type a key-file path or browse it, then the SSH service
  reads and validates the key through its filesystem seam. The selected path
  becomes absolute when accepted from the picker; TUI tests cover that handoff
  and the refresh after the journalled add operation.
  The current manual-test package `0.1.1~dev.17.ssh-key-picker` was installed into
  the isolated `pv` Debian 13 container after its version and fixture
  subscriptions (`demo`, `staging`) were verified. Snapshot `tui-ssh-key-picker`
  preserves this exact interactive test state; `clean` remains the untouched
  baseline for privileged integration scenarios.
  Subscription creation is now also available in the fullscreen picker through
  `n`: the bounded form accepts the derived subscription name and preserves the
  existing safe service defaults for user identity, home and unlimited quotas.
  It follows the same confirmation/progress path as every other write and
  refreshes the picker after completion. Subscription metadata editing remains
  deliberately unimplemented because the service/CLI has no coherent update
  operation to expose yet; it must not become a DB-only TUI shortcut.
  Původní návrh je zaznamenán v [tui-design.md](tui-design.md) a
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
  Pro ruční vizuální kontrolu je nyní připravený `pv` snapshot `tui-ready`:
  obsahuje balíček `0.1.0`, dokončený bootstrap, subscriptions `demo` a
  `staging` a PHP-FPM/static demo weby. Spouští se přes `incus exec` v reálném
  terminálu; po kontrole lze stav bezpečně obnovit ze snapshotu `clean` nebo
  znovu otevřít `tui-ready`.
  Databáze, cron a backup history jsou záměrně read-only, dokud pro jejich
  mutace nevznikne stejně bezpečný modalní návrh. Reálná TUI reprodukce
  2026-09-13 opravila file browser: `ansi.TruncateLeft` přijímá počet
  odstraněných buněk, nikoli cílovou šířku, a proto skrýval krátké cesty i
  názvy. Prohlížeč nyní správně zobrazuje adresář i položky; regresní test a
  nově nasazený balíček v `pv` to ověřují.
  **Statické review 2026-09-16:** produkční `internal/ui` neobchází `Deps`
  přímým filesystemovým, procesovým, SQLite ani systémovým přístupem;
  `go test ./internal/ui ./internal/arch -race -count=1` ověřil routing a
  package boundaries. Zkratkový registry test dále potvrzuje, že každá
  inzerovaná normální akce má router.
  Pro opakovatelnou ruční kontrolu nyní
  `scripts/dev/run-tui-test.sh` jediným příkazem obnoví izolovanou fixture
  `pv-tls-debug-20260913/tui-ssh-key-picker`, sestaví aktuální `.deb`,
  nainstaluje jej a otevře TUI v terminálu volajícího. Postup je popsán v
  [testing-cookbook.md](testing-cookbook.md#ruční-tui-smoke-test-jedním-příkazem).
  **Ruční acceptance 2026-09-16:** aktuální build `0.0.0+git.7345962` byl
  nasazený do této fixture a uživatel v reálném terminálu potvrdil správný
  layout i ovládání TUI. M8 je tím uzavřený. Destruktivní `backup restore`
  zůstává úmyslně samostatným CLI workflow, nikoli chybějící TUI funkcí.
- [x] **M9 — distribuce:** je přidána deklarace `packaging/nfpm.yaml` pro
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
  Před tímto pushem `go mod tidy` odhalil a opravil metadata dvou přímo
  importovaných Bubble Tea závislostí (`lipgloss`, `x/ansi`); CI už po tidy
  nemá měnit `go.mod`.
  E1 install/purge nyní prošel také přes `piuparts 1.6.0` v čerstvém Debian
  13 chrootu uvnitř `pv`; log končil `PASS: All tests`. Reprodukovatelný
  `scripts/tests/t05-piuparts.sh` instaluje E1 nástroje pouze do `pv`, testuje
  konkrétní `.deb` a přes `trap` vrací `clean` snapshot. Cookbook již
  nepoužívá přepínač `--warn-on-leftover-files`, který aktuální piuparts
  nepodporuje.
  T06 je obdobně opakovatelný přes `scripts/tests/t06-piuparts-upgrade.sh`.
  Jeho název je historický: `piuparts` neumí ověřit upgrade dvou lokálních
  `.deb`, protože je nemá v APT cache. Wrapper proto provádí skutečný
  `dpkg -i` upgrade v `pv`, změní `config.toml` a ověří zachování komentáře i
  `vhosts = "/data/web/vhosts"`; samostatná reprodukce potvrdila obě hodnoty
  před upgradem i po něm. `piuparts` zůstává přesně vymezený na T05
  install/purge.
  Nový `scripts/tests/run-all.sh` spouští implementované T04, T05, volitelné
  T06 a T10 postupně a při selhání předává skutečný nenulový návratový kód
  etapy. Úplný běh s `0.0.5~local` a předchozím `0.0.4~local` skončil kódem
  0; všechny čtyři etapy vypsaly `PASS` a `pv` se po závěrečném restore vrátil
  do stavu `RUNNING`.
  Read-only kontrola 2026-09-12 potvrdila, že lokální `main` je synchronní s
  `origin/main` a vzdálený repozitář stále nemá žádný tag. Další release
  brána je proto pouze uživatelem zvolený první tag `vX.Y.Z`; až ten může
  spustit produkční podpis a Pages deployment.
  První CI běh po pushi odhalil, že aktuální `ubuntu-latest` (Noble) nemá
  instalační kandidát pro `piuparts`; package job proto končil před buildem
  kódem 100. Kontrola install/purge proto běží v samostatném Debian 13 Docker
  kontejneru a stahuje artefakt z package jobu. Přímý GitHub Actions
  `container:` job selhal při piuparts mountu `/proc` kvůli chybějící
  `CAP_SYS_ADMIN`; workflow nyní používá pouze pro tento izolovaný chroot
  `docker run --privileged`. Lokální běh i vzdálený CI běh `34653180305`
  (commit `9af00a0`) úspěšně ověřily build, lintian a Debian 13 piuparts.
  Kontrola po TUI blocích 2026-09-13 potvrzuje zelené běhy `34744196491`
  (`64521dd`) a `34744244556` (`e912201`): oba dokončily package build,
  lintian, `go vet`, staticcheck, race-enabled testy i Debian 13 piuparts.
  Starší běh `34743203661` selhal pouze na třech staticcheck nálezech
  (nepoužité popup konstanty a nevyužitý výsledek v UI testu); jejich oprava
  je součástí pozdějších zelených běhů, nejde tedy o aktuální CI blokér.
  Lokální `v0.1.0` tag míří na starší commit a nikdy nebyl publikován. První
  veřejný release proto bude `v0.1.1` z aktuálního zeleného `main`; jeho tag
  je zároveň produkční test signing secrets, GitHub Release a Pages APT
  deploymentu. **M9 completed 2026-09-13:** workflow `34744458540` dokončil
  testy, amd64/arm64 build, GitHub Release i podpis a deployment APT obsahu
  na Pages. Veřejný klientský test v novém dočasném Debian 13 Incus kontejneru
  ověřil fingerprint, kandidáta `0.1.1` z
  `https://jozuekupec.github.io/provctl/debian` a úspěšně nainstalovaný
  `provctl --version` `0.1.1`; kontejner byl po testu smazán.
- [x] **M10 — migrace:**
  Statická revize TLS adopce 2026-09-13 odstranila Pebble blokér: renewal
  manager dříve vždy volal `certbot renew --dry-run`, který Certbotu pro
  lokální ACME server přepíše directory na veřejný staging endpoint. Při
  explicitním `ssl.server` proto nyní používá skutečný `--force-renewal
  --no-random-sleep-on-renew`; produkční konfigurace bez override zůstává na
  bezpečném `--dry-run`. Regresní test pokrývá oba argumentové kontrakty a
  T17 cookbook je upravený pro Pebble. Stejná revize synchronizovala seznam
  skutečně implementovaných testovacích skriptů a rozsah `run-all.sh` v
  cookbooku. Reálný Pebble průchod zůstává oddělený follow-up, aby se neměnil
  ruční TUI snapshot `pv`.
  Izolovaný klon `pv-tls-debug-20260913` nyní ověřil skutečné HTTP-01 vydání
  pro `ssl.test`, Apache `configtest`, Certbot renewal proti Pebble a deploy
  hook zaznamenaný v audit logu; následný `ssl disable` odstranil TLS vhost.
  Běh odhalil a opravil dva scénářové rozdíly: explicitní `[ssl].server` nesmí
  Certbot kombinovat s `--staging` a HTTP redirect se zapíná už přes `ssl
  enable`, nikoliv neexistujícím `website set` příkazem. E2 helper před
  obnovou klonovaného snapshotu zastaví instanci a může regenerovat její
  volatilní MAC, takže test nezasahuje do interaktivního `pv`.
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
  vhost i name-based výpis; opakovatelný T17 vytváří skutečný aktivní legacy
  vhost v `pv` a ověřuje odmítnutí ještě před přesunem dat či zápisem do
  provctl. Po jeho explicitním vypnutí test provede atomickou adopci a ověří
  PHP-FPM/Apache artefakty i HTTP odpověď. Balíček `0.0.7~local` prošel oběma
  větvemi a `pv` se následně obnovil na `clean`.
  E2 helper po obnově snapshotu nyní čeká až 60 sekund na systemd místo 30;
  tím se odstraní falešné selhání T05 při pomalejším startu `pv`. Změna
  prošla také kompletní vzdálenou CI `34655796789` včetně Debian 13 piuparts.
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
  **Revalidace 2026-09-16:** aktuální balíček
  `0.0.0+git.4b91a52` prošel izolovaným T17 proti
  `pv-tls-debug-20260913`: live legacy vhost byl odmítnut před mutací a po
  explicitním vypnutí se document root bezpečně adoptoval. T16 téhož balíčku
  současně prošel skutečným Pebble HTTP-01 issuance, renewal, deploy hookem a
  TLS disable. T16 nyní Pebble spouští přes transientní `systemd` unit, protože
  Incus při ukončení jednorázového `exec` scope ukončí běžný background proces.
  Reálný Pebble test *adoptovaného* TLS lineage zůstává samostatný follow-up.
  **M10 completed 2026-09-16:** nový izolovaný T17b vytvořil funkční legacy
  Apache web a jeho externě vlastněný Pebble certifikát, následně data adoptoval
  do `migrated-tls`, ověřil TLS vhost s původním lineage, změněný renewal
  webroot, skutečný forced renewal i zachování Certbot souborů po smazání
  provctl website. Spolu s T17 tak pokrývá odmítnutí aktivní kolize i bezpečný
  přesun reálné struktury. Automatická obnova po pádu během reconfigure a
  retence recovery kopií jsou vědomé budoucí provozní follow-upy, ne podmínka
  dokončeného migračního kontraktu.

## Pravidla ověřování

**Aktuální integrační stav (2026-09-16):** běh mimo filesystem sandboxu má
přístup k Incus skupině `incus-admin`. Aktuální `dist` balíček
`0.0.0+git.4b91a52` prošel offline `make test`, izolovaným E2 T17 a E3 T16.
Oba scénáře použily `pv-tls-debug-20260913` s `isolated-clean` snapshotem;
interaktivní `pv` nebyl změněn. T16 po skončení ověřeně obnovil snapshot.
**Release revalidace 2026-09-16:** balíček `0.0.0+git.4c0cd79` prošel celý
izolovaný gate `run-all` (T04 package structure, T05 piuparts install/purge,
T10 filesystem/PHP/log isolation a T17 adoption). T10 při tom odhalil
zastaralé očekávání jména per-domain PHP-FPM poolu a skutečnou chybu práv
parent log adresáře: nová subscription nyní vytváří
`/var/log/provctl/<subscription>` jako `root:<subscription> 0750`; vlastník
čte své `0640` logy, zatímco cizí subscription i `www-data` zůstávají
odmítnuté. Stejný invariant platí pro adopci.

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

V balíčkové CI se ukázal nesoulad licenčních metadat a chybějící Debian
dokumentace. Závazná licence projektu je GPLv3 (`LICENSE`), proto nfpm i
specifikace uvádějí `GPL-3.0-only`. Balíček nově nese strojově čitelný
copyright a generovaný `changelog.Debian.gz`; T04 je kontroluje. Statický,
CGO-free Go binární soubor a záměrně restriktivní oprávnění provozních cest
mají úzce popsané lintian overrides. Lokální build `0.0.6~local`, T04,
`lintian --no-tag-display-limit` v izolovaném Debian 13 `pv`, `make test` a
actionlint prošly. Vzdálený package job po pushi `b4b59d0` také prošel;
piuparts selhal výhradně na omezeném mountu `/proc` v Actions container jobu.
Workflow byl upravený na privilegovaný Debian Docker chroot; následující
vzdálený běh `34653180305` pro `9af00a0` je kompletně zelený.

Poslední integrační ověření (2026-08-30): bootstrap vytvořil požadované cesty
včetně práv, `apachectl configtest` a reload uspěly a druhý běh byl beze změn.
Následný `doctor` potvrdil provctl, Apache, PHP-FPM a Certbot; testovací obraz
zatím nemá funkční MariaDB socket autentizaci a obsahuje dva certbot renewal
mechanismy. Nejde o změnu provctl a kontejner byl následně obnoven na `clean`.
Ve stejný den proxy website úspěšně předala odpověď z lokálního upstreamu a
redirect website vrátila očekávané `302` a `Location`; i po tomto testu byl
kontejner obnoven na `clean`.

## Follow-up scope after v0.1

- [x] **CR 2026-09-20 — current TUI against refreshed branchctl and cookbook:**
  `branchctl` and the personal Bubble Tea cookbook were fast-forwarded before
  this review. The immediate correctness finding is that `New.Init()` starts
  subscription loading with generation `0`, while `opSlot.stale(0)` always
  returns false. A delayed initial reply can therefore overwrite a later
  explicit refresh; initialize through the same `opSlot.start()` path and add
  a regression test. The resulting work is complete: the strict generation
  guard prevents the stale reply, services publish typed plan stages, and a
  failed progress checklist remains until Enter/Esc acknowledges it. Terminal
  mutation results now pass through one pre-routing applicator that clears the
  complete progress state after acknowledgement, rather than each result case
  independently clearing only `active`. The existing binding table, popup classes,
  path picker, generation guards for normal reads, minimum-terminal guard and
  focused layout tests already match the current reference patterns; no broad
  UI rewrite is warranted.

- [x] **TUI read-race and failed-progress follow-up:** `opSlot` now compares
  every generation, including the initial generation `0`, so an initial
  subscription reply cannot overwrite a later refresh. A regression test
  covers that ordering. A failed terminal result of every stepped mutation now
  keeps its checklist modal open, with `Enter`/`Esc` explicitly dismissing it
  before the existing result handler writes the error and any follow-up state.
  The modal has priority over all underlying forms, its footer changes to
  document the acknowledgement, and a focused test verifies both defer and
  apply. The remaining review follow-up is to make mutation stage definitions
  typed domain pipeline plans rather than UI-owned positional labels; defer
  the larger result-handler extraction until that refactor can use it. `go vet
  ./...` and `go test ./... -race -count=1` passed; `make test` remains blocked
  only because `staticcheck` is not currently installed on the host PATH.

- [x] **Typed operation pipeline:** `plan.Executor` now publishes a typed,
  context-scoped progress stream: the immutable step list followed by running,
  done, failed and rolled-back transitions. The stream deliberately omits
  command previews and errors to keep terminal progress safe. Its focused
  executor test and the service suite pass with `-race`. Bubble Tea now
  transports that stream through its existing operation channel: once an
  executor starts, the generic apply placeholder is replaced by its actual
  plan steps, while a UI-only post-operation refresh remains last. PHP-FPM
  therefore already shows pool, socket, vhost and SQLite steps from its real
  plan; all other mutations using `plan.Executor` inherit the same behavior
  without per-form wiring. Rollbacks render distinctly. A focused UI test
  proves replacement and preservation of the refresh step. TLS certificate
  issuance now uses the same stream without being forced through executor
  rollback semantics: it reports the safe sequence DNS preflight → HTTP ACME
  vhost → HTTP self-check → Certbot → HTTPS vhost → metadata → optional
  renewal verification. Its focused UI test proves the concrete Certbot step
  replaces the generic label and remains failed until dismissed. TLS disable
  and TLS alias reconciliation now report their own safe sequences too, so
  every TUI TLS mutation is covered. Their service tests assert the published
  ordering. The audit found that all other TUI mutations use `plan.Executor`;
  no untyped multi-stage service path remains.

- [x] **TUI administration refinements:** Settings now uses the installed
  PHP-FPM version picker for `php.default_version`; selection stays pending
  until `Ctrl+S`, and the same popup marks the configured value. Cron's `n`
  creates a job and `Enter`/`e` edits the selected job through the same modal
  form; it previews the next local server execution, keeps the job ID stable,
  rewrites the generated crontab transactionally, and confirms the change.
  Each domain's Logs tab now also displays its default or explicit Apache log
  directory; `e` opens a path-picker-backed form and confirmation. The
  persisted override is constrained to `/var/log/provctl/<subscription>/…`,
  created as `root:<subscription> 0750`, and reflected by log reads, HTTP and
  TLS vhosts. `website logdir set` exposes the same validated service path.
  The focused TUI suite passed with the race detector. The current package
  (`0.0.0+git.3a33814`) also passed T04 and the isolated Incus T10 server
  test: bootstrap, Apache, PHP-FPM, two subscriptions, HTTP isolation,
  private sessions and log permissions. The test container was restored to
  `isolated-clean`.

- [x] **Final validation 2026-09-20:** `staticcheck 2026.2.1` is installed in
  the standard Go tool directory and the complete `make test` entry point now
  passes (`go vet`, `staticcheck`, and race-enabled unit tests). The current
  package `0.0.0+git.886e627` passed live T10 PHP-FPM/Apache isolation and T17
  legacy-vhost adoption in `pv-tls-debug-20260913`; the instance was restored
  to `isolated-clean` after each run. A separate stateful E2 lifecycle created
  and rotated a database credential without logging it, created/edited/removed
  a cron job, added/removed an SSH key before enabling key access, and
  created/inspected/dry-run-restored a backup before removing the database.
  T16 then passed against a fresh official Pebble checkout (`13f2ac3`): actual
  HTTP-01 issuance, forced renewal, deploy hook, and TLS disable all succeeded.
  The failed setup attempts documented the intended guards: credential export
  requires an existing subscription-owned directory, SSH key access requires a
  key first, and database deletion requires `--yes`. A fresh current-package
  Incus fixture also opened the real alternate-screen TUI and rendered its
  subscription list, including the newly created PHP-FPM subscription, before
  it too was restored to `isolated-clean`.
- [x] **Official-release Docker server test:** added a disposable Debian 13
  systemd-capable container that installs `provctl` only from the signed
  GitHub Pages APT repository, then exercises bootstrap, doctor, subscription
  creation and a real Apache static-site enable/disable lifecycle. The first
  run against `0.1.1` found that Debian's non-interactive `policy-rc.d` leaves
  newly installed PHP-FPM and MariaDB inactive; `v0.1.2` starts inactive
  required services before applying its artifacts. A clean, privileged Docker
  server installed the signed Pages `0.1.2` package, completed bootstrap and
  doctor with all checks OK, served the static site, removed that site's
  content when disabled (while allowing Apache's catch-all response), then
  served it again after enable. Because privileged systemd Docker containers
  can contend with a desktop host's cgroups and disk I/O, the helper now
  requires `PROVCTL_ALLOW_PRIVILEGED=1`; local E2 validation remains Incus.
- [x] **v0.1.4 public APT server validation:** a Debian 13 Incus system
  container verified the public signing-key fingerprint, installed `0.1.3`
  and then upgraded to `0.1.4` exclusively from GitHub Pages. Bootstrap and
  doctor passed; the scenario exercised subscription quotas, PHP-FPM/static/
  proxy/redirect vhosts with real Apache responses, per-domain PHP, document
  root, log directory, alias, enable/disable, reconcile, database credentials
  and password rotation, SSH access and keys, cron create/edit/remove, backup
  create/inspect/restore dry-run, and subscription suspend/resume/archive/
  double-confirmed delete. This run found and fixed the proxy/redirect health
  false-positive recorded in M7. The official `0.1.4` artifact also passed
  T16 against Pebble: HTTP-01 issuance, forced renewal, deploy hook, and TLS
  disable. Both scenarios restored `isolated-clean` afterward.
