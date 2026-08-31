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

- [~] `bootstrap`: vytvoření systémových adresářů a audit logu s právy ze
  specifikace, moduly, výchozí certifikát, vhost, deploy-hook, logrotate i
  skutečně prázdný druhý běh (`nothing to do`) jsou hotové. Chybí přepínače
  `--yes`, `--skip`, `--install-missing` a automatické vypsání výsledku
  `doctor`.
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
- [~] **M6 — SSL:** příprava pro stavový automat Certbotu obsahuje
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
  self-check. Zbývá adopt.
- [ ] **M7 — provoz:** backup/restore, health checks, audit log a kvóty.
- [~] **M8 — TUI:** návrh je zaznamenán v [tui-design.md](tui-design.md) a
  cíleně přebírá konzistentní Bubble Tea vzor z projektu `depo`: hodnotový
  model, `Deps`, samostatné routing/render/keys/theme a I/O jen přes `tea.Cmd`.
  První read-only subscriptions obrazovka je funkční (`provctl` bez argumentů):
  načítá přes `tea.Cmd`, umí pohyb a refresh a má modelový test. Websites se
  nyní načtou pro vybranou subscription přes service vrstvu. Detail a omezený
  copy-on-write output panel jsou hotové a testované; následují potvrzované
  mutace a vizuální terminálové ověření.
- [ ] **M9 — distribuce:** nfpm, maintainer skripty, CI, APT repozitář a
  package testy (`lintian`, `piuparts`, upgrade/purge).
- [ ] **M10 — migrace:** `subscription adopt` pro existující weby.

## Pravidla ověřování

Unit a golden testy běží neprivilegovaně přes `make test`. Integrační ověření
probíhá jen v Debian 13 kontejneru `pv`, nikdy na hostiteli; návrat do čistého
stavu provede `incus snapshot restore pv clean`. Před uzavřením každého milníku
se do tohoto souboru doplní rozsah ověření a případná odchylka od specifikace.

Poslední integrační ověření (2026-08-30): bootstrap vytvořil požadované cesty
včetně práv, `apachectl configtest` a reload uspěly a druhý běh byl beze změn.
Následný `doctor` potvrdil provctl, Apache, PHP-FPM a Certbot; testovací obraz
zatím nemá funkční MariaDB socket autentizaci a obsahuje dva certbot renewal
mechanismy. Nejde o změnu provctl a kontejner byl následně obnoven na `clean`.
Ve stejný den proxy website úspěšně předala odpověď z lokálního upstreamu a
redirect website vrátila očekávané `302` a `Location`; i po tomto testu byl
kontejner obnoven na `clean`.
