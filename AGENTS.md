# Repository Guidelines

## Project Structure & Module Organization

This repository currently contains the implementation specification in `docs/`:

- `docs/project-specification.md` is the binding v0.1 design and architecture.
- `docs/testing-cookbook.md` defines local, package, container, and integration test scenarios.

Implement the planned Go layout: wiring in `cmd/provctl/`; business code in `internal/`; TUI in `tui/`; templates in `templates/`; Debian packaging in `packaging/`; and releases in `.github/workflows/`. Keep domain types I/O-free. CLI and TUI must call services rather than OS or SQLite packages directly.

## Build, Test, and Development Commands

Once the Go project is initialized, use the repository Makefile as the standard entry point:

```bash
make test           # vet, staticcheck, race-enabled unit tests
make build          # build a trimpath, CGO-free binary in dist/
make golden-update  # deliberately refresh renderer golden files
make deb            # build the Debian package via scripts/build-deb.sh
```

Run `go test ./internal/arch/...` when changing package boundaries. Package checks described in the cookbook use `lintian` and `piuparts`; do not run them against production systems.

## Coding Style & Naming Conventions

Use Go 1.22+; use tabs and run `gofmt` on every changed Go file. Keep code, identifiers, comments, commits, and technical messages in English. Use lower-case package names; exported identifiers use `PascalCase`; unexported identifiers use `camelCase`. Define product paths and the `provctl` name centrally in `internal/meta`—do not scatter literals.

Use explicit command arguments through the `system.Commander` abstraction. Never invoke a shell (`sh -c` or `bash -c`) or interpolate user input into commands.

## Testing Guidelines

Use the standard `testing` package and `github.com/google/go-cmp`. Name tests `TestThing_Scenario` and prefer table-driven tests. Unit tests must run unprivileged, offline, and without Debian services; use `internal/system/fake` for filesystem, process, and rollback cases. Store renderer fixtures in `testdata/` and review golden-file diffs before committing. Run `make test` before every pull request.

## Commit & Pull Request Guidelines

No Git history is currently available to infer an existing commit convention. Use concise imperative subjects, for example `Add Apache vhost renderer`. Keep commits focused. Pull requests should explain the behavior change, link the relevant issue or specification section, list validation commands, and include terminal output or screenshots for CLI/TUI changes. Call out migrations, package changes, and any deviation from the binding specification.

## Security & Configuration

`provctl` is a root-facing provisioning tool. Preserve least privilege, never log credentials, and treat SQLite as the source of truth while generating system configuration as artifacts. Do not hard-code PHP versions; detect installed versions as specified.
