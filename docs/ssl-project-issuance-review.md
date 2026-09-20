# TLS lifecycle

This document records the current TLS model and its operational boundaries.
It is not a substitute for the command help or the [testing cookbook](testing-cookbook.md).

## Current model

TLS belongs to a website, not to an entire subscription. Each website has a
stable certificate lineage name; generated certificates use a website-derived
name and an adopted website may retain its validated pre-existing lineage.
SQLite indexes ownership and state, while Certbot's live and renewal files are
the authority for certificate material, SANs, and expiry.

`provctl ssl enable <subscription> <domain>` is explicit. It first renders an
HTTP-only vhost, performs DNS and HTTP-01 reachability checks for every
requested hostname, runs Certbot with the managed webroot, verifies the live
lineage, then renders TLS and reloads Apache. HTTP-to-HTTPS redirects always
exclude `/.well-known/acme-challenge/`.

`ssl status` reads live expiry. The global Certbot deploy hook validates Apache
configuration and reloads it after a successful renewal. Disabling TLS removes
TLS configuration but does not delete a lineage. Deleting a provctl-managed
website removes its vhost before certificate metadata and Certbot artifacts;
an adopted, externally owned lineage is intentionally retained.

## Operator workflow

1. Point every required A/AAAA record to the server and ensure public port 80
   reaches Apache.
2. In development, set `[ssl].staging = true` in Settings or
   `/etc/provctl/config.toml`.
3. Create or select the website, then run `ssl enable` from the TUI or CLI.
4. Check `provctl ssl status <subscription> <domain>` and HTTPS with the
   intended hostname.
5. Before a release, run the Pebble scenarios in the cookbook rather than
   consuming Let's Encrypt production limits.

DNS mismatch is a blocking safety check. `--force` is reserved for an operator
who understands a NAT or reverse-proxy topology; it is not a background retry
mechanism.

## Deferred work

Automatic issuance during website creation is deliberately out of scope: DNS
may not be ready then, and provctl has no persisted retry queue. The safe
workflow remains an explicit `ssl enable` after DNS is ready.

SAN changes through alias addition/removal require full Pebble regression
coverage for both directions, including renewal. Renewal configuration backups
are retained for manual recovery; automatic recovery after process loss and
backup retention policies are future work.
