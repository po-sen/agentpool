# AgentPool

AgentPool is a self-hosted execution pool for team AI agent runs.

It provides a runtime server where users can submit agent tasks, queue runs, let workers process them, and track lifecycle state through a simple HTTP API. AgentPool is the runtime layer; audit dashboards, analytics, billing, reporting, and long-term compliance UI are intentionally outside this repository.

## Current Status

AgentPool is currently an early MVP scaffold.

- Storage and queue are in-memory only.
- Runs complete through noop infrastructure implementations.
- There is no real Docker sandbox yet.
- There is no real AI model execution yet.
- There is no real GitHub PR creation yet.
- There is no persistent database or queue yet.

Use `agentpool dev` for local demos. It starts the API server and worker in the same process so they share the same in-memory state.

## Quick Start

Build the binary:

```sh
go build -o bin/agentpool ./cmd/agentpool
```

Start local dev mode:

```sh
go run ./cmd/agentpool dev
```

Check health:

```sh
curl http://localhost:8080/healthz
```

Submit a run:

```sh
curl -sS -X POST http://localhost:8080/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{
    "project_id": "demo",
    "prompt": "Update the API handler tests",
    "repository_url": "https://github.com/example/repo",
    "branch": "main"
  }'
```

Example response:

```json
{
  "id": "run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1",
  "status": "queued",
  "task": {
    "project_id": "demo",
    "prompt": "Update the API handler tests",
    "repository_url": "https://github.com/example/repo",
    "branch": "main"
  },
  "steps": [],
  "created_at": "2026-04-30T08:00:00Z",
  "updated_at": "2026-04-30T08:00:00Z"
}
```

List runs:

```sh
curl -sS http://localhost:8080/v1/runs
```

Get one run:

```sh
curl -sS http://localhost:8080/v1/runs/run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
```

Cancel a run:

```sh
curl -sS -X POST http://localhost:8080/v1/runs/run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1/cancel
```

Cancellation only applies before a run reaches a terminal state. In `dev` mode the noop worker may complete runs quickly.

## CLI

```sh
go run ./cmd/agentpool dev
go run ./cmd/agentpool server
go run ./cmd/agentpool worker
go run ./cmd/agentpool version
```

- `agentpool dev`: starts the HTTP API and an embedded worker in the same process.
- `agentpool server`: starts only the HTTP API server.
- `agentpool worker`: starts only the worker process.
- `agentpool version`: prints version info.

`server` and `worker` are separate process modes intended for future persistent infrastructure implementations. With the current in-memory repository and queue, separate processes do not share state.

## HTTP API

```text
GET  /healthz
POST /v1/runs
GET  /v1/runs
GET  /v1/runs/{id}
POST /v1/runs/{id}/cancel
```

Create-run request:

```json
{
  "project_id": "demo",
  "prompt": "Write unit tests for the worker",
  "repository_url": "https://github.com/example/repo",
  "branch": "main"
}
```

Only `prompt` is required. The prompt must be non-empty and no longer than 8000 characters.

## Configuration

The server listens on `:8080` by default.

Override the address with:

```sh
AGENTPOOL_HTTP_ADDR=127.0.0.1:9000 go run ./cmd/agentpool dev
```

## Run Lifecycle

The happy path is:

```text
queued -> preparing -> running -> completed
```

Other supported states:

```text
waiting_approval
failed
cancelled
```

## Architecture

- `internal/domain`: core business rules for runs and approvals.
- `internal/application`: use cases, worker workflows, and ports.
- `internal/delivery`: HTTP and CLI entrypoints.
- `internal/infrastructure`: concrete technology implementations such as memory persistence and noop integrations.
- `internal/bootstrap`: dependency wiring.
- `internal/runtime`: product-agnostic process helpers.
- `internal/test`: repository policy tests.

Delivery depends only on application inbound ports. Infrastructure implements domain and application outbound contracts. Bootstrap is the only place that wires concrete implementations.

## Development

Run the standard checks:

```sh
go test ./...
golangci-lint config verify
golangci-lint run ./...
pre-commit run --all-files
```

Contributor architecture and repository policy rules live in `AGENTS.md` and `internal/test`.
