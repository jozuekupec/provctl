# provctl roadmap

This is the live, concise status of the repository. Detailed behavior belongs
in the [project specification](project-specification.md); reproducible checks
belong in the [testing cookbook](testing-cookbook.md).

Legend: `[x]` completed and validated, `[ ]` deferred or not yet scheduled.

## Current release state

`v0.1.7` is the latest public release. GitHub Actions built amd64 and arm64
packages, published the GitHub Release, and updated the signed GitHub Pages
APT repository. A fresh Debian 13 Incus container verified the public key,
installed exactly `0.1.7` from `stable`, ran bootstrap and doctor, and updated
an existing subscription's quotas, including the `0` unlimited disk value.

The working integration target is the disposable Debian 13 Incus container
`pv`. Restore it with `./scripts/e2.sh reset` before a scenario and after a
mutating run. Do not run privileged Docker/systemd tests on the desktop host.
For an interactive smoke test, `scripts/dev/run-tui-test.sh` builds, installs,
bootstraps, opens the TUI, and restores `pv:clean` after exit.

## Completed scope

- [x] **M0 — foundation:** Go CLI, typed configuration, system-command seam,
  SQLite migrations, audit foundation, `doctor`, and architectural tests.
- [x] **M1 — operations:** planned, journaled mutations with locking,
  rollback, idempotence, reconciliation, and dry-run support.
- [x] **M2 — subscriptions:** isolated Unix users, homes and permissions;
  lifecycle actions; quota creation limits and usage reporting.
- [x] **M3 — websites and Apache:** PHP-FPM, static, proxy and redirect
  sites; aliases; enable/disable/delete; logs; generated vhosts; atomic
  configtest/reload; drift detection and reconciliation.
- [x] **M4 — PHP-FPM:** installed-version detection, per-domain selection,
  subscription pools, safe socket handover, and Apache reload.
- [x] **M5 — hosting services:** MariaDB database lifecycle and credentials,
  SSH access and keys, cron jobs, and subscription backups/restores.
- [x] **M6 — TLS:** explicit HTTP-01 issuance, DNS/HTTP preflight, Certbot
  deployment hook, status, staging configuration, safe redirects, and
  Pebble-based issuance/renewal/disable validation.
- [x] **M7 — operations visibility:** text/JSON health checks, quota warnings,
  and redacted JSONL audit events.
- [x] **M8 — terminal UI:** default no-argument TUI, subscription picker,
  scoped workspace, contextual help/filtering, settings, forms, path picker,
  confirmations, and plan-backed progress popups. See [TUI design](tui-design.md).
- [x] **M9 — distribution:** nfpm Debian packages, GitHub Actions, signed APT
  repository, signing-key recovery procedure, and public package validation.
- [x] **M10 — adoption:** journaled PHP-FPM/static/proxy/redirect import,
  data ownership transfer, external runtime boundary, optional backups, TUI
  entry form, and Certbot-lineage handling. See [adoption](subscription-adopt-design.md).

- [x] **Post-v0.1 TLS and quotas:** TLS alias reconciliation issues the exact
  complete SAN set, including safe removal, and T16 Pebble covers add/remove,
  certificate contents, renewal, and Apache rendering. Renewal recovery
  snapshots are retained for 30 days per lineage; cleanup failure retains extra
  evidence, and automatic post-crash restoration is intentionally prohibited
  because operation completion cannot be inferred safely. Subscription quotas
  are editable through `subscription quota set` and the administration
  Overview tab (`e`), with a journaled SQLite update and rollback.

## Recent validation

- `make test` passes: `go vet`, `staticcheck`, and race-enabled unit tests.
- Public `v0.1.6` was installed from GitHub Pages in fresh Debian 13; bootstrap,
  doctor, quota creation and a persisted quota update were checked.
- Pebble E3 verified HTTP-01 issuance, forced renewal, deploy hook, and TLS
  disable without using public ACME limits.
- A current-package `pv` run validated `static`, `proxy`, `redirect`, and
  `php-fpm` adoption, including data transfer, UID:GID, rendered vhosts, PHP
  response, and `apache2ctl configtest`.

## Deferred follow-up


## Change rule

Any material implementation change updates this document with its validation
and limitation. Run `make test`; privileged work belongs in `pv` and ends by
restoring `clean`.
