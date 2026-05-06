# AgentPool

AgentPool is a self-hosted execution pool for team AI agent runs.

It provides a runtime server where users can submit agent tasks, queue runs, let workers process them, and track lifecycle state through a simple HTTP API. AgentPool is the runtime layer; audit dashboards, analytics, billing, reporting, and long-term compliance UI are intentionally outside this repository.

## Current Status

AgentPool is currently an early MVP scaffold.

- Storage and queue are in-memory only.
- Runs complete through noop infrastructure implementations.
- The default sandbox provider is noop. A dev-only Docker sandbox can be enabled explicitly to verify sandbox-backed shell commands.
- The default model client is noop.
- Agent loop v1 supports a minimal JSON action protocol, a `workspace` source-staging tool, and opt-in dev Docker `sandbox_exec`. The default runtime has no command-capable sandbox and does not expose `sandbox_exec`.
- CLI uploads can expand selected files, directories, and small text archives into the run workspace for POC tasks.
- Changed or newly-created files under `/workspace` are captured as in-memory run artifacts before workspace cleanup.
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
      "name": "workspace",
      "status": "completed",
      "message": "Prepared empty workspace",
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
      "name": "workspace",
      "status": "completed",
      "message": "Prepared empty workspace",
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
      "message": "model response did not match AgentPool action protocol",
      "request_messages": [
        {"role": "developer", "content": "AgentPool is running one task..."},
        {"role": "user", "content": "do work"}
      ],
      "raw_response": "{\"type\":\"tool_result\",\"result\":\"hello\"}",
      "response_format": "json_object",
      "protocol_error_code": "unknown_action_type",
      "correction_message": "Protocol error:\nError code: unknown_action_type\nA previous model response was invalid because action type must be final or tool_call.\n\nReturn exactly one JSON object with only the allowed fields..."
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
        "operation": "list_sources"
      },
      "result": "sources:\n- source_id: input_001\n  path: README.md\n  virtual_path: /workspace/README.md\n  size_bytes: 123\n  staged: false\n",
      "is_error": false
    }
  ]
}
```

Diagnostics are stored only in the current in-memory run state for now. There is no server-push streaming, persistent database, or raw internal/provider error log in the HTTP API.

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
go run ./cmd/agentpool run --prompt "Use sandbox_exec to calculate 234 * 887123 with sh." --watch
go run ./cmd/agentpool watch run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
go run ./cmd/agentpool get run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
go run ./cmd/agentpool list
go run ./cmd/agentpool cancel run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
go run ./cmd/agentpool artifacts run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
go run ./cmd/agentpool artifact run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1 report.md
```

- `agentpool dev`: starts the HTTP API and an embedded worker in the same process.
- `agentpool server`: starts only the HTTP API server.
- `agentpool worker`: starts only the worker process.
- `agentpool version`: prints version info.
- `agentpool run`: submits a run through the HTTP API and waits for a terminal result by default.
- `agentpool run --watch`: submits a run and prints a polling timeline while waiting.
- `agentpool watch`: polls an existing run and prints new timeline rows until it reaches a terminal state.
- `agentpool get`: fetches one run.
- `agentpool list`: lists known runs.
- `agentpool cancel`: cancels one run before it reaches a terminal state.
- `agentpool artifacts`: lists captured `/workspace` artifacts for one run.
- `agentpool artifact`: prints one captured artifact body.

`run` and `watch` wait without a CLI-side time limit by default. Use `--timeout <duration>` when you want the CLI to stop waiting after a fixed duration; `--timeout 0` disables the limit.

`server` and `worker` are separate process modes intended for future persistent infrastructure implementations. With the current in-memory repository and queue, separate processes do not share state.

The local testing CLI is an HTTP client. Start `dev` in one terminal so the server and worker share in-memory state:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=agentpool/poc-sandbox:latest \
go run ./cmd/agentpool dev
```

Then submit and wait for a run from another terminal:

```sh
go run ./cmd/agentpool run \
  --prompt "Use sandbox_exec to calculate 234 * 887123 with sh."
```

Use `--watch` when you want a visible timeline while the CLI waits:

```sh
go run ./cmd/agentpool run \
  --prompt "Use sandbox_exec to calculate 234 * 887123 with sh." \
  --watch
```

The watch output is a log-style polling timeline:

```text
2026-05-06 10:00:00 agent [turn 1] turn_status=model_waiting action=model
  message:
    waiting for model response

2026-05-06 10:00:01 agent [turn 1] turn_status=tool_call action=sandbox_exec
  message:
    model requested tool call
  request:
    command: wc -l /workspace/README.md
  response:
    type: tool_call
    tool: sandbox_exec

2026-05-06 10:00:02 tool [tool 1] tool_status=ok action=sandbox_exec
  request:
    command: wc -l /workspace/README.md
  response:
    12 /workspace/README.md
```

It uses the existing `GET /v1/runs/{id}` response, so it shows newly persisted run state rather than server-pushed events. Agent turn progress is persisted while the agent loop is running, including the current `model_response` turn before a model response arrives. Use `agentpool watch <run_id>` to follow a run submitted earlier.

Upload files with repeated `--file` flags:

```sh
go run ./cmd/agentpool run \
  --prompt "Inspect README.md and summarize it" \
  --file README.md
```

Upload a small directory or archive for POC repo inspection:

```sh
go run ./cmd/agentpool run \
  --prompt "Use workspace and sandbox_exec to inspect this project and summarize the Go packages." \
  --dir .

go run ./cmd/agentpool run \
  --prompt "Inspect this archived project and write report.md under /workspace." \
  --archive project.tar.gz
```

The CLI expands `--dir` and `.tar`, `.tar.gz`, `.tgz`, or `.zip` archives into multipart uploaded files. Paths are preserved relative to the selected directory or archive root. It skips `.git`, `.hg`, `.svn`, `node_modules`, `vendor`, `dist`, `build`, `target`, `.next`, `.turbo`, and `.cache`, and it does not follow symlinks.

List or fetch artifacts after a run:

```sh
go run ./cmd/agentpool artifacts run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1
go run ./cmd/agentpool artifact run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1 report.md
```

Debug output includes full `agent_turns` with provider-facing `request_messages`, and full `tool_calls`:

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
GET  /v1/runs/{id}/artifacts
GET  /v1/runs/{id}/artifacts/{path...}
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

Agent loop turn limit defaults to `8` and can be overridden with:

```sh
AGENTPOOL_AGENT_MAX_TURNS=10 go run ./cmd/agentpool dev
```

Sandbox provider defaults to `noop`. Local command sandbox verification can be enabled with:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=alpine:3.20 \
go run ./cmd/agentpool dev
```

Build the richer POC sandbox image for real local tasks:

```sh
docker build -t agentpool/poc-sandbox:latest docker/poc-sandbox
```

Then run dev mode with:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=agentpool/poc-sandbox:latest \
go run ./cmd/agentpool dev
```

The alpine image remains useful as a minimal smoke test. Available commands inside `sandbox_exec` depend on `AGENTPOOL_SANDBOX_IMAGE`.

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

AgentPool builds a provider-neutral model request with `instructions`, available tool definitions, and typed conversation turns and parts, such as task prompt, workspace context, assistant attempt, protocol correction, tool correction, native tool call, and tool result. Runtime correction turns use the provider-neutral `runtime` role so they are not represented as user-authored messages.

Provider adapters map that request into each provider's strongest native shape:

- OpenAI: `developer` messages for instructions/runtime corrections, `tools` function schemas, assistant `tool_calls`, and `role: "tool"` messages with `tool_call_id` for results.
- OpenAI-compatible: conservative `system` messages for instructions/runtime corrections, OpenAI-style `tools`, assistant `tool_calls`, and `role: "tool"` results. This is the best format for compatible servers that implement tool calling; endpoints without tool support should fail fast instead of entering a repeated JSON repair loop.
- Anthropic: top-level `system` for instructions/runtime corrections and invalid assistant attempts, `tools` with `input_schema`, assistant `tool_use` blocks, and user `tool_result` blocks. Anthropic does not use a literal `tool` role.
- Gemini: `system_instruction` for instructions/runtime corrections and invalid assistant attempts, `functionDeclarations`, model `functionCall` parts, and user `functionResponse` parts with the original function call id when available. Gemini does not consistently use a literal `tool` role across APIs.
- Noop: diagnostic-only provider; it echoes provider-neutral request messages and does not call real tools natively.

`request_messages` in run diagnostics are provider-facing request messages returned by the model adapter, not the provider-neutral canonical request. During the POC, instruction content is exposed directly in these diagnostics, and roles and ordering match the adapted provider request. For providers without a chat-message array, top-level instruction fields are represented as messages such as `system` or `system_instruction` for debugging.

## Workspace And Files

The current MVP supports prompt-only JSON runs and multipart runs with already-authorized uploaded files. The CLI can expand selected directories and common archives into multipart file uploads. `repository_url` and `branch` are metadata only.

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

Uploaded files must use safe relative names and are currently limited to at most 500 files, 50 MiB per file, and 200 MiB total. The HTTP multipart body limit is 256 MiB. Text-like uploads must be UTF-8. Supported text names and extensions include `.txt`, `.md`, `.csv`, `.json`, `.yaml`, `.yml`, `.xml`, `.go`, `.py`, `.js`, `.ts`, `.tsx`, `.jsx`, `.html`, `.css`, `.sh`, `.toml`, `.mod`, `.sum`, `.env.example`, `.gitignore`, `.dockerignore`, `Makefile`, and `Dockerfile`. Supported document and image extensions include `.pdf`, `.doc`, `.docx`, `.xls`, `.xlsx`, `.ppt`, `.pptx`, `.odt`, `.ods`, `.odp`, `.rtf`, `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.tif`, `.tiff`, `.bmp`, and `.heic`.

AgentPool does not mutate product files. The `/workspace` directory is an ephemeral mutable working copy during execution. Authorized inputs are staged into `/workspace` as copies. Before cleanup, AgentPool captures up to 100 changed or newly-created regular files from `/workspace` as in-memory artifacts, with a 1 MiB per-file limit and 10 MiB total limit. Unchanged staged inputs, directories, and symlinks are not captured.

Artifact list response:

```json
{
  "artifacts": [
    {
      "path": "report.md",
      "size_bytes": 123,
      "media_type": "text/markdown"
    }
  ]
}
```

Artifact content is available as raw bytes:

```sh
curl -sS http://localhost:8080/v1/runs/run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1/artifacts/report.md
```

## Tools

AgentPool has an application-owned tool loop. Models can respond with a JSON `tool_call` action, the agent runner validates the requested tool against the tools advertised for that run, executes valid tools through the `ToolRunner` port, and feeds the tool result back to the model for a final JSON answer.

The agent protocol accepts only `tool_call` and `final` JSON actions. Models must return exactly one JSON object with no markdown fences or surrounding prose. `final.summary` is the complete user-facing answer in the user's requested language, not a completion note. AgentPool converts simple scalar values that can safely become strings, such as `{"type":"final","summary":true}` becoming summary `"true"` and numeric tool arguments becoming string arguments. Plain natural-language output, multiple JSON objects, arrays, unknown JSON action types, malformed JSON, unsupported fields, and nested argument values are rejected as protocol errors with targeted correction feedback.

AgentPool exposes exactly two model-facing tools:

- `workspace`: control-plane. It lists authorized input sources, stages selected sources into `/workspace`, restores staged files from their source, and lists currently staged/generated workspace files. It does not read file contents.
- `sandbox_exec`: execution/data-plane. It runs commands through a command-capable sandbox from `/workspace`.

Each run gets one mutable workspace:

```text
/workspace  disposable agent working copy
```

Run attachments become authorized input sources first. They are not automatically readable by sandbox commands until the agent stages them with `workspace`. Once staged, files are normal mutable copies under `/workspace`; the agent may modify, move, delete, or restore source-backed files. `workspace` accepts safe relative paths or full virtual paths such as `/workspace/README.md`. It never returns host temp paths. `sandbox_exec` is advertised only when a command-capable sandbox is available. In the default runtime the sandbox provider is `noop`, so only `workspace` is available.

When `sandbox_exec` is available, exact or verifiable tasks should be checked through sandbox execution instead of guessed. That includes exact arithmetic, counting or searching files, transforming data, and running small scripts, tests, builds, or linters. Subjective discussion, design advice, brainstorming, and simple conversation can return a final JSON action directly when no command is needed.

AgentPool now advertises available tools through provider-native function/tool schemas when the configured provider supports them. The model should use the native tool mechanism first, because that preserves the provider's tool-call id and lets AgentPool return the result in the provider's expected tool-result channel.

The JSON tool-call action remains as a compatibility fallback when native tool calls are unavailable:

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
- workspace: Manages authorized input sources and staged files for the mutable /workspace.
  Arguments:
  - operation (required): Operation to run. Supported values: "list_sources", "stage", "restore", or "list". Example: list_sources
  - source_id (optional): Authorized source id to stage or restore. Example: input_001
  - path (optional): Safe relative /workspace path, or a full /workspace virtual path. Example: README.md
- sandbox_exec: Runs commands in a general-purpose sandbox from /workspace.
  Arguments:
  - command (required): Command to run from /workspace. Example: wc -l /workspace/README.md
  - timeout_seconds (optional): Optional timeout in seconds. Must be a positive integer and no more than the configured maximum. Example: 10
  - max_output_bytes (optional): Optional maximum combined stdout and stderr bytes returned by the tool. Example: 65536
```

Final answers use:

```json
{
  "type": "final",
  "summary": "Here is the answer the user asked for."
}
```

Tools are dynamically advertised. Runs without the required context do not expose unavailable tools to the model. The model may only call tools listed under `Available tools`; unknown or unadvertised tool names are rejected by the agent runtime before `ToolRunner` dispatch. Those requests appear in `agent_turns` as invalid tool-call diagnostics, not in `tool_calls`.

The run response does not include separate prompt metadata fields. During the POC, system/developer prompt diagnostics are exposed through `agent_turns[].request_messages`.

Workspace path is separate from sandbox state, and sandbox execution is reserved for future side-effectful tools.

## Agent Turn Diagnostics

Run diagnostics are intentionally split:

- `steps`: coarse lifecycle timeline, currently `workspace` and `agent`.
- `agent_turns`: per-model-turn diagnostics from the agent loop.
- `tool_calls`: concrete tool execution history and tool results.

`agent_turns` records bounded model-loop diagnostics for each model turn: index, status, parsed action type, tool name for tool calls, safe message, the provider-facing request messages returned by the model adapter, the raw model response content returned through the model port, response format classification, optional protocol error code, optional correction message sent back to the model, optional short response preview, and timestamps. Provider-facing request messages may include `tool_call_id` and `tool_name` when the provider format carries them. During the POC, instruction content is exposed in user-facing diagnostics. `response_format` can be `json_object`, `json_array`, `json_scalar`, `multiple_json_values`, `invalid_json`, `plain_text`, `markdown_fence`, or `empty`. Protocol repair keeps a bounded invalid assistant attempt in provider history and sends the repair instruction separately as a runtime instruction mapped by the provider adapter.

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
      "request_messages": [
        {"role": "developer", "content": "AgentPool is running one task..."},
        {"role": "user", "content": "do work"}
      ],
      "raw_response": "{\"type\":\"tool_result\",\"result\":\"hello\"}",
      "response_format": "json_object",
      "protocol_error_code": "unknown_action_type",
      "correction_message": "Protocol error:\nError code: unknown_action_type\nA previous model response was invalid because action type must be final or tool_call.\n\nReturn exactly one JSON object with only the allowed fields...",
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

`agent_turns` are in-memory only for now. `request_messages`, `raw_response`, `response_preview`, and `correction_message` are UTF-8 safe and truncated. `raw_response` is the provider-neutral model content or provider-neutral native tool-call summary, not a provider-specific SDK or HTTP payload. During the POC, raw system/developer prompts are exposed through `request_messages`; raw provider errors, secrets resolved by AgentPool, stack traces, hidden provider internals, and product ACL decisions are intentionally not exposed through user-facing diagnostics.

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
      "result": "exit_code: 0\ntimed_out: false\nstdout:\n/workspace\n...",
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

There is still no write-back to product files, Git, or external systems. Agents may freely change the disposable `/workspace` copy; bounded changed/new artifacts are captured before workspace contents are cleaned up after the run. Shell commands are available only when the dev Docker sandbox is explicitly enabled.

## Dev Docker Sandbox

Docker-backed shell execution is available only for local sandbox verification. It is opt-in:

```sh
AGENTPOOL_SANDBOX_PROVIDER=docker \
AGENTPOOL_SANDBOX_IMAGE=agentpool/poc-sandbox:latest \
go run ./cmd/agentpool dev
```

The default dev image is intentionally minimal. Build and use `agentpool/poc-sandbox:latest` when tasks need common CLI tools, `jq`, ripgrep, Git, Python 3, Node.js/npm, Go, or Make:

```sh
docker build -t agentpool/poc-sandbox:latest docker/poc-sandbox
```

Commands available inside `sandbox_exec` depend on `AGENTPOOL_SANDBOX_IMAGE`.

Submit a prompt-only run that asks the agent to use sandbox execution:

```sh
curl -sS -X POST http://localhost:8080/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Use sandbox_exec to calculate 234 * 887123 with sh."}'
```

When Docker sandboxing is enabled, `sandbox_exec` is advertised to the model under `Available tools`, and the command runs inside Docker with `/workspace` as the current writable directory. During the POC, user-facing debug output exposes provider-facing request messages, including instructions.

Submit an uploaded-file run that asks the agent to inspect metadata and read contents through sandbox execution:

```sh
curl -sS -X POST http://localhost:8080/v1/runs \
  -F 'prompt=List files with workspace, read README.md with sandbox_exec, then run pwd and ls -la.' \
  -F 'files=@README.md'
```

With the default `noop` sandbox, `workspace` is available but `sandbox_exec` is unavailable. With `AGENTPOOL_SANDBOX_PROVIDER=docker`, the worker enables `sandbox_exec` for the run workspace, and each command runs through:

```text
docker run --rm --network none -v <workspace>:/workspace:rw -w /workspace <image> /bin/sh -lc <command>
```

Docker must be installed and running. The mutable run workspace is mounted read-write at `/workspace`, networking is disabled with `--network none`, and no secrets are passed into the container. This provider is dev-only and does not provide persistent storage or production sandbox hardening.

## Run Artifacts

Completed and failed runs can expose in-memory artifacts captured from changed or newly-created `/workspace` files before workspace cleanup. This is POC-only storage, not persistent object storage.

```sh
curl -sS http://localhost:8080/v1/runs/run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1/artifacts
curl -sS http://localhost:8080/v1/runs/run_2f7b7f3b8ec0f65d6e079d6f4bd4e8c1/artifacts/report.md
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

Completed runs persist the agent result summary.

## Run Steps

Run steps are coarse execution timeline entries. The current worker records `workspace` for preparing the runtime workspace and `agent` for model and tool-loop execution. Steps explain the broad phase outcome, but they are not full logs. Tool logs and streaming output are future features.

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
