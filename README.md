# AgentPool

AgentPool is a self-hosted execution pool for team AI agent runs.

It provides a runtime server where users can submit agent tasks, queue runs, let workers process them, and track lifecycle state through a simple HTTP API. AgentPool is the runtime layer; audit dashboards, analytics, billing, reporting, and long-term compliance UI are intentionally outside this repository.

## Current Status

AgentPool is currently an early MVP scaffold.

- Storage and queue are in-memory only.
- Runs complete through noop infrastructure implementations.
- The default sandbox provider is noop. A dev-only Docker sandbox can be enabled explicitly to verify sandbox-backed shell commands.
- The default model client is noop.
- Agent loop v1 supports a minimal JSON action protocol, a `workspace` metadata tool, and opt-in dev Docker `sandbox_exec`. The default runtime has no command-capable sandbox and does not expose `sandbox_exec`.
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

Failed runs keep the public `failure_reason` sanitized and may include safe diagnostics:

```json
{
  "id": "run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1",
  "status": "failed",
  "task": {
    "project_id": "demo",
    "prompt": "Summarize the current task"
  },
  "failure_reason": "run failed",
  "failure_code": "agent_max_turns",
  "failure_message": "agent reached max turns",
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

## Failed Run Diagnostics

`failure_code` is a safe machine-readable code, and `failure_message` is a short safe explanation. Raw provider errors, stack traces, API keys, and hidden provider details are intentionally not exposed through the API.

Failed runs can include partial `agent_turns` and `tool_calls` when the agent invoked the model or tools before failing:

```json
{
  "status": "failed",
  "failure_reason": "run failed",
  "failure_code": "agent_max_turns",
  "failure_message": "agent reached max turns",
  "agent_turns": [
    {
      "index": 1,
      "status": "protocol_error",
      "message": "model response did not match AgentPool action protocol"
    },
    {
      "index": 2,
      "status": "tool_call",
      "action_type": "tool_call",
      "tool_name": "workspace"
    },
    {
      "index": 5,
      "status": "max_turns",
      "message": "agent reached max turns"
    }
  ],
  "tool_calls": [
    {
      "name": "workspace",
      "arguments": {
        "operation": "list",
        "area": "all",
        "path": "."
      },
      "result": "files:\n/workspace/input/README.md",
      "is_error": false
    }
  ]
}
```

Diagnostics are stored only in the current in-memory run state for now. There is no streaming, persistent database, or raw internal/provider error log in the HTTP API.

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
go run ./cmd/agentpool run --prompt "Use sandbox_exec to calculate 234 * 887123 with sh."
go run ./cmd/agentpool get run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
go run ./cmd/agentpool list
go run ./cmd/agentpool cancel run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
```

- `agentpool dev`: starts the HTTP API and an embedded worker in the same process.
- `agentpool server`: starts only the HTTP API server.
- `agentpool worker`: starts only the worker process.
- `agentpool version`: prints version info.
- `agentpool run`: submits a run through the HTTP API and waits for a terminal result by default.
- `agentpool get`: fetches one run.
- `agentpool list`: lists known runs.
- `agentpool cancel`: cancels one run before it reaches a terminal state.

`server` and `worker` are separate process modes intended for future persistent infrastructure implementations. With the current in-memory repository and queue, separate processes do not share state.

The local testing CLI is an HTTP client. Start `dev` in one terminal so the server and worker share in-memory state:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=alpine:3.20 \
go run ./cmd/agentpool dev
```

Then submit and wait for a run from another terminal:

```sh
go run ./cmd/agentpool run \
  --prompt "Use sandbox_exec to calculate 234 * 887123 with sh."
```

Upload files with repeated `--file` flags:

```sh
go run ./cmd/agentpool run \
  --prompt "Inspect README.md and summarize it" \
  --file README.md
```

Debug output includes the recorded `agent_system_prompt`, full `agent_turns`, and full `tool_calls`:

```sh
go run ./cmd/agentpool run \
  --prompt "Use sandbox_exec to calculate 234 * 887123 with sh." \
  --debug
```

Use JSON output when scripting:

```sh
go run ./cmd/agentpool run \
  --prompt "Use sandbox_exec to calculate 234 * 887123 with sh." \
  --json
```

The CLI address defaults to `http://localhost:8080`. Override it with `--addr` or `AGENTPOOL_CLI_ADDR`.

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

Product applications own file authorization, file selection, and product ACLs before submitting a run. AgentPool creates an ephemeral per-run temp workspace for every run. Prompt-only runs get an empty workspace; uploaded-file runs get a workspace populated with the selected files.

Prompt-only JSON runs prepare an empty workspace:

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

The runtime does not persist workspace contents beyond cleanup. AgentPool does not currently accept archive uploads, mounted directories, or git checkout as workspace input. AgentPool does not mutate run inputs or product files. The writable `/workspace/work` area is ephemeral run-local storage.

## Tools

AgentPool has an application-owned tool loop. Models can respond with a JSON `tool_call` action, the agent runner validates the requested tool against the tools advertised for that run, executes valid tools through the `ToolRunner` port, and feeds the tool result back to the model for a final JSON answer.

The agent protocol accepts only `tool_call` and `final` JSON actions. Models should return exactly one JSON object with no markdown fences. For compatibility, AgentPool normalizes whole-response fenced JSON blocks and simple scalar values that can safely become strings, such as `{"type":"final","summary":true}` becoming summary `"true"` and numeric tool arguments becoming string arguments. AgentPool does not extract JSON from arbitrary prose. Unknown JSON action types, malformed JSON, unsupported fields, nested argument values, or multiple JSON objects are rejected as protocol errors with targeted correction feedback. Plain natural-language output is still accepted as a final summary for compatibility with local models.

AgentPool exposes exactly two model-facing tools:

- `workspace`: metadata/control-plane. It lists files and stats paths, but does not read file contents.
- `sandbox_exec`: execution/data-plane. It runs commands through a command-capable sandbox.

Each run gets a workspace with two areas:

```text
/workspace/input  read-only run inputs
/workspace/work   read-write agent working directory
```

Run attachments are materialized under `/workspace/input` by default. File contents are inspected through `sandbox_exec`, for example with `sed`, `cat`, `grep`, or scripts written under `/workspace/work`. `sandbox_exec` is advertised only when a command-capable sandbox is available. In the default runtime the sandbox provider is `noop`, so only `workspace` is available.

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

Advertised tools include minimal argument hints in the system prompt while execution still receives string arguments:

```text
Available tools:
- workspace: Lists workspace files and stats workspace paths without reading file contents.
  Arguments:
  - operation (required): Operation to run. Supported values: "list" or "stat". Example: list
  - area (optional): Workspace area to inspect. Supported values: "input", "work", or "all". Example: input
  - path (optional): Safe relative path inside the selected area. Example: README.md
- sandbox_exec: Runs a shell command inside the prepared sandbox with /workspace/work as the working directory.
  Arguments:
  - command (required): Shell command to run inside the sandbox. Example: sed -n '1,160p' /workspace/input/README.md
  - timeout_seconds (optional): Optional timeout in seconds. Must be a positive integer and no more than the configured maximum. Example: 10
  - max_output_bytes (optional): Optional maximum combined stdout and stderr bytes returned by the tool. Example: 65536
```

Final answers use:

```json
{
  "type": "final",
  "summary": "Finished the task."
}
```

Tools are dynamically advertised. Runs without the required context do not expose unavailable tools to the model. The model may only call tools listed under `Available tools`; unknown or unadvertised tool names are rejected by the agent runtime before `ToolRunner` dispatch. Those requests appear in `agent_turns` as invalid tool-call diagnostics, not in `tool_calls`.

The run response includes bounded `agent_system_prompt` debug metadata after an agent session starts. It shows the exact provider-neutral system prompt used for the run, including the advertised tools:

```json
{
  "agent_system_prompt": "AgentPool is running a task.\nAvailable tools:\n- none\n..."
}
```

Workspace path is separate from sandbox state, and sandbox execution is reserved for future side-effectful tools.

## Agent Turn Diagnostics

Run diagnostics are intentionally split:

- `steps`: coarse lifecycle timeline, such as `prepare` and `agent`.
- `agent_turns`: per-model-turn diagnostics from the agent loop.
- `tool_calls`: concrete tool execution history and tool results.

`agent_turns` records safe, bounded metadata for each model turn: index, status, parsed action type, tool name for tool calls, safe message, optional response preview, and timestamps. This helps debug `agent_max_turns` without turning steps into a full model transcript.

Example snippet:

```json
{
  "status": "failed",
  "failure_code": "agent_max_turns",
  "failure_message": "agent reached max turns",
  "steps": ["..."],
  "agent_turns": [
    {
      "index": 1,
      "status": "protocol_error",
      "message": "model response did not match AgentPool action protocol",
      "response_preview": "{\"type\":\"tool_result\"}"
    },
    {
      "index": 2,
      "status": "tool_call",
      "action_type": "tool_call",
      "tool_name": "workspace"
    },
    {
      "index": 5,
      "status": "max_turns",
      "message": "agent reached max turns"
    }
  ],
  "tool_calls": ["..."]
}
```

`agent_turns` are in-memory only for now. `response_preview` is UTF-8 safe and truncated, and it is not a full model transcript. Raw provider errors, secrets, stack traces, and product ACL decisions are intentionally not exposed.

## Tool Call History

Completed runs, and failed runs with available partial history, expose in-memory `tool_calls` so users can inspect what the agent actually did. Each record includes the tool name, arguments, bounded result content, error flag, and start/end timestamps.

Example response snippet:

```json
{
  "tool_calls": [
    {
      "name": "sandbox_exec",
      "arguments": {
        "command": "pwd && ls -la"
      },
      "result": "exit_code: 0\ntimed_out: false\nstdout:\n/workspace/work\n...",
      "is_error": false,
      "started_at": "2026-05-04T16:59:50Z",
      "ended_at": "2026-05-04T17:00:22Z"
    }
  ]
}
```

This is useful for verifying `workspace` metadata calls and dev Docker `sandbox_exec` execution. Command stdout, stderr, timeout, exit code, and truncation marker are included because `sandbox_exec` formats them into the tool result. Tool call history is stored only in the current in-memory run state for now, and each result is truncated before it is stored.

Example prompt:

```text
Summarize the task and explain the next implementation step.
```

There is still no write-back to run inputs, product files, Git, or external systems. Agents may write temporary/generated files only under `/workspace/work`, and workspace contents are cleaned up after the run. Shell commands are available only when the dev Docker sandbox is explicitly enabled.

## Dev Docker Sandbox

Docker-backed shell execution is available only for local sandbox verification. It is opt-in:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=alpine:3.20 \
go run ./cmd/agentpool dev
```

The default dev image is intentionally minimal. Commands available inside `sandbox_exec` depend on `AGENTPOOL_SANDBOX_IMAGE`; use a richer image when tasks need Python, Go, Node.js, or other tooling.

Submit a prompt-only run that asks the agent to use sandbox execution:

```sh
curl -sS -X POST http://localhost:8080/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Use sandbox_exec to calculate 234 * 887123 with sh."}'
```

When Docker sandboxing is enabled, `agent_system_prompt` should show `sandbox_exec` under `Available tools`, and the command runs inside Docker with `/workspace/work` as the current writable directory.

Submit an uploaded-file run that asks the agent to inspect metadata and read contents through sandbox execution:

```sh
curl -sS -X POST http://localhost:8080/v1/runs \
  -F 'prompt=List files with workspace, read README.md with sandbox_exec, then run pwd and ls -la.' \
  -F 'files=@README.md'
```

With the default `noop` sandbox, `workspace` is available but `sandbox_exec` is unavailable. With `AGENTPOOL_SANDBOX_PROVIDER=docker`, the worker prepares a command-capable sandbox for the run workspace, and `sandbox_exec` runs through:

```text
docker run --rm --network none -v <input>:/workspace/input:ro -v <work>:/workspace/work:rw -w /workspace/work <image> /bin/sh -lc <command>
```

Docker must be installed and running. Run inputs are mounted read-only at `/workspace/input`, agent work is mounted read-write at `/workspace/work`, networking is disabled with `--network none`, and no secrets are passed into the container. This provider is dev-only and does not provide persistent storage or production sandbox hardening.

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
