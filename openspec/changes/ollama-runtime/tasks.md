## 1. Module scaffolding

- [ ] 1.1 Create `runtime-ollama/go.mod` — module path
  `github.com/kostiantyn-matsebora/agent-ops-operator/runtime-ollama`, `go 1.25`
  (the official MCP SDK's floor), with `github.com/modelcontextprotocol/go-sdk`
  as the only direct dependency
- [ ] 1.2 Create `runtime-ollama/Dockerfile` — build in `golang:1.25`, ship a
  distroless or slim runtime image with `git`, `openssh-client`, `ca-certificates`
  and ordinary shell utilities, and NO domain tooling (no kubectl, no cloud CLI,
  no bundled MCP server); `HOME=/data/home`, non-root user
- [ ] 1.3 Record in `CLAUDE.md`'s build section that this module builds in
  `golang:1.25` while the rest stay on 1.23, so `go build ./...` in the shared
  1.23 container is expected to fail here
- [ ] 1.4 Confirm the manager needs no change: `WorkUnit` carries `agent`,
  `allowedTools`, `toolsMode`, `maxTurns`, `systemPrompt`, `runtimeContextId`,
  and `workDone` accepts `continuity`/`continuityReason`. If anything is
  missing, stop and raise it as a contract change

## 2. Work contract plumbing

- [ ] 2.1 `work.go`: the unit and done-report types mirroring
  `internal/dispatch.WorkUnit` and `internal/httpapi.workDone`, the long-poll
  `GET /work?convo&pod&wait=25`, and `POST /work/done` with the reference
  runtime's retry cadence
- [ ] 2.2 `main.go`: read the injected env (`CONTROL_URL`, `CONVO_ID`,
  `POD_NAME`, `REPO_URL`, `REPO_REF`, `RUNTIME_IDLE_TTL_M`, `HOME`,
  `MCP_CONFIG`, git auth) plus this runtime's own (`OLLAMA_URL`,
  `OLLAMA_MODEL`, `OLLAMA_NUM_CTX`, `OLLAMA_KEEP_ALIVE`, `OLLAMA_TIMEOUT_S`,
  `BASH_TIMEOUT_S`, `TOOL_OUTPUT_MAX`); exit non-zero when a required one is
  missing
- [ ] 2.3 Poll loop with idle TTL: exit 0 after `RUNTIME_IDLE_TTL_M` minutes
  without work
- [ ] 2.4 `repo.go`: clone/fetch the profile repo at `/data/workspace`, ssh and
  https auth as the reference runtime does; clear directory CONTENTS, never
  rmdir the mount point
- [ ] 2.5 Resolve the prompt: `promptText`, or `promptFile` read relative to the
  workspace with `promptVars` substituted; an empty prompt fails the run with a
  stated reason

## 3. Ollama client

- [ ] 3.1 `ollama.go`: `/api/chat` over `net/http` behind a `chat(ctx, messages,
  tools)` interface — hand-rolled, NOT `github.com/ollama/ollama/api` (Go 1.26
  floor, gin/cobra/sqlite3/bubbletea in its graph)
- [ ] 3.2 Always set `options.num_ctx` explicitly from `OLLAMA_NUM_CTX`; add a
  test asserting the field is present on every request — the server default
  truncates silently
- [ ] 3.3 Set `keep_alive` from `OLLAMA_KEEP_ALIVE`; stream responses and write
  assistant text to stdout as it arrives
- [ ] 3.4 Startup checks: `GET /api/tags` for reachability, `POST /api/show` for
  model presence and tool capability; one summary line to the log
- [ ] 3.5 Map transport, non-2xx and decode failures to run failures carrying the
  endpoint and the error in the result

## 4. Allowlist composition and the gate

- [ ] 4.1 `tools.go`: port `runtime-claude/tools.js` — frontmatter `tools:`
  reader (flow list, inline, block forms), keeping its "not a YAML parser,
  unreadable declares nothing" posture
- [ ] 4.2 Port `tools.test.js` cases to `tools_test.go`, plus merge/overwrite
  composition cases
- [ ] 4.3 Pattern matcher: exact name, trailing `*` prefix, `mcp__server__*`;
  a narrowing specifier such as `Bash(kubectl:*)` grants NOTHING and is logged —
  never widened to bare `Bash`
- [ ] 4.4 The gate is applied once, before the request: only allowed tools are
  advertised, and an empty allowlist advertises none
- [ ] 4.5 At run start, log every allowlist entry that no built-in and no
  connected MCP server provides, as unavailable on this runtime

## 5. Built-in tools

- [ ] 5.1 `builtin.go`: `Read`, `Grep`, `Glob`, `Edit`, `Write`, `Bash` with
  JSON Schema declarations for the model
- [ ] 5.2 Path confinement: resolve every path against `/data/workspace`, reject
  absolute paths, `..` escapes and symlinked escapes as tool errors; tests for
  each escape form
- [ ] 5.3 Output bounds: truncate results at `TOOL_OUTPUT_MAX` (default 64 KiB)
  and state the truncation in the result
- [ ] 5.4 `Bash`: run in the workspace with `BASH_TIMEOUT_S` (default 120),
  terminate on timeout and return the timeout as the tool result, bounded
  captured output
- [ ] 5.5 A malformed tool argument returns a readable tool error, never a panic
  or a failed run

## 6. MCP client

- [ ] 6.1 `mcpclient.go`: read `$MCP_CONFIG`, connect each server with the
  official SDK (stdio and streamable HTTP), `tools/list`, and expose them as
  `mcp__<server>__<tool>` with their JSON Schemas
- [ ] 6.2 `tools/call` execution with per-call timeout, results mapped into the
  message stream
- [ ] 6.3 A server that fails to connect is logged with the tools consequently
  unavailable; the run continues with the rest
- [ ] 6.4 Test against an in-process MCP server from the same SDK

## 7. The agent loop

- [ ] 7.1 `agent.go`: assemble `[system] + transcript + [prompt]`, where system
  is the runtime's own base plus the unit's `systemPrompt` appended
- [ ] 7.2 Loop to `unit.maxTurns`: chat → execute tool calls in order → append
  results → repeat until no tool calls
- [ ] 7.3 Stream a readable transcript to stdout: assistant text, `[tool]` lines
  with truncated arguments, and each result's outcome and size
- [ ] 7.4 The final assistant message is the reported `result`; turn-limit
  exhaustion reports `failed` with a stated reason and a non-empty result
- [ ] 7.5 A unit with a non-empty allowlist against a model without tool support
  fails naming the model and the limitation
- [ ] 7.6 Test the loop against a scripted fake Ollama: text-only, one tool call,
  a hallucinated tool name, malformed arguments, and turn-limit exhaustion

## 8. Context store

- [ ] 8.1 `contextstore.go`: one JSON file per context under
  `$HOME/.agentops/contexts/<id>.json` (`{id, conversation, created, updated,
  messages[]}`), ids of the form `oc-<12 hex>`, atomic write via temp + rename
- [ ] 8.2 Handle given → load, append, report `continuity: continued`; none
  given → create, report `new`; always report the handle that now exists
- [ ] 8.3 Gone-versus-slow: on a miss re-check at 500 ms / 1.5 s / 3 s; a read
  ERROR is unavailability of the store, never absence of the context
- [ ] 8.4 Confirmed absence fails the run with `continuity: unavailable`, a
  `continuityReason` naming the home volume, and a non-empty user-facing message
  telling the person to start a new conversation
- [ ] 8.5 Trimming: keep the system prompt and the current turn, drop oldest
  first to fit `OLLAMA_NUM_CTX`, and log what was dropped; trimming is NOT
  reported as `unavailable`
- [ ] 8.6 Tests: continue, create, slow-then-found, error-is-not-absent,
  confirmed-missing, and trimming

## 9. Chart

- [ ] 9.1 Add `extraRuntimes: []` to `chart/values.yaml` with a commented Ollama
  example (endpoint, model, `contextStorage: volume`) and the note that the chart
  deploys no model server
- [ ] 9.2 Render them from a sibling of `chart/templates/runtime.yaml`, resolving
  home and workspace claims through the SAME helper the default runtime uses,
  and defaulting `serviceAccountName` to the release runtime SA
- [ ] 9.3 Confirm an empty list renders byte-identically to the current chart
  (`helm template` diff)
- [ ] 9.4 Pin the rendering in `internal/integration/charttemplate_test.go`: one
  declared entry produces a second `AgentRuntime` with its image,
  `contextStorage` and env, sharing the runtime SA and the persistence claims
- [ ] 9.5 Bump the chart version and add a `CHANGELOG.md` entry describing
  `extraRuntimes` (feature, not breaking — empty default changes nothing)

## 10. Documentation

- [ ] 10.1 Write `docs/runtimes.md`: both shipped images side by side — tools,
  continuity, cost, latency, the env each reads, how to choose, how to write a
  third; state the Ollama operational facts plainly (per-model serialisation vs
  `MAX_ACTIVE_CONVERSATIONS`, `keep_alive` vs `RUNTIME_IDLE_TTL_M`, `num_ctx`,
  and that `agentops-shell` means the pod's shell)
- [ ] 10.2 `docs/contracts.md`: name the second reference implementation in the
  work contract section
- [ ] 10.3 `CLAUDE.md`: add `runtime-ollama/` to the map, add the routing row for
  runtime images → `docs/runtimes.md`, and record the Go 1.25 build container
- [ ] 10.4 `README.md`: only if the kind list, pitch, demo or install command
  changes — they do not; verify it stays at ≤150 lines untouched

## 11. Build, test and verify

- [ ] 11.1 `docker run --rm -v $PWD:/src -w /src/runtime-ollama golang:1.25 sh -c
  'go build ./... && go vet ./... && go test ./...'`
- [ ] 11.2 Confirm the other eight modules still build in `golang:1.23`
- [ ] 11.3 Build and push `agentops-runtime-ollama:0.1.0` (never overwrite a
  pushed tag)
- [ ] 11.4 Against a real Ollama endpoint: a text-only run, a run using a
  built-in tool, and a run using an MCP tool — checking pod logs for the tool
  lines and the manager for the reported result
- [ ] 11.5 Continuity: run a second unit on the same conversation and confirm
  `continuity: continued`; delete the context file and confirm the run FAILS with
  `unavailable` and a readable message rather than answering
- [ ] 11.6 Tool gating: a Pipeline binding `agentops-observe` only produces a run
  whose advertised tools exclude `Bash`
- [ ] 11.7 Cluster apply: `helm upgrade --dry-run=server` before applying the
  chart change, then verify with a live task signal to a claimed source
