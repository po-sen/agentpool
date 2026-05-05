# AgentPool

AgentPool is a self-hosted execution pool for team AI agent runs.

It provides a runtime server where users can submit agent tasks, queue runs, let workers process them, and track lifecycle state through a simple HTTP API. AgentPool is the runtime layer; audit dashboards, analytics, billing, reporting, and long-term compliance UI are intentionally outside this repository.

## Current Status

AgentPool is currently an early MVP scaffold.

- Storage and queue are in-memory only.
- Runs complete through noop infrastructure implementations.
- The default sandbox provider is noop. A dev-only Docker sandbox can be enabled explicitly to verify sandbox-backed shell commands.
- The default model client is noop.
- Agent loop v1 supports a minimal JSON action protocol and read-only uploaded-file tools. The default runtime has no command-capable sandbox and does not expose `run_shell`.
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
    "prompt": "Summarize the current task"
  }'
```

Example response:

```json
{
  "id": "run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1",
  "status": "queued",
  "task": {
    "project_id": "demo",
    "prompt": "Summarize the current task"
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
    "prompt": "Summarize the current task"
  },
  "result": {
    "summary": "noop model response"
  },
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
    "prompt": "Summarize the current task"
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
  "prompt": "Write unit tests for the worker"
}
```

`repository_url` and `branch` are accepted as run task metadata only. They are not used to clone code in the current MVP.

Metadata example:

```json
{
  "project_id": "demo",
  "prompt": "Review the submitted task",
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

Sandbox provider defaults to `noop`. Local command sandbox verification can be enabled with:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=alpine:3.20 \
go run ./cmd/agentpool dev
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
AGENTPOOL_SANDBOX_PROVIDER
AGENTPOOL_SANDBOX_IMAGE
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

## Workspace And Files

The current MVP supports prompt-only JSON runs and multipart runs with already-authorized uploaded text files. `repository_url` and `branch` are metadata only.

Product applications own file authorization, file selection, and product ACLs before submitting a run. AgentPool receives the selected files, writes them to an ephemeral per-run temp workspace, and exposes only read-only file tools to the agent.

Prompt-only JSON runs still prepare no workspace:

```json
{
  "prompt": "Explain DDD repository"
}
```

Upload run files with multipart form data:

```sh
curl -sS -X POST http://localhost:8080/v1/runs \
  -F 'prompt=Inspect the uploaded files and summarize them' \
  -F 'files=@README.md' \
  -F 'files=@internal/application/workflow/worker.go'
```

Uploaded files are currently limited to UTF-8 text files with safe relative names, at most 10 files, 1 MiB per file, and 5 MiB total. Supported extensions are `.txt`, `.md`, `.json`, `.yaml`, `.yml`, `.go`, `.py`, `.js`, and `.ts`.

The runtime does not persist uploaded files beyond the run workspace cleanup. AgentPool does not currently accept archive uploads, mounted directories, or git checkout as workspace input. It does not provide file mutation, workspace diffing, archive materialization, or product file permissions.

## Tools

AgentPool has an application-owned tool loop. Models can respond with a JSON `tool_call` action, the agent runner executes the tool through the `ToolRunner` port, and the tool result is fed back to the model for a final JSON answer.

The agent protocol accepts only `tool_call` and `final` JSON actions. Unknown JSON action types, malformed JSON, or multiple JSON objects are rejected as protocol errors and the model is asked to correct itself. Plain natural-language output is still accepted as a final summary for compatibility with local models.

When uploaded files are present, the agent can discover them with `list_files` and read selected text files with `read_file`. File contents are not injected into the initial prompt by default.

The `run_shell` tool is sandbox-backed and is advertised only when a run has workspace context and a command-capable sandbox. In the default runtime the sandbox provider is `noop`, so `run_shell` is not available.

Tool calls use this provider-neutral shape when a tool is available:

```json
{
  "type": "tool_call",
  "tool": "tool_name",
  "arguments": {
    "key": "value"
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

## Tool Call History

Completed runs expose in-memory `tool_calls` so users can inspect what the agent actually did. Each record includes the tool name, arguments, bounded result content, error flag, and start/end timestamps.

Example response snippet:

```json
{
  "tool_calls": [
    {
      "name": "run_shell",
      "arguments": {
        "command": "pwd && ls -la"
      },
      "result": "exit_code: 0\nstdout:\n/workspace\n...",
      "is_error": false,
      "started_at": "2026-05-04T16:59:50Z",
      "ended_at": "2026-05-04T17:00:22Z"
    }
  ]
}
```

This is useful for verifying `list_files`, `read_file`, and dev Docker `run_shell` execution. Shell stdout, stderr, timeout, and exit code are included because the shell tool formats them into the tool result. Tool call history is stored only in the current in-memory run state for now, and each result is truncated before it is stored.

Example prompt:

```text
Summarize the task and explain the next implementation step.
```

There is still no write access, Git mutation, network fetch, package install, or arbitrary host command execution. Shell commands are available only when the dev Docker sandbox is explicitly enabled.

## Dev Docker Sandbox

Docker-backed shell execution is available only for local sandbox verification. It is opt-in:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=alpine:3.20 \
go run ./cmd/agentpool dev
```

Submit an uploaded-file run that asks the agent to use both file tools and shell when available:

```sh
curl -sS -X POST http://localhost:8080/v1/runs \
  -F 'prompt=List files, read README.md, then run pwd and ls -la if run_shell is available.' \
  -F 'files=@README.md'
```

With the default `noop` sandbox, `list_files` and `read_file` are available when files are uploaded, but `run_shell` is unavailable. With `AGENTPOOL_SANDBOX_PROVIDER=docker`, the worker prepares a command-capable sandbox for uploaded-file runs, and `run_shell` runs through:

```text
docker run --rm --network none -v <workspace>:/workspace:ro -w /workspace <image> /bin/sh -lc <command>
```

Docker must be installed and running. The uploaded workspace is mounted read-only at `/workspace`, networking is disabled with `--network none`, and no secrets are passed into the container. This provider is dev-only and does not provide write-back, persistent storage, or production sandbox hardening.

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
