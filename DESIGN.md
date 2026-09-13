# TUI Design Context

`provctl` is a root-facing server control plane. Its terminal interface must
prioritize orientation, scope, and safe changes over visual density.

## Navigation contract

- The full-screen subscription picker is the only place where a subscription
  is selected. `s` returns there from the normal workspace.
- A workspace shows one subscription only. Its metadata is informative, never
  focusable; Domains, Detail, Logs, and Output are the focusable panels.
- Detail is a read-only preview. Editing begins from the selected domain and
  uses a dedicated modal or full-screen editor, not hidden panel shortcuts.

## Information contract

- Picker rows show subscription state, active and disabled website counts,
  configured website/database quotas, and live disk use where measurable.
- Domain rows always pair the hostname with a bracketed type tag such as
  `[php-fpm]`, `[static]`, or `[proxy]`.
- Use concise English labels, the established cyan focus treatment, muted
  structural borders, and warning colour only for consequences or errors.

## Interaction contract

- Use arrows or `j`/`k` for list navigation and left/right for panels.
- Any write requires the shared confirmation and progress overlays.
- Output is durable history, never an automatic focus target after a change.
- I/O stays behind `Deps` and arrives through `tea.Cmd` messages.
