# TUI design

`provctl` opens its Bubble Tea terminal UI when invoked without a subcommand.
It is a keyboard-first administration surface, while the CLI remains the
automation interface.

## Navigation and screens

The initial screen is a full-screen subscription picker. It supports filtering
and presents subscription status, domain counts, quota usage, and identity.
`Enter` opens the selected subscription; `s` returns to the picker from any
workspace screen.

The workspace is read-mostly. Domains are the primary selectable resource and
show their hosting type (`php-fpm`, `static`, `proxy`, or `redirect`). `Enter`
opens a tabbed domain editor. The Detail panel is deliberately read-only;
domain, database, cron, SSH-key, backup, TLS, and subscription changes use
dedicated forms rather than panel-local mode switching. Long-running mutations
render their real plan steps in a progress popup and leave focus where the
operation began.

Settings is a tabbed form for `/etc/provctl/config.toml`. PHP versions are
selected from the detected installed versions. Every editable filesystem path
has direct text input and the shared picker on `Enter`.

## Interaction rules

- The binding table in `internal/ui/keys.go` is the single source of truth for
  key handling, the contextual keybar, and help.
- `?` opens contextual help; `/` filters the active list. A retained filter
  shows its query and match count.
- Shift+Left and Shift+Right change tabs. Plain arrows navigate a list or
  focused control; they do not change a tab or escape a picker.
- Writes always use a centred form or confirmation popup. Destructive actions
  require an explicit confirmation, including a typed target where appropriate.
- Popups use the shared small/medium/large size classes. Their footer is a
  reserved bottom block, never wrapped or clipped by content.

## Architecture and verification

The UI is a value-model Bubble Tea application in `internal/ui`. I/O is
performed only through `tea.Cmd` and `Deps` service seams; it does not call
SQLite or OS packages directly. Key routing, rendering, styles, and popup
state remain separate so model tests can exercise the UI without root or a
terminal. Generation guards prevent stale asynchronous reads from replacing
newer selections.

The shared path picker lists through a dependency-backed command, returns
absolute paths, treats `..` as a row, and confirms the chosen path separately
with Space or Alt+Enter. Service validation still enforces allowed roots.

Follow the personal Go TUI cookbook and sibling `depo` conventions when
changing this surface. Run `make test`; verify privileged changes in the
disposable Incus `pv` container and restore `clean` afterward.
