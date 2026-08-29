# provctl

`provctl` is a root-facing provisioning tool for Debian hosting servers. It manages subscriptions, websites, PHP-FPM, Apache, MariaDB, TLS certificates, backups, cron jobs, and SSH access from SQLite as the source of truth.

The binding design is [the project specification](docs/project-specification.md). Test environments and acceptance scenarios are in [the testing cookbook](docs/testing-cookbook.md).

## Safety model

System configuration is generated from the database; it is not the source of truth. Mutating operations are planned, journaled, locked, and rolled back on failure. Commands use explicit arguments through a restricted system abstraction—never a shell. Do not run unfinished mutating commands as root on a workstation.

## Current status

The foundation is in place: configuration loading, SQLite migrations, system abstractions, `provctl doctor`, and architecture checks. The next milestone adds the executor, durable operation journal, exclusive lock, and rollback support. Subscription and website lifecycle commands are not available yet.

## Local development

Go 1.22+ is required. These commands do not require root or Debian services:

```bash
make test                 # vet, staticcheck, race-enabled unit tests
make build                # produces build/provctl
build/provctl --version
```

`doctor` is read-only, but checks the host's services and paths, so it may deliberately return a non-zero result on a development machine:

```bash
build/provctl doctor --config packaging/config.toml.default --json
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
  apt install -y apache2 php-fpm mariadb-server certbot
'
incus snapshot create pv clean

incus exec pv -- systemctl is-system-running
incus exec pv -- systemctl status apache2
```

The container does not need privileged mode or nested virtualization. Reset it before each mutating scenario:

```bash
incus restore pv clean
```

Kernel, firewall, public DNS, and live Let's Encrypt checks require the later VM/VPS environments described in the cookbook.
