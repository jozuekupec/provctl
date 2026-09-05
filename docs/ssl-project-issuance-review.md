# SSL issuance review for projects

This review compares the current implementation with the proposed
per-project certificate model. It is a design input, not a claim that the
behavior is already complete.

## Current state

`ssl enable <subscription> <domain>` already uses the correct safe sequence:
an HTTP-only vhost, DNS and HTTP self-checks, `certbot --webroot`, then TLS
configuration. HTTP renderers exempt the ACME path from a force-HTTPS
redirect. The Certbot deploy hook and live-file expiry check are also in
place.

The current lineage is `provctl-<subscription>-<primary-domain>`. Certificates
are linked only to a subscription, and adding or removing an alias merely
rewrites Apache configuration. Consequently an enabled TLS site can advertise
an alias absent from its certificate. Project deletion removes only SQLite
metadata; it deliberately leaves the Certbot lineage behind.

## Recommended model

In this repository a *project* should mean one `website`, not a subscription:
a subscription may own multiple independent websites. Add a stable,
non-domain-derived `certificate_name` to `websites`, generated when the
website is created (for example `provctl-site-<id>`), and add a unique
`website_id` foreign key to `certificates`. Use that name for every Certbot,
Apache, status, and deploy-hook operation. Certbot's renewal and live files
remain authoritative; SQLite is only the ownership/cache index.

## Required lifecycle changes

1. Bootstrap a managed global ACME Apache configuration for the central
   webroot, while retaining the renderer's redirect exemption. Test that the
   catch-all and every generated vhost return the challenge file unchanged.
2. Make DNS preflight inspect every A and AAAA result for every SAN. A mismatch
   should block automatic issuance; retain an explicit `--force` escape hatch
   only for documented NAT/proxy deployments. Check the HTTP challenge path on
   every requested hostname, not only the primary domain.
3. Keep the existing two-phase Apache sequence. Never render `:443` before
   `fullchain.pem` exists and is readable.
4. On alias add/remove for an SSL website, calculate the complete desired SAN
   set, update the lineage first, verify the live certificate, then update the
   vhost and SQLite. Do not rely on `--expand` for removal until its behavior
   with a shorter set has an integration test; use the Certbot invocation that
   demonstrably replaces the complete requested set.
5. On website deletion, remove and reload the vhost first, then call
   `certbot delete --cert-name <stable-name>` and finally delete cached
   metadata. Make the irreversible Certbot step explicit in the operation
   result. Subscription deletion must delegate this for each website.
6. Extend `ssl status` to resolve the stable certificate name and inspect the
   live Certbot lineage (optionally parse renewal `domains` for display), and
   provide a `--staging` switch on issuance and domain-change operations.

## Decision needed for automatic issuance

The existing binding specification permits a DNS mismatch only with explicit
`--force`; it has no background job or retry state. Therefore the safe default
is **not** to issue automatically during `website create`: creation succeeds
with HTTP, then `ssl enable` is a blocking, explicit operation once DNS is
ready. If automatic issuance is required, expose it as an opt-in create flag
and fail before calling Certbot when DNS is not ready. A background retry loop
would require a separate persisted job model and should not be introduced
implicitly.

## Validation before implementation

Add unit tests for all SAN preflight results, alias/domain reconciliation and
delete ordering. In `pv` with Pebble, cover initial issuance, alias addition,
alias removal, `certbot renew --dry-run`, and deletion after Apache reload.
Run these with Certbot staging; never consume production rate limits in tests.

## Implementation cut

The current repository still derives a lineage in three independent places:
`SSLService.Enable`, `CertificateService.Status`, and `WebsiteService.RenderVHost`.
The migration must replace all three in one change; changing only issuance would
produce a valid certificate that Apache and status cannot find.

1. Add a migration that gives each existing website a deterministic temporary
   certificate name and stores a unique `website_id` on certificate metadata.
   New websites receive a stable name only after their SQLite ID exists.
2. Add one service-level lineage resolver used by rendering, issuance, status,
   deploy hooks, aliases, deletion, and adoption. No CLI or renderer should
   reconstruct a lineage from a domain.
3. Change alias mutation to use a single journaled SAN reconciliation plan:
   issue/replace the complete SAN list, verify the live files, then render the
   vhost and commit the alias row. Failed Certbot work must remain explicit in
   the journal as irreversible/inconsistent work.
4. Delete the enabled vhost and reload Apache before `certbot delete`; retain
   a clear warning when Certbot deletion fails. Subscription deletion must use
   the same per-website operation rather than deleting certificate rows first.
5. Only after this resolver exists may `subscription adopt` attach an existing
   Certbot lineage to an imported website. Until then it may safely reconfigure
   renewal, but must not claim that it serves the imported certificate.
