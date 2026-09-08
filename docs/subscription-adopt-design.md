# Subscription Adopt Design

`subscription adopt` migrates one existing document root into one new
subscription. It is a production mutation and must use one journaled plan;
it must not compose the existing `subscription create` and `website create`
commands as separate operations.

## Contract

```text
provctl subscription adopt <name> --from <path> --domain <domain>
    [--copy] [--no-backup] [--dry-run]
```

The source must exist, be a directory, and must not resolve inside the
configured vhosts root. The target is exactly
`<vhosts>/<name>/sites/<domain>/public`; it must not exist. The domain and
subscription name use the ordinary domain validators. `--copy` is opt-in;
the default is an atomic rename on the same filesystem. Cross-filesystem
renames fail with an actionable message rather than silently copying data.

## Plan order

### Accepted TLS identity decision

Adoption preserves the existing Certbot lineage instead of issuing a new
certificate. `Website.CertificateName` is the persistent website-to-lineage
mapping: ordinary creation defaults to `provctl-site-<id>`, while adoption
may supply the validated original name. This uses the existing column and
unique constraint; a lineage cannot silently become owned by two websites.
Live certificate files remain authoritative for SANs and expiry.

Repository persistence supports this mapping. Adoption wiring, certificate
validation, TLS activation, certificate metadata, and renewal rollback are
still required before this decision is fully implemented.

### Execution

1. Inspect source, destination, Unix identity, database/domain conflicts, and
   matching Certbot renewal lineages. Present all of these in dry-run output.
2. Create a recoverable backup of the source when backup is enabled (default).
3. Create the Unix user and subscription-owned base directories.
4. Create the website/log/PHP-FPM artifacts required for a PHP-FPM website,
   but leave its document root empty.
5. Move or copy the source into the exact document root and recursively assign
   it to the subscription UID/GID.
6. Run Apache/PHP-FPM validation and enable the vhost.
7. Write the subscription and website rows only after system artifacts exist.
8. Reconfigure every discovered certificate lineage to the shared ACME
   webroot, then run `certbot renew --cert-name <lineage> --dry-run`.

Renewal configuration uses Certbot 2.3+ `reconfigure --authenticator webroot
--webroot-path <shared-root> --cert-name <lineage>`. It tests the new options
against staging before saving them and preserves the live certificate. Do not
use `certonly --keep-until-expiring` here: it can issue a replacement when the
certificate is nearing expiration. See the
[Certbot renewal configuration guide](https://eff-certbot.readthedocs.io/en/stable/using.html#modifying-the-renewal-configuration-of-existing-certificates).

The first seven steps are reversible. If certificate renewal verification
fails after the filesystem transfer, the operation is recorded as
`inconsistent`; it must state the affected lineage and preserve both the data
and the pre-change renewal configuration for manual recovery.

## Required seams and tests

- `system.FileMover` for same-filesystem rename; a dedicated copy seam for
  `--copy`, never a shell string.
- a command seam for recursive ownership, using explicit `chown` arguments;
  no user input reaches a shell.
- a renewal inspector/reconfigurer seam, with tests for no certificate,
  multiple SAN lineages, reconfiguration failure, and failed renewal dry-run.
- fake-FS plan tests for destination escape, pre-existing target, rollback
  after every system step, and SQLite being written last.
- an Incus test against a copied legacy document root. Pebble is required for
  the certificate branch; its clean snapshot is deliberately separate from
  the normal E2 image.
