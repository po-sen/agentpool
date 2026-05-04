# AgentPool

AgentPool is a self-hosted execution pool for team AI agent runs.

It provides a runtime server where users can submit agent tasks, queue runs, let workers process them, and track lifecycle state through a simple HTTP API. AgentPool is the runtime layer; audit dashboards, analytics, billing, reporting, and long-term compliance UI are intentionally outside this repository.

## Current Status

AgentPool is currently an early MVP scaffold.

- Storage and queue are in-memory only.
- Runs complete through noop infrastructure implementations.
- There is no real Docker sandbox yet.
- The default model client is noop.
- Agent loop v1 supports a minimal JSON action protocol, snapshot workspace plumbing, and a sandbox-backed `run_shell` tool contract. The default noop sandbox does not execute commands yet.
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

Completed runs include the one-shot model response summary:

```json
{
  "id": "run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1",
  "status": "completed",
  "task": {
    "project_id": "demo",
    "prompt": "Update the API handler tests",
    "repository_url": "https://github.com/example/repo",
    "branch": "main"
  },
  "result": {
    "summary": "The requested handler tests were updated."
  },
  "workspace_changes": [
    {
      "path": "README.md",
      "status": "modified"
    }
  ],
  "steps": [
    {
      "name": "prepare",
      "status": "completed",
      "message": "Prepared policy, secrets, and source context",
      "started_at": "2026-04-30T08:00:00Z",
      "ended_at": "2026-04-30T08:00:00Z"
    },
    {
      "name": "agent",
      "status": "completed",
      "message": "Agent generated result summary",
      "started_at": "2026-04-30T08:00:00Z",
      "ended_at": "2026-04-30T08:00:01Z"
    }
  ],
  "created_at": "2026-04-30T08:00:00Z",
  "updated_at": "2026-04-30T08:00:01Z"
}
```

Failed runs may include the failure reason:

```json
{
  "id": "run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1",
  "status": "failed",
  "task": {
    "project_id": "demo",
    "prompt": "Update the API handler tests",
    "repository_url": "https://github.com/example/repo",
    "branch": "main"
  },
  "failure_reason": "run failed",
  "steps": [
    {
      "name": "prepare",
      "status": "completed",
      "message": "Prepared policy, secrets, and source context",
      "started_at": "2026-04-30T08:00:00Z",
      "ended_at": "2026-04-30T08:00:00Z"
    },
    {
      "name": "agent",
      "status": "failed",
      "message": "Agent execution failed",
      "started_at": "2026-04-30T08:00:00Z",
      "ended_at": "2026-04-30T08:00:01Z"
    }
  ],
  "created_at": "2026-04-30T08:00:00Z",
  "updated_at": "2026-04-30T08:00:01Z"
}
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

Agent loop turn limit defaults to `4` and can be overridden with:

```sh
AGENTPOOL_AGENT_MAX_TURNS=6 go run ./cmd/agentpool dev
```

## Model Providers

AgentPool currently supports these model providers:

- `noop`: local placeholder provider, used by default.
- `openai_compatible`: OpenAI-style Chat Completions endpoint for Ollama, vLLM, llama.cpp server, LocalAI, LM Studio, OpenRouter-like gateways, or internal model gateways.
- `openai`: official OpenAI API provider.
- `anthropic`: official Anthropic Claude API provider.
- `gemini`: official Google Gemini API provider.

Provider configuration uses:

```sh
AGENTPOOL_MODEL_PROVIDER
AGENTPOOL_MODEL_BASE_URL
AGENTPOOL_MODEL_NAME
AGENTPOOL_MODEL_API_KEY
AGENTPOOL_MODEL_TIMEOUT
AGENTPOOL_AGENT_MAX_TURNS
```

Local Ollama example:

```sh
ollama serve
ollama pull qwen2.5-coder:7b

AGENTPOOL_MODEL_PROVIDER=openai_compatible \
AGENTPOOL_MODEL_BASE_URL=http://localhost:11434/v1 \
AGENTPOOL_MODEL_NAME=qwen2.5-coder:7b \
go run ./cmd/agentpool dev
```

OpenAI example:

```sh
AGENTPOOL_MODEL_PROVIDER=openai \
AGENTPOOL_MODEL_NAME=gpt-4.1-mini \
AGENTPOOL_MODEL_API_KEY=... \
go run ./cmd/agentpool dev
```

Anthropic example:

```sh
AGENTPOOL_MODEL_PROVIDER=anthropic \
AGENTPOOL_MODEL_NAME=claude-sonnet-4-5 \
AGENTPOOL_MODEL_API_KEY=... \
go run ./cmd/agentpool dev
```

Gemini example:

```sh
AGENTPOOL_MODEL_PROVIDER=gemini \
AGENTPOOL_MODEL_NAME=gemini-2.5-flash \
AGENTPOOL_MODEL_API_KEY=... \
go run ./cmd/agentpool dev
```

Use `openai_compatible` with a local or internal endpoint for air-gapped environments. Do not configure external providers when outbound internet is disabled.

## Workspace Sources

Workspace is a run-level concept for sources such as snapshot upload, git clone, or mounted workspace. Core AgentPool currently supports no workspace access and the snapshot source model/materializer. A real snapshot upload or storage backend is not implemented yet, so the default noop snapshot store returns "not found".

Without a workspace request, no workspace is prepared:

```json
{
  "prompt": "Explain DDD repository"
}
```

The explicit no-workspace form is also accepted:

```json
{
  "prompt": "Explain DDD repository",
  "workspace": {
    "type": "none"
  }
}
```

Snapshot workspace requests use a future snapshot ID:

```json
{
  "prompt": "Inspect the uploaded workspace and run the tests.",
  "workspace": {
    "type": "snapshot",
    "snapshot_id": "wsnap_example"
  }
}
```

Any other workspace source type, including `configured`, is rejected until an explicit source provider is implemented.

## Tools

AgentPool has an application-owned tool loop. Models can respond with a JSON `tool_call` action, the agent runner executes the tool through the `ToolRunner` port, and the tool result is fed back to the model for a final JSON answer.

The agent protocol accepts only `tool_call` and `final` JSON actions. Unknown JSON action types, malformed JSON, or multiple JSON objects are rejected as protocol errors and the model is asked to correct itself. Plain natural-language output is still accepted as a final summary for compatibility with local models.

The `run_shell` tool is sandbox-backed and is advertised only when a run has both workspace and sandbox context. In the default noop runtime the tool contract is wired, but command execution reports unavailable until a real sandbox command runner is implemented.

Tool calls use:

```json
{
  "type": "tool_call",
  "tool": "run_shell",
  "arguments": {
    "command": "go test ./...",
    "timeout_seconds": "30"
  }
}
```

Final answers use:

```json
{
  "type": "final",
  "summary": "Finished the task."
}
```

Tools are dynamically advertised. Runs without the required context do not expose unavailable tools to the model. Workspace path is separate from sandbox state, and sandbox execution is reserved for future side-effectful tools.

Example prompt:

```text
Inspect the workspace, run the test command if available, and summarize the result.
```

There is still no concrete shell execution backend, write access, Docker execution, Git mutation, network fetch, package install, or arbitrary host command execution.

After a workspace-backed run, AgentPool compares the materialized workspace with its snapshot baseline and records file-level `workspace_changes` with `added`, `modified`, or `deleted` status. This is a change summary only; patch contents, artifacts, and logs are future features.

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

Completed runs persist the agent result summary.

## Run Steps

Run steps are coarse execution timeline entries. The current worker records `prepare` for policy, secrets, and source context preparation, and `agent` for model and tool-loop execution. Steps explain the broad phase outcome, but they are not full logs. Tool logs, artifacts, and streaming output are future features.

## Architecture

- `internal/domain`: core business rules for runs and approvals.
- `internal/application`: use cases, agent runner, worker workflows, and ports.
- `internal/delivery`: HTTP and CLI entrypoints.
- `internal/infrastructure`: concrete technology implementations such as memory persistence, noop model provider, and noop integrations.
- `internal/bootstrap`: dependency wiring.
- `internal/runtime`: product-agnostic process helpers.
- `internal/test`: repository policy tests.

Delivery depends only on application inbound ports. Infrastructure implements domain and application outbound contracts. Bootstrap is the only place that wires concrete implementations.

The worker calls `internal/application/agent.Runner`. The agent runner calls `outbound.ModelClient`. Current and future model providers, including noop, OpenAI, Anthropic, or local models, belong under `internal/infrastructure/llm/*`.

## Development

Run the standard checks:

```sh
go test ./...
golangci-lint config verify
golangci-lint run ./...
pre-commit run --all-files
```

Contributor architecture and repository policy rules live in `AGENTS.md` and `internal/test`.
