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
- [~] **M3 — websites a Apache:** PHP-FPM website create, ukládání domény,
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
  Apache, jinak může jeho runtime adresář krátce chybět. Read model nyní nese
  aliasy a SQLite bezpečně vynucuje jejich globální unikátnost; zbývá atomická
  změna vhostu, CLI aliasů a `reconcile`.
  Golden testy nyní pokrývají všechny čtyři typy HTTP vhostu.
- [~] **M4 — PHP-FPM:** automatická detekce a výběr verze, render a atomické
  vytvoření poolu včetně ověření socketu jsou hotové. Zbývá změna verze poolu
  (`php set`) s rollbackem.

## Bezprostřední práce

- [~] `bootstrap`: vytvoření systémových adresářů a audit logu s právy ze
  specifikace, moduly, výchozí certifikát, vhost, deploy-hook, logrotate i
  skutečně prázdný druhý běh (`nothing to do`) jsou hotové. Chybí přepínače
  `--yes`, `--skip`, `--install-missing` a automatické vypsání výsledku
  `doctor`.
- [~] Unit testy bootstrapu pokrývají prázdný plán, chybějící systémové cesty
  a odmítnutí změny práv existujícího adresáře. Zbývá cílený rollback test.
- [x] Projít end-to-end HTTP požadavek přes Apache v Incus kontejneru `pv`; po
  každém integračním testu obnovit snapshot `clean`. PHP-FPM, static, proxy i
  redirect tok jsou ověřeny přes lokální HTTP požadavky s `--resolve`.
  Poslední běh proxy přenesl tělo z `127.0.0.1:8080`; redirect vrátil `302` a
  očekávaný `Location`. Apache configtest uspěl a kontejner byl obnoven na
  `clean`.

## Následující milníky

- [ ] **M5 — MariaDB, SSH a cron:** bezpečné předávání hesel přes stdin,
  databázové lifecycle operace, SSH klíče a crontab.
- [ ] **M6 — SSL:** stavový automat Certbotu, deploy-hook command a přijetí
  existujících certifikátů.
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
