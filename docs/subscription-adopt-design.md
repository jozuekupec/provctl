# Subscription adoption

`provctl subscription adopt` imports one existing project into a new managed
subscription through a single journaled operation. It is available from the
CLI and from the subscription picker (`i`) in the TUI.

## Command contract

```text
provctl subscription adopt <name> --domain <domain>
    [--type php-fpm|static|proxy|redirect]
    [--from <path> | --target <url>]
    [--redirect-code 301|302] [--copy] [--no-backup] [--dry-run]
```

`php-fpm` and `static` require `--from`: it must be an existing directory
outside the managed vhosts root. The default is an atomic move into
`<vhosts>/<name>/sites/<domain>/public`; `--copy` is explicit. A cross-device
move fails safely rather than silently copying. A backup is created by default.

`proxy` and `redirect` have no document root. They require `--target`; proxy
targets use the ordinary loopback/allowlist validation, while redirects accept
only 301 or 302. They never inspect or alter Docker, Compose, or project-owned
runtime files.

## Safety and ownership

Data-bearing adoption creates the subscription identity and recursively assigns
the final document root to its allocated UID:GID using explicit,
non-symlink-following command arguments. This is part of the plan, not an
operator follow-up. The operation validates source/destination, name and domain
conflicts, and generated Apache configuration before committing metadata.

For a container-backed proxy, adapting the application process to the new
UID:GID remains the operator's responsibility. Configure its documented
runtime environment; do not grant the subscription access to the Docker daemon
or ask provctl to rewrite `.env` or Compose files.

## TLS lineage

An adoption may connect a validated existing Certbot lineage to the imported
website. The website stores the association; live Certbot files remain the
source of certificate material and expiry. A lineage predating provctl is
marked externally owned: deleting its adopted website removes generated vhost
artifacts and local metadata but does not revoke that certificate.

Renewal reconfiguration is guarded by a saved pre-change configuration under
`/var/lib/provctl/renewal-backups/`. If the irreversible verification stage
fails, the journal records an inconsistent operation and the prior renewal
configuration remains available for recovery.

## Verification

Use `--dry-run` first. The repeatable PHP and non-PHP Incus scenarios are T17,
T17b, and T17c in the [testing cookbook](testing-cookbook.md). T17c has been
run against a freshly built package for static, proxy, redirect, and PHP-FPM:
it verified content transfer, ownership, rendered vhosts, an HTTP response,
and `apache2ctl configtest`.
