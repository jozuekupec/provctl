# TUI Pattern Comparison

The requested `../examples/depo` directory is not present. This comparison
therefore uses the available `../examples/branchctl`, the closely matching
`../examples/dbctl` named by the shared cookbook, and `../depo` as the current
Codex-built reference.

## Patterns to adopt

- Keep `appModel` a value type with thematic sub-states and copy-on-write
  slices. `depo` demonstrates this especially well for output and panel state.
- Route keys through a dedicated `keys.go` and a mode enum. A single binding
  table should drive both handling and the visible keybar/help, preventing
  advertised shortcuts from diverging from behavior.
- Use a fixed minimum terminal size and focused, bordered panels. `depo`'s
  layout helpers provide stable widths, cursor visibility, truncation, and
  scroll markers instead of allowing long output to break the screen.
- Represent confirmation as an enum action and serializable state, never a
  closure. `branchctl` and `depo` both keep destructive actions explicit and
  cancellable.
- Give each asynchronous operation a bounded context and operation slot. A
  generation guard prevents a late result for an old selection from replacing
  the current detail; a spinner gives immediate feedback while it runs.

## Deliberate scope for provctl

Provctl remains read-mostly: it needs subscriptions, websites, details, logs,
health, and only confirmed enable/disable and suspend/resume actions. It does
not need `depo`'s vault, form, transfer, or multi-step deployment interfaces.
Long health and log reads will use the same cancellable operation-slot pattern,
but a pipeline progress tree is unnecessary until an operation exposes real
steps from its service layer.

## Current gap and implementation order

The first TUI implementation has the correct service seam and `tea.Cmd` I/O,
but lacks the shared layout, keybar/help source, minimum-size guard, themed
panels, and stale-result protection. M8 will add those foundations before its
terminal verification. This keeps it visually and behaviorally consistent with
the reference projects without copying features that do not belong to a
hosting-control UI.
