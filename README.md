# provctl

`provctl` is a root-facing provisioning tool for Debian hosting servers. It manages subscriptions, websites, PHP-FPM, Apache, MariaDB, TLS certificates, backups, cron jobs, and SSH access from SQLite as the source of truth.

The binding design is [the project specification](docs/project-specification.md). Test environments and acceptance scenarios are in [the testing cookbook](docs/testing-cookbook.md).

Maintainers: [APT signing-key setup, vault backup, and recovery](docs/apt-signing-keys.md).

## Safety model

System configuration is generated from the database; it is not the source of truth. Mutating operations are planned, journaled, locked, and rolled back on failure. Commands use explicit arguments through a restricted system abstraction—never a shell. Do not run unfinished mutating commands as root on a workstation.

## Current status

The implementation status and outstanding work are tracked in
[the roadmap](docs/roadmap.md). The command-line interface and read-mostly TUI
are both available.

## Terminal UI

Running `provctl` without a subcommand starts the terminal UI:

```bash
sudo provctl
```

The UI reads the same `/etc/provctl/config.toml` and SQLite state as the CLI.
On a fresh server, install the package and run `sudo provctl bootstrap` first;
in an uninitialized development checkout, the missing configuration error is
expected. Use explicit subcommands for non-interactive administration, for
example `sudo provctl subscription list`.

Press `,` in either TUI screen to open **Settings**. Its section tabs expose
every supported key from `/etc/provctl/config.toml`; Shift+Left/Right changes
section, Tab/Up/Down changes field, and Enter (or Ctrl+S) saves atomically
without discarding comments or unknown administrator keys. Restart the TUI
after saving non-TLS settings so its already-opened service runtimes reload
them. Keep ACME staging enabled until certificate issuance has been verified;
public Let's Encrypt issuance also needs public DNS and reachable HTTP port 80.
For filesystem paths, Enter opens a directory or file picker; use Enter to
open a directory, Space to select it, and confirm the selected absolute path.

## Install from the APT repository

The following endpoint is the planned release destination; its first public
deployment is still pending verification. Once published, install on your
Debian hosting server using:

```bash
curl -fsSLo /tmp/provctl.asc https://jozuekupec.github.io/provctl/debian/provctl.asc
gpg --show-keys --with-fingerprint /tmp/provctl.asc
```

Check the primary fingerprint against the maintainer-provided value:
`578A5B0F5AABFB5851803D05FE92B73E4B3967C4`.

```bash
sudo install -d -m 0755 /etc/apt/keyrings
sudo gpg --dearmor --output /etc/apt/keyrings/provctl.gpg /tmp/provctl.asc
echo 'deb [signed-by=/etc/apt/keyrings/provctl.gpg] https://jozuekupec.github.io/provctl/debian stable main' | sudo tee /etc/apt/sources.list.d/provctl.list
sudo apt update
sudo apt install provctl
sudo provctl doctor
sudo provctl bootstrap
```

Use `testing` instead of `stable` for release candidates. Public repository
configuration never requires the private signing key or its passphrase.

## Local development

Go 1.22+ is required. These commands do not require root or Debian services:

```bash
make test                 # vet, staticcheck, race-enabled unit tests
make build                # produces dist/provctl
dist/provctl --version
```

`doctor` and `health` are read-only, but inspect the host's services and paths, so they may deliberately return a non-zero result on a development machine:

```bash
dist/provctl doctor --config packaging/config.toml.default --json
dist/provctl health acme example.test --config /etc/provctl/config.toml --json
```

`subscription create` is a root-facing operation. Its dry run reads the existing database and account state, then prints the exact planned steps without changing the system:

```bash
sudo dist/provctl subscription create acme --config /etc/provctl/config.toml --dry-run
sudo dist/provctl subscription create acme --quota-disk 20G
sudo dist/provctl subscription create acme --quota-websites 5 --quota-databases 3 --quota-backups 2
```

The non-dry-run form requires the state directory and database created by the upcoming `bootstrap` command; do not create those system paths manually on a workstation.

List recorded archives without changing the server:

```bash
sudo dist/provctl backup list acme
```

## Isolated server tests with Incus

Run E2/E3 integration tests in a Debian 13 VM, then use an unprivileged Incus **system container**. Incus is sufficient; do not also install standalone LXC tooling. Its network and storage stay inside the VM.

```bash
sudo apt update
sudo apt install -y incus
sudo incus admin init --minimal
sudo usermod -aG incus-admin "$USER"
newgrp incus-admin

incus launch images:debian/13 pv
incus exec pv -- bash -lc '
  apt update &&
  apt install -y apache2 php-fpm mariadb-server certbot cron zstd
'
incus snapshot create pv clean

incus exec pv -- systemctl is-system-running
incus exec pv -- systemctl status apache2
```

If Docker's `FORWARD DROP` policy blocks the Incus bridge, install the narrowly scoped, persistent forwarding service from this repository:

```bash
sudo ./scripts/dev/install-incus-docker-forwarding.sh
```

It determines the current default uplink, enables IPv4 forwarding, and permits only Incus egress plus established replies. To remove it: `sudo systemctl disable --now incus-docker-forward.service` and remove `/etc/systemd/system/incus-docker-forward.service` and `/etc/sysctl.d/90-incus-forwarding.conf`.

The container does not need privileged mode or nested virtualization. Reset it before each mutating scenario:

```bash
incus restore pv clean
```

Kernel, firewall, public DNS, and live Let's Encrypt checks require the later VM/VPS environments described in the cookbook.
