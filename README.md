# provctl

`provctl` provisions and operates Debian web-hosting subscriptions from one
auditable source of truth. It manages isolated Unix accounts, Apache vhosts,
per-domain PHP-FPM, MariaDB databases, TLS certificates, cron jobs, SSH keys,
and backups through both a CLI and terminal UI.

It is a root-facing server tool, not a desktop application. Use it on a
disposable test server first; never run mutating commands against a personal
workstation.

## Features

- Subscription lifecycle: create, suspend, archive, resume, and delete
- PHP-FPM, static, proxy, and redirect websites with per-domain settings
- Apache configuration generated from SQLite, with journaling and rollback
- Certbot HTTP-01 TLS, databases, SSH access, cron jobs, quotas, and backups
- A keyboard-first terminal UI: running `provctl` with no subcommand opens it

## Install

`provctl` supports Debian 13 and its packaged services. Verify the signing-key
fingerprint before adding the public APT repository:

```bash
curl -fsSLo /tmp/provctl.asc https://jozuekupec.github.io/provctl/debian/provctl.asc
gpg --show-keys --with-fingerprint /tmp/provctl.asc
# Expected primary fingerprint: 578A5B0F5AABFB5851803D05FE92B73E4B3967C4

sudo install -d -m 0755 /etc/apt/keyrings
sudo gpg --dearmor -o /etc/apt/keyrings/provctl.gpg /tmp/provctl.asc
echo 'deb [signed-by=/etc/apt/keyrings/provctl.gpg] https://jozuekupec.github.io/provctl/debian stable main' \
  | sudo tee /etc/apt/sources.list.d/provctl.list >/dev/null
sudo apt update
sudo apt install provctl
```

For a release candidate, replace `stable` with `testing`. The repository needs
only the public key; keep the private signing key outside the server.

## First server

Bootstrap creates the managed paths, Apache integration, audit log, ACME
webroot, deploy hook, and required packages. Inspect the plan first, then run
the confirmed command:

```bash
sudo provctl bootstrap --dry-run
sudo provctl bootstrap --install-missing --yes
sudo provctl doctor
sudo provctl
```

The last command opens the terminal UI. Press `?` for context-sensitive help,
`s` to return to the subscription picker, and `,` to edit configuration. The
UI exposes domain editing, PHP selection, TLS, document roots, log directories,
databases, cron jobs, SSH keys, backups, and subscription lifecycle actions.
Destructive actions always require a confirmation dialog.

Use the CLI for automation:

```bash
sudo provctl subscription create acme --quota-disk 20G --quota-websites 5
sudo provctl website create acme example.test --type php-fpm
sudo provctl php set acme example.test --version 8.4
sudo provctl ssl enable acme example.test
sudo provctl health acme example.test --json
```

Certificate issuance requires every requested hostname to resolve to the
server and HTTP port 80 to be reachable. Before a first real issuance, set
`[ssl].staging = true` in Settings (or `/etc/provctl/config.toml`) and verify
that path against the Let's Encrypt staging CA.

## Develop and test

Go 1.22+ is required. Build artifacts always go to `dist/`:

```bash
make test                 # vet, staticcheck, and race-enabled tests
make build                # dist/provctl
make deb                  # dist/provctl_<version>_amd64.deb
```

Integration tests must run in a Debian 13 Incus system container or VM, never
on the development host. Incus is sufficient for real Apache, PHP-FPM,
MariaDB, systemd, and filesystem tests; it does not need a privileged
container or nested virtualization. The reproducible setup, snapshots, E2/E3
scenarios, and Docker caveat are in the
[testing cookbook](docs/testing-cookbook.md).

## Documentation

- [Roadmap](docs/roadmap.md) — current scope, validation status, and deferred work
- [Testing cookbook](docs/testing-cookbook.md) — repeatable local, Incus, package, and Pebble checks
- [TUI design](docs/tui-design.md) — interaction model and implementation rules
- [Subscription adoption](docs/subscription-adopt-design.md) — migration contract and operational limits
- [TLS lifecycle](docs/ssl-project-issuance-review.md) — current certificate model and remaining work
- [Signing-key operations](docs/apt-signing-keys.md) — key backup and recovery
- [Project specification](docs/project-specification.md) — detailed v1 architecture reference

## Safety

SQLite is the managed-state source of truth; system configuration is generated
from it. Mutating operations are planned, journaled, locked, and rolled back
where possible. `provctl` passes explicit command arguments and does not use a
shell to execute user-controlled input.
