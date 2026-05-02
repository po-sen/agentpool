# AGENTS.md

## Project

agentpool is a Go project with module path `github.com/po-sen/agentpool`.

## Layout

- `cmd/agentpool/`: application entrypoint.
- `internal/bootstrap/`: dependency wiring.
- `internal/config/`: runtime configuration.
- `internal/domain/`: domain concepts organized by aggregate/concept, such as `run` and `approval`. Domain code must not import application, adapter, infrastructure, HTTP, Docker, GitHub, OpenAI, or database packages.
- `internal/application/command/`: state-changing application use cases.
- `internal/application/query/`: read-only application use cases.
- `internal/application/workflow/`: worker and lifecycle workflows.
- `internal/application/port/inbound/`: use case contracts and application command/query/view DTOs exposed to inbound adapters. These DTOs must not have JSON, HTTP, DB, or external API tags.
- `internal/application/port/outbound/`: external capability contracts required by the application. Do not put a vague `RunRepository` here.
- `internal/adapters/inbound/`: inbound adapters such as HTTP and CLI.
- `internal/adapters/outbound/`: outbound port implementations such as memory, noop integrations, and ID generation.
- `internal/runtime/`: process/runtime helpers such as HTTP server lifecycle and logging. Runtime helpers must stay product-agnostic and must not implement business rules.
- `pkg/`: do not create unless there is a truly stable public API.

`README.md` exists because the user explicitly requested product and usage documentation. Keep it practical and honest.

## Product Boundary

AgentPool is a self-hosted runtime server for team AI agent runs. It accepts run submissions, manages run lifecycle, dispatches runs to workers, executes through an abstract `AgentExecutor`, prepares execution through an abstract `SandboxProvider`, and publishes lifecycle events through an abstract `EventPublisher`.

AgentPool does not own audit dashboards, analytics, billing, reporting, or long-term compliance UI.

Approval is currently only an application command boundary. Do not add real approval behavior until the approval domain and inbound route are intentionally designed.

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

Architecture import boundaries and domain import constraints are checked by `internal/test/import_policy_test.go`. Package topology and banned catch-all package names are checked by `internal/test/package_topology_test.go`. Required unit-test coverage is checked by `internal/test/unit_test_policy_test.go`. Keep `internal/test` limited to repository-level policy tests. No production package or other test package may import `internal/test`.
Inbound adapters may depend on `internal/application/port/inbound`, but they must not import domain models, outbound ports, outbound adapters, or concrete `command`, `query`, or `workflow` packages. Outbound adapters may depend on outbound ports and domain models, but they must not import inbound ports or delivery adapters. Runtime helpers must not depend on product layers. Bootstrap owns concrete wiring.

Domain imports are allowlisted. Production code may directly import `internal/domain/...` only from domain packages, `application/command`, `application/query`, `application/workflow`, `application/port/outbound`, and `adapters/outbound`. Inbound ports, inbound adapters, runtime, config, bootstrap, and cmd must not directly import domain packages.

Domain concepts are isolated from each other. A package under `internal/domain/<concept>` must not import another `internal/domain/<other-concept>` package. Aggregates should reference other aggregates by ID/value, not by importing the other aggregate package. Do not add `internal/domain/shared`; if a value is needed across concepts, keep it as a primitive or define it at the application boundary until a stronger domain boundary is proven.

Run persistence roles are intentionally split:

- `internal/domain/run.Repository`: pure DDD repository for the `Run` aggregate. It owns only `Save` and `FindByID`. Do not add list views, pagination, filters, queue behavior, status-conditional persistence, DB transaction semantics, or outbox behavior to it.
- `internal/application/port/outbound.RunStateStore`: application persistence coordination port. It owns `SaveIfStatus` for optimistic lifecycle writes.
- `internal/application/port/outbound.RunReader`: read/query port. It owns `List`.
- `internal/application/port/outbound.RunQueue`: worker dispatch queue. It owns `Enqueue` and `Dequeue`.
- Concrete adapters may implement several small interfaces with one type when appropriate. `memory.RunRepository` is allowed as a concrete adapter name because it implements `run.Repository`, `outbound.RunStateStore`, and `outbound.RunReader`.

Application dependencies are directed. `application/port/inbound` and `application/port/outbound` must stay implementation-free and must not import command, query, or workflow packages. `application/command`, `application/query`, and `application/workflow` must not import each other. They may depend on application ports and domain repository interfaces. Keep domain-to-view mapping local to the command or query package that returns the view; do not create a shared application helper package for it.

Unit tests are mandatory for domain packages, application port packages, application command/query/workflow packages, inbound adapters, outbound adapters, bootstrap, config, and runtime helpers. Every production `.go` file under `internal/` must have a same-directory companion test file with the same basename, such as `run_queue.go` and `run_queue_test.go`; `internal/test` is the repository policy-test exception. `cmd/agentpool` stays thin and is exempt unless business logic is added there. Application unit tests must use test-local fakes for ports instead of importing concrete adapters. Test imports are checked by `internal/test/import_policy_test.go`.

Keep DTO ownership explicit:

- Adapter DTOs describe external formats such as HTTP JSON, DB rows, or third-party API payloads.
- Application DTOs describe use case input/output such as commands, queries, and views.
- Domain models protect business state and invariants; do not expose them directly as HTTP responses or request bodies.

Run locally:

```sh
go run ./cmd/agentpool version
go run ./cmd/agentpool dev
go run ./cmd/agentpool server
go run ./cmd/agentpool worker
```

Use `dev` for local demos while the memory repository and queue are the only adapters. `server` and `worker` run as separate processes and do not share in-memory state.

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
