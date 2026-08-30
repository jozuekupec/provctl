# TUI design

`provctl` adopts the established Bubble Tea conventions from the sibling
`depo` project, adapted to a root-facing provisioning tool.

The TUI lives in `internal/ui` and is read-mostly: its dependencies expose
service-level reads and explicitly selected mutations, never SQLite or system
packages. The model is a value type with thematic state for subscriptions,
websites, details, output, help, and confirmation. All I/O is performed by
`tea.Cmd` through a `Deps` seam, so routing and rendering remain testable
without a terminal or root access.

Initial panels are Subscriptions, Websites, Detail, and Output. Key routing,
rendering, theme, and asynchronous refresh are separate files. Destructive
actions require a confirmation overlay; lengthy service operations display an
observable status/spinner. Features from the reference blueprint are adopted
only when provctl needs them.

Sources: `../depo/docs/go-tui.md` and
`/home/jozue/development/ai-recipes/terminal/go-tui-blueprint.md`.
