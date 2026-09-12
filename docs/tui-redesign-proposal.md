# TUI redesign proposal

## Purpose

The default `provctl` command should make safe hosting administration
discoverable without weakening the CLI. The TUI follows the interaction model
of `branchctl`: a focused picker first, explicit modal forms for writes,
centred confirmation overlays, and visible progress for long-running work.
All reads and writes remain service calls behind `ui.Deps` and run through
`tea.Cmd`.

## Screen flow

### Subscription picker

Startup is a full-screen, filterable subscription list rather than the current
four-panel dashboard. Each row shows name, status, PHP version, UID, home,
domain count, and quota summary. `Enter` opens the selected subscription.

The picker provides `n` create, `e` edit metadata, `s` suspend/resume, `a`
archive, and `d` permanent delete. Creation and editing use forms; archive and
delete use confirmation overlays. Permanent deletion requires typing the exact
subscription name, matching the CLI safety gate.

### Subscription workspace

After selection, the workspace has a stable two-column layout:

```text
Metadata / subscription       Detail of selected domain
Domains and subdomains        Logs
                              Output / operation progress
```

Metadata lists status, Unix identity, home, PHP version, SSH mode, and quotas.
The domain list contains primary domains and aliases, with enabled/TLS/type
state. The detail panel shows the complete domain configuration, including
document root, upstream or redirect target, TLS expiry and redirect policy.

`Left` and `Right` move between the left and right panel on the same row.
`Up` and `Down` select or scroll within a focused panel. `Esc` returns from
the workspace to the picker. `Tab` and vi bindings may remain as unadvertised
compatibility shortcuts, but arrows are the documented navigation.

## Interaction rules

`?` opens a scrollable, filterable help overlay. `/` filters the focused list;
while typing, the filter bar replaces the keybar. A retained filter displays
its query and match count. Key bindings have one source of truth, from which
both the adaptive one-line keybar and help rows are rendered. The keybar must
truncate or omit low-priority bindings, never wrap.

Every write opens a centred, ANSI-safe overlay. The overlay states the exact
target and consequence, renders current status or errors, and owns keyboard
input. Ordinary reversible changes accept `y`; destructive actions require the
target name. Long operations lock conflicting actions and display their
service-backed pipeline/progress in Output.

## Domain and document-root model

Website type belongs to a domain, not a subscription: `php-fpm`, `static`,
`proxy`, or `redirect`. A subscription can host all four. The domain create
form selects type and conditionally reveals proxy/redirect target fields.

The default document root remains
`<subscription-home>/sites/<domain>/public`. The redesign adds a domain-root
field and a service operation for changing it. Its value must resolve inside
the selected subscription home; it cannot silently move files. The operation
validates ownership and the path, renders Apache configuration, runs
`apachectl configtest`, and only then reloads Apache. A future data-move flow
must be a separate planned, confirmed operation. Subscription home itself is
not editable in the TUI.

## Delivery order

1. Replace the frame with exact outer-size rendering and layout tests at
   80x24, 100x28, and wide terminals.
2. Build the picker, key-binding source, help, filters, and modal overlay
   primitives.
3. Add subscription create/edit/status/archive/delete service seams and forms.
4. Build the workspace and domain actions: create, edit, aliases, enable,
   disable, logs, and safe document-root changes.
5. Add PHP selection, TLS enable/disable/status, then database, SSH, cron,
   backup, health, and reconcile views.

Each stage requires model tests with fake dependencies. Privileged workflows
are verified in `pv`; the container is restored to `clean` after each run.
