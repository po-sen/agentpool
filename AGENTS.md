# AGENTS.md

## Project

agentpool is a Go project with module path `github.com/po-sen/agentpool`.

## Layout

- `cmd/agentpool/`: application entrypoint.
- `internal/bootstrap/`: dependency wiring.
- `internal/config/`: runtime configuration.
- `internal/domain/`: domain concepts organized by aggregate/concept, such as `run` and `approval`. Domain code must not import application, delivery, infrastructure, runtime, HTTP, Docker, GitHub, OpenAI, or database packages.
- `internal/application/command/`: state-changing application use cases.
- `internal/application/query/`: read-only application use cases.
- `internal/application/workflow/`: worker and lifecycle workflows.
- `internal/application/agent/`: application-owned agent runner and agent loop. This is AgentPool product logic.
- `internal/application/port/inbound/`: use case contracts and application command/query/view DTOs exposed to delivery code. These DTOs must not have JSON, HTTP, DB, or external API tags.
- `internal/application/port/outbound/`: external capability contracts required by the application. Do not put a vague `RunRepository` here.
- `internal/delivery/`: inbound delivery mechanisms such as HTTP API and CLI. Delivery code translates external formats into application commands/queries and translates application views back into external responses.
- `internal/infrastructure/`: concrete external technology implementations such as persistence, LLM providers, sandbox providers, event publishers, policy engines, secret brokers, and ID generators.
- `internal/infrastructure/llm/`: model provider implementations such as noop, OpenAI-compatible endpoints, OpenAI, Anthropic, Gemini, or local models.
- `internal/infrastructure/tool/`: tool execution implementations. Tool implementations must implement `internal/application/port/outbound.ToolRunner`.
- `internal/runtime/`: process/runtime helpers such as HTTP server lifecycle and logging. Runtime helpers must stay product-agnostic and must not implement business rules.
- `pkg/`: do not create unless there is a truly stable public API.

`README.md` exists because the user explicitly requested product and usage documentation. Keep it practical and honest.

## Product Boundary

AgentPool is a self-hosted runtime server for team AI agent runs. It accepts prompt-only run submissions and already-authorized uploaded run files, manages run lifecycle, dispatches runs to workers, executes through an application-owned agent runner, calls model providers through an abstract `ModelClient`, and publishes lifecycle events through an abstract `EventPublisher`.

AgentPool does not own audit dashboards, analytics, billing, reporting, long-term compliance UI, product ACLs, or product file permission systems. Product-side applications must authorize and select files before submitting them to a run. AgentPool receives already-authorized run files and enforces workspace confinement and runtime tool safety.

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
Delivery code may depend on `internal/application/port/inbound`, but it must not import domain models, outbound ports, infrastructure, or concrete `command`, `query`, or `workflow` packages. Infrastructure code may depend on outbound ports and domain models, but it must not import inbound ports, delivery packages, or concrete application handlers. Runtime helpers must not depend on product layers. Bootstrap owns concrete wiring.

Domain imports are allowlisted. Production code may directly import `internal/domain/...` only from domain packages, `application/agent`, `application/command`, `application/query`, `application/workflow`, `application/port/outbound`, and `infrastructure`. Inbound ports, delivery, runtime, config, bootstrap, and cmd must not directly import domain packages.

Domain concepts are isolated from each other. A package under `internal/domain/<concept>` must not import another `internal/domain/<other-concept>` package. Aggregates should reference other aggregates by ID/value, not by importing the other aggregate package. Do not add `internal/domain/shared`; if a value is needed across concepts, keep it as a primitive or define it at the application boundary until a stronger domain boundary is proven.

Run persistence roles are intentionally split:

- `internal/domain/run.Repository`: pure DDD repository for the `Run` aggregate. It owns only `Save` and `FindByID`. Do not add list views, pagination, filters, queue behavior, status-conditional persistence, DB transaction semantics, or outbox behavior to it.
- `internal/application/port/outbound.RunStateStore`: application persistence coordination port. It owns `SaveIfStatus` for optimistic lifecycle writes.
- `internal/application/port/outbound.RunReader`: read/query port. It owns `List`.
- `internal/application/port/outbound.RunQueue`: worker dispatch queue. It owns `Enqueue` and `Dequeue`.
- Concrete infrastructure implementations may implement several small interfaces with one type when appropriate. `memory.RunRepository` is allowed as a concrete infrastructure name because it implements `run.Repository`, `outbound.RunStateStore`, and `outbound.RunReader`.

Application dependencies are directed. `application/port/inbound` and `application/port/outbound` must stay implementation-free and must not import command, query, or workflow packages. `application/command`, `application/query`, and `application/workflow` must not import each other. They may depend on application ports and domain repository interfaces. Keep domain-to-view mapping local to the command or query package that returns the view; do not create a shared application helper package for it.

Agent behavior belongs in `internal/application/agent`, including agent loop mechanics, provider-neutral action parsing, tool orchestration, tool call history capture, agent turn diagnostics, and safe failure classification for model/protocol/tool-loop failures. Agent code must not prepare sandboxes directly. Model provider integrations belong in `internal/infrastructure/llm`. Provider packages must implement `internal/application/port/outbound.ModelClient`; they must not contain agent loops, planning, tool orchestration, evaluation logic, or workflow state transitions. Tool execution implementations belong under `internal/infrastructure/tool/*` and must implement `internal/application/port/outbound.ToolRunner`. Tool implementations should not duplicate run history persistence. Workspace path is not the same as sandbox state. Tools must be dynamically advertised only when their required context exists. The agent runtime must validate `tool_call.tool` against the advertised tool set before dispatch and must not call `ToolRunner` for unknown or unadvertised tools. The system prompt should explicitly forbid invented tool names. Read-only uploaded-file tools may be advertised when `outbound.ToolContext.WorkspacePath` is non-empty and `outbound.ToolContext.WorkspaceHasFiles` is true. `run_shell` requires a workspace path and command-capable sandbox, not uploaded files. Do not add arbitrary shell, unrestricted filesystem, network, Git mutation, package install, or command-execution tools without an intentionally designed sandbox and policy gate. The only current command-capable provider is the opt-in dev Docker sandbox. Do not add provider-specific SDK types, JSON wire structs, or provider-specific concepts to application ports.

The agent action parser may normalize whole-response fenced JSON blocks and simple scalar values that are string-compatible, such as boolean or numeric final summaries and flat tool argument values. Parser compatibility must not extract JSON from arbitrary prose. Protocol correction feedback must be specific, bounded, and safe; do not expose raw provider errors, secrets, stack traces, hidden provider internals, or product ACL decisions in correction messages.

The domain `Run` may store provider-neutral tool call history, provider-neutral agent turn diagnostics, the bounded provider-neutral agent system prompt used for debugging, and safe provider-neutral failure diagnostics. `steps` are coarse run lifecycle records. `agent_turns` are model-loop diagnostics owned by `internal/application/agent` and stored by the domain `Run`. `tool_calls` are tool execution records. Unknown or unadvertised tool requests belong in `agent_turns`, not `tool_calls`. Do not merge these concepts into one field. Workflow maps classified agent failures into `Run` diagnostics and may persist partial tool call and agent turn history for failed runs when available. Tool result content, agent turn previews, and stored system prompts must stay bounded before they are exposed through HTTP. Do not log secrets, raw provider errors, hidden provider internals, product ACL decisions, or product permission details into `agent_turns`, `tool_calls`, `failure_message`, `agent_system_prompt`, or other run diagnostics.

Workspace is a per-run runtime invariant. Workspace creation is worker/runtime lifecycle, not a model tool; do not add a `create_workspace` tool. Product apps own file ACL and file selection; AgentPool materializes accepted attachments into an ephemeral per-run temp workspace, or creates an empty temp workspace for prompt-only runs, and passes its path through `outbound.ToolContext`. Empty workspaces are valid. `WorkspaceHasFiles` controls read-only file tool exposure. Workspace must not be coupled to worker-local environment paths, and `WorkspacePath` must not be stored on `Sandbox`. Use `outbound.ToolContext` to pass workspace and sandbox context to tools. Sandbox lifecycle must stay lazy and should only be used for sandbox-backed tools.

Sandbox command tools must execute through `internal/application/port/outbound.SandboxCommandRunner`; they must not call host `os/exec` directly from tool packages. Concrete sandbox providers may implement command execution when an isolation boundary and policy gate are intentionally designed.

The default sandbox provider is noop. The Docker sandbox provider is dev-only and may invoke the Docker CLI with `os/exec`, but it must never execute requested commands directly on the host. `run_shell` must only be advertised when a workspace path exists and the sandbox reports `SupportsCommands`.

Do not add persistent workspace storage, archive materialization, workspace diffing, file mutation, unrestricted shell execution, or worker-local path workspace providers without explicit design review. Future workspace source implementations should be explicit product-authorized input sources, not product ACL logic inside AgentPool. Local path access is not core behavior.

Git checkout is not implemented in the current MVP. `repository_url` and `branch` are metadata only; any future Git workspace source must be intentionally designed later.

Unit tests are mandatory for domain packages, application port packages, application command/query/workflow packages, delivery packages, infrastructure packages, bootstrap, config, and runtime helpers. Every production `.go` file under `internal/` must have a same-directory companion test file with the same basename, such as `run_queue.go` and `run_queue_test.go`; `internal/test` is the repository policy-test exception. `cmd/agentpool` stays thin and is exempt unless business logic is added there. Application unit tests must use test-local fakes for ports instead of importing concrete infrastructure. Test imports are checked by `internal/test/import_policy_test.go`.
Tests under `internal/` must use the same package as the production code, such as `package bootstrap`, not external test packages such as `package bootstrap_test`. External `_test` packages are reserved for future stable public APIs outside `internal/`.

Keep DTO ownership explicit:

- Delivery DTOs describe external request/response formats such as HTTP JSON.
- Infrastructure DTOs describe storage records or third-party API payloads.
- Application DTOs describe use case input/output such as commands, queries, and views.
- Domain models protect business state and invariants; do not expose them directly as HTTP responses or request bodies.

Run locally:

```sh
go run ./cmd/agentpool version
go run ./cmd/agentpool dev
go run ./cmd/agentpool server
go run ./cmd/agentpool worker
```

Use `dev` for local demos while the memory repository and queue are the only infrastructure implementations. `server` and `worker` run as separate processes and do not share in-memory state.

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
