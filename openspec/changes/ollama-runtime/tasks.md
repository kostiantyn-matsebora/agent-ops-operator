## 1. Module scaffolding

- [x] 1.1 Create `runtimes/ollama/go.mod` — module path
  `github.com/kostiantyn-matsebora/agent-ops-operator/runtimes/ollama`, `go 1.25`
  (the official MCP SDK's floor — RE-VERIFY it before pinning), with
  `github.com/modelcontextprotocol/go-sdk` as the only direct dependency
- [x] 1.2 Create `runtimes/ollama/Dockerfile` — `golang:1.25` build stage, a
  `debian:bookworm-slim` runtime stage with `git`, `openssh-client`,
  `ca-certificates` and ordinary shell utilities, NO domain tooling (no kubectl,
  no cloud CLI, no bundled MCP server), non-root user, the
  `org.opencontainers.image.source` LABEL, and `$BUILDPLATFORM`/`TARGETARCH`
  so it builds multi-arch. Confirm `.github/components.sh images` lists
  `runtime-ollama` with this Dockerfile and `modules` lists the module
- [x] 1.3 Start the second persistent build container, `agentops-go125`, from
  `golang:1.25` with the SAME mounts as `agentops-go` (repo at its real path,
  the worktrees parent, both named cache volumes); record it in
  `.claude/rules/build-test.md` so `go build ./...` failing on this module in
  the 1.23 container is expected
- [x] 1.4 Confirm the manager needs no change: `WorkUnit` carries `agent`,
  `allowedTools`, `toolsMode`, `maxTurns`, `systemPrompt`, `runtimeContextId`,
  and `workDone` accepts `continuity`/`continuityReason`; `runtimepod` injects
  `HOME=/data/context`, `WORKSPACE` and `MCP_CONFIG`; `context-sync` proxies
  `CONTROL_URL`. If anything is missing, stop and raise it as a contract change

## 2. Work contract plumbing

- [x] 2.1 `work.go`: the unit and done-report types mirroring
  `internal/dispatch.WorkUnit` and `internal/httpapi.workDone`, the long-poll
  `GET /work?convo&pod&wait=25`, and `POST /work/done` with the reference
  runtime's retry cadence
- [x] 2.2 `main.go`: read the injected env (`CONTROL_URL`, `CONVO_ID`,
  `POD_NAME`, `REPO_URL`, `REPO_REF`, `RUNTIME_IDLE_TTL_M`, `HOME`, `WORKSPACE`,
  `MCP_CONFIG`, `GIT_AUTH_TYPE`/`GIT_SSH_KEY`/`GIT_TOKEN`) plus this runtime's
  own (`OLLAMA_URL`, `OLLAMA_MODEL`, `OLLAMA_NUM_CTX`, `OLLAMA_KEEP_ALIVE`,
  `OLLAMA_TIMEOUT_S`, `BASH_TIMEOUT_S`, `TOOL_OUTPUT_MAX`); exit non-zero
  naming a required one that is missing
- [x] 2.3 Poll loop with idle TTL: exit 0 after `RUNTIME_IDLE_TTL_M` minutes
  without work
- [x] 2.4 `repo.go`: clone/fetch the profile repo at `/data/workspace`, ssh and
  https auth as the reference runtime does; clear directory CONTENTS, never
  rmdir the mount point
- [x] 2.5 Resolve the prompt: `promptText`, or `promptFile` read relative to the
  workspace with `promptVars` substituted; an empty prompt fails the run with a
  stated reason

## 3. Ollama client

- [x] 3.1 `ollama.go`: `/api/chat` over `net/http` behind a `chat(ctx, messages,
  tools)` interface — hand-rolled, NOT `github.com/ollama/ollama/api` (Go 1.26
  floor, gin/cobra/sqlite3/bubbletea in its graph)
- [x] 3.2 Always set `options.num_ctx` explicitly from `OLLAMA_NUM_CTX`; add a
  test asserting the field is present on every request — the server default
  truncates silently
- [x] 3.3 Set `keep_alive` from `OLLAMA_KEEP_ALIVE`; stream responses and write
  assistant text to stdout as it arrives
- [x] 3.4 Startup checks: `GET /api/tags` for reachability, `POST /api/show` for
  model presence and tool capability; one summary line to the log
- [x] 3.5 Map transport, non-2xx and decode failures to run failures carrying the
  endpoint and the error in the result

## 4. Allowlist composition and the gate

- [x] 4.1 `tools.go`: port `runtimes/claude/tools.js` — frontmatter `tools:`
  reader (flow list, inline, block forms), keeping its "not a YAML parser,
  unreadable declares nothing" posture
- [x] 4.2 Port `tools.test.js` cases to `tools_test.go`, plus merge/overwrite
  composition cases
- [x] 4.3 Pattern matcher: exact name, trailing `*` prefix, `mcp__server__*`;
  a narrowing specifier such as `Bash(kubectl:*)` grants NOTHING and is logged —
  never widened to bare `Bash`
- [x] 4.4 The gate is applied once, before the request: only allowed tools are
  advertised, and an empty allowlist advertises none
- [x] 4.5 At run start, log every allowlist entry that no built-in and no
  connected MCP server provides, as unavailable on this runtime

## 5. Built-in tools

- [x] 5.1 `builtin.go`: `Read`, `Grep`, `Glob`, `Edit`, `Write`, `Bash` with
  JSON Schema declarations for the model
- [x] 5.2 Path confinement: resolve every path against `/data/workspace`, reject
  absolute paths, `..` escapes and symlinked escapes as tool errors; tests for
  each escape form
- [x] 5.3 Output bounds: truncate results at `TOOL_OUTPUT_MAX` (default 64 KiB)
  and state the truncation in the result
- [x] 5.4 `Bash`: run in the workspace with `BASH_TIMEOUT_S` (default 120),
  terminate on timeout and return the timeout as the tool result, bounded
  captured output
- [x] 5.5 A malformed tool argument returns a readable tool error, never a panic
  or a failed run

## 6. MCP client

- [x] 6.1 `mcpclient.go`: read `$MCP_CONFIG`, connect each server with the
  official SDK (stdio and streamable HTTP), `tools/list`, and expose them as
  `mcp__<server>__<tool>` with their JSON Schemas
- [x] 6.2 `tools/call` execution with per-call timeout, results mapped into the
  message stream
- [x] 6.3 A server that fails to connect is logged with the tools consequently
  unavailable; the run continues with the rest
- [x] 6.4 Test against an in-process MCP server from the same SDK

## 7. The agent loop

- [x] 7.1 `agent.go`: assemble `[system] + transcript + [prompt]`, where system
  is the runtime's own base plus the unit's `systemPrompt` appended
- [x] 7.2 Loop to `unit.maxTurns`: chat → execute tool calls in order → append
  results → repeat until no tool calls
- [x] 7.3 Stream a readable transcript to stdout: assistant text, `[tool]` lines
  with truncated arguments, and each result's outcome and size
- [x] 7.4 The final assistant message is the reported `result`; turn-limit
  exhaustion reports `failed` with a stated reason and a non-empty result
- [x] 7.5 A unit with a non-empty allowlist against a model without tool support
  fails naming the model and the limitation
- [x] 7.6 Test the loop against a scripted fake Ollama: text-only, one tool call,
  a hallucinated tool name, malformed arguments, and turn-limit exhaustion

## 8. Context store

- [x] 8.1 `contextstore.go`: one JSON file per context under
  `$HOME/.agentops/contexts/<id>.json` (`{id, conversation, created, updated,
  messages[]}`), ids of the form `oc-<12 hex>`, atomic write via temp + rename
- [x] 8.2 Handle given → load, append, report `continuity: continued`; none
  given → create, report `new`; always report the handle that now exists
- [x] 8.3 Gone-versus-slow: on a miss re-check at 500 ms / 1.5 s / 3 s; a read
  ERROR is unavailability of the store, never absence of the context
- [x] 8.4 Confirmed absence fails the run with `continuity: unavailable`, a
  `continuityReason` naming the context volume, and a non-empty user-facing
  message telling the person to start a new conversation
- [x] 8.5 Trimming: keep the system prompt and the current turn, drop oldest
  first to fit `OLLAMA_NUM_CTX`, and log what was dropped; trimming is NOT
  reported as `unavailable`
- [x] 8.6 Tests: continue, create, slow-then-found, error-is-not-absent,
  confirmed-missing, and trimming

## 9. Chart: the `ollama` bundle

- [x] 9.1 Create `chart/charts/ollama/` — `Chart.yaml`, `values.yaml`
  (`enabled: false`, `name: ollama`, `image`, `env` with `OLLAMA_URL` and
  `OLLAMA_MODEL` left for the adopter and the tuning knobs commented,
  `contextSync.paths: [".agentops/contexts/**"]`, an OPTIONAL
  `credentialsSecret`), and `templates/runtime.yaml` calling the parent's
  `agentops.renderRuntime` exactly as `chart/charts/claude/` does; the values
  comment states the bundle deploys no model server
- [x] 9.2 Add the dependency to `chart/Chart.yaml` with
  `condition: ollama.enabled`; run `.github/scripts/serviceaccount-guard.py`
  and the publication guard — the bundle renders no account and names no real
  endpoint
- [x] 9.3 Confirm `ollama.enabled: false` renders byte-identically to the
  current chart (`helm template` diff)
- [x] 9.4 Pin the rendering in
  `platform/manager/internal/integration/charttemplate_test.go`: the enabled
  bundle produces a second `AgentRuntime` with its image, env and
  `contextSync`, inheriting `contextStorage`, `idleTtlMinutes` and `resources`
  from the defaults, and renders no ServiceAccount
- [x] 9.5 Confirm the default-runtime guard: `claude.enabled: false` +
  `ollama.name: default` renders; `claude.enabled: false` + `ollama.name: ollama`
  with a route naming no `runtimeRef` FAILS the render
- [x] 9.6 Bump the chart minor and add the `docs/CHANGELOG.md` entry (feature,
  not breaking — the bundle is off by default and changes nothing)

## 10. Build, publish and verify

- [x] 10.1 `docker exec -i -w "$PWD/runtimes/ollama" agentops-go125 sh -c
  'go build ./... && go vet ./... && go test ./...'` from the worktree
- [x] 10.2 Confirm every other module still builds in `agentops-go` (1.23) via
  the `components.sh modules` loop, and that CI's matrix picks the right Go
  version for this one
- [ ] 10.3 Publish by tag: `git tag runtime-ollama-v0.1.0 && git push origin
  runtime-ollama-v0.1.0`; then check the REGISTRY, flip the new package to
  public in the UI, and confirm both architectures landed (never overwrite a
  pushed tag)
- [x] 10.4 Deploy the worktree's chart (helmfile with the worktree `chartPath`,
  `helm upgrade --dry-run=server` first) with the bundle enabled against a real
  Ollama endpoint: a text-only run, a run using a built-in tool, and a run using
  an MCP tool — checking pod logs for the tool lines and the manager for the
  reported result
- [x] 10.5 Continuity: run a second unit on the same conversation and confirm
  `continuity: continued`; confirm the sidecar snapshot holds
  `.agentops/contexts/`; delete the context file and confirm the run FAILS with
  `unavailable` and a readable message rather than answering
- [x] 10.6 Tool gating: a Pipeline binding `agentops-observe` only produces a
  run whose advertised tools exclude `Bash`, and an MCP call outside the bound
  toolsets is refused by the egress proxy as well as by the gate
- [x] 10.7 Rollback: disable the bundle, confirm an inflight conversation reports
  the missing runtime on its status and a new one on the re-pointed route runs
  on the reference runtime

## 11. Documentation

Both halves, ticked separately, and this section is the last one before the
change is finished.

**Reference docs:**

- [x] 11.1 `docs/contracts.md`: name the second reference implementation in the
  work contract section, and what building it found about the contract's
  vendor-neutrality
- [x] 11.2 `docs/concepts.md`: the runtime section names both images where it
  names one; `docs/claude.md` gets one line pointing at the Ollama page
- [x] 11.3 `.claude/rules/structure.md`: `ollama` in the `runtimes/` row and a
  short entry under the runtime section; `.claude/rules/build-test.md`: the
  `agentops-go125` container (if 1.3 did not already land it)

**The adopter site:**

- [x] 11.4 Write `docs/runtimes/ollama.md` — a RUNTIME page, the kind
  `docs/CLAUDE.md` gains in this task: what it executes, what it needs, where
  its context lives, what it costs — carrying the
  `<!-- generated: renders bundle=ollama -->` marker: what Ollama is here (an
  endpoint you already run — the bundle deploys none), enabling the bundle with
  worked values (`ollama.endpoint`, `ollama.model`),
  the env it reads, what the runtime supports (tools, continuity, cost,
  latency), how to choose between the two images, and the operational facts
  stated plainly — per-model serialisation vs `MAX_ACTIVE_CONVERSATIONS`,
  `keep_alive` vs `idleTtlMinutes`, `num_ctx`, `agentops-shell` means the pod's
  shell, model sizes worth trying
- [x] 11.5 `docs/_data/nav.yml`: a new **Runtimes** group holding the page; `docs/CLAUDE.md`: the runtime page kind beside the integration one, and the `renders` marker's applicability
- [x] 11.6 `docs/index.md`: an Ollama chip in "Works with" linking the page, with
  `docs/assets/img/logos/ollama.svg` added the way the other four logos were;
  the "Runs Claude Code" claim amended to "Runs Claude Code — or a local model";
  a "Why agent-ops?" row for keeping the cluster's data in the cluster
- [x] 11.7 `README.md`: mirror the three — the claims line, the "Works with"
  line, the integrations table row — and verify it stays within the 215-line
  budget
- [x] 11.8 `docs/installation.md`: the bundle in the bundles list with its
  `enabled` flag and the two values an adopter must decide; the hand-written
  `runtimes:` Ollama example becomes the bundle's values
- [x] 11.9 `docs/introduction.md` / `docs/guides/agent-runtime.md`: where they
  say "a local model", link the page — nothing more, the default install is
  unchanged
- [x] 11.10 Run `python3 .github/scripts/docs-generate.py` and commit what it
  regenerates (the renders table; `cr-reference.md` is unaffected — no CRD
  changed), then `python3 .github/scripts/publication-guard.py` — the page names
  a placeholder endpoint, never a real one
