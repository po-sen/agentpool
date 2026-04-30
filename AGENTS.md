# AGENTS.md

## Project

agentpool is a Go project. The module path is currently `agentpool` because no remote repository is configured yet. Update the module path when the final import path is known.

## Layout

- `cmd/agentpool/`: application entrypoint.
- `internal/`: preferred location for private application packages.
- `pkg/`: use only for packages intentionally exported for external consumers.

Do not add `README.md` unless the user explicitly asks for it.

## Tooling

- Go version is initialized from the local toolchain in `go.mod`.
- Lint and formatting configuration lives in `.golangci.yml`.
- General pre-commit and commit message checks live in `.pre-commit-config.yaml`.

Install hooks:

```sh
pre-commit install
```

Run checks manually:

```sh
go mod tidy
go test ./...
golangci-lint config verify
golangci-lint run ./...
pre-commit run --all-files
pre-commit run --hook-stage commit-msg --commit-msg-filename <commit-msg-file>
```

## Commit Messages

Use Conventional Commits:

```text
type(scope): summary
```

Allowed types are `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, and `revert`.

## Development Notes

- Keep changes small and idiomatic Go.
- Prefer standard library packages until a dependency has clear value.
- Keep generated files out of manual edits.
- Run `go mod tidy` after dependency changes.
- Do not revert unrelated user changes in the working tree.
