# provctl

`provctl` is a root-facing provisioning tool for Debian hosting servers. It manages subscriptions, websites, PHP-FPM, Apache, MariaDB, TLS certificates, backups, cron jobs, and SSH access from SQLite as the source of truth.

The binding design is [the project specification](docs/project-specification.md). Test environments and acceptance scenarios are in [the testing cookbook](docs/testing-cookbook.md).

## Safety model

System configuration is generated from the database; it is not the source of truth. Mutating operations are planned, journaled, locked, and rolled back on failure. Commands use explicit arguments through a restricted system abstraction—never a shell. Do not run unfinished mutating commands as root on a workstation.

## Current status

The foundation, operation executor, durable journal, lock, rollback, and the first subscription operation are in place. `subscription create` creates the Unix account, isolated directory layout, and final SQLite record; website and PHP-FPM lifecycle commands are not available yet.

## Local development

Go 1.22+ is required. These commands do not require root or Debian services:

```bash
make test                 # vet, staticcheck, race-enabled unit tests
make build                # produces dist/provctl
dist/provctl --version
```

`doctor` is read-only, but checks the host's services and paths, so it may deliberately return a non-zero result on a development machine:

```bash
dist/provctl doctor --config packaging/config.toml.default --json
```

`subscription create` is a root-facing operation. Its dry run reads the existing database and account state, then prints the exact planned steps without changing the system:

```bash
sudo dist/provctl subscription create acme --config /etc/provctl/config.toml --dry-run
```

The non-dry-run form requires the state directory and database created by the upcoming `bootstrap` command; do not create those system paths manually on a workstation.

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
  apt install -y apache2 php-fpm mariadb-server certbot
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
