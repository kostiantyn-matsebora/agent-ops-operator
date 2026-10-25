## [13.1.0] — 2026-08-26

**A second runtime: `agentops-runtime-ollama` 0.1.0, shipped as the `ollama`
bundle.** A local-model runtime over an Ollama endpoint you already run.

The RUNTIME is the harness here — the agent loop, tool dispatch, the
transcript and the context handle are its own. Ollama is called only for
the next message.

Building it needed no manager, CRD or work-contract change, which is the
finding: the contract is vendor-neutral.

### Added

- `chart/charts/ollama/` — OFF by default. `ollama.enabled: true` with
  `ollama.endpoint` and `ollama.model` renders one `AgentRuntime` named
  `ollama` through the parent's shared renderer, inheriting
  `global.agentops.runtimeDefaults`. A route selects it with
  `pipelines[].runtimeRef: ollama`. The bundle deploys no model server, and
  the render FAILS naming the key when the endpoint is missing. The model is
  optional while the server has exactly one pulled.
- The runtime implements the six built-in tools natively — `Read`, `Grep`,
  `Glob`, `Edit`, `Write`, `Bash` — so `agentops-observe` / `-shell` / `-edit`
  mean the same thing on both runtimes, connects the bound MCP servers from the
  same `mcp.json`, and keeps context as one transcript per conversation under
  `$HOME/.agentops/contexts/`, declared to `context-sync` by the bundle.
- `docs/runtimes/ollama.md`, and the Ollama chip on the landing page.

- **Every runtime has its own name, and `default` is a copy.** The claude
  bundle's CR is now named `claude`, the ollama one `ollama`, and the parent
  renders one more `AgentRuntime` named `default` — a copy of the runtime
  flagged `default: true` (`claude.default`, `ollama.default`,
  `runtimes[].default`), or of the first configured when none is. So a fresh
  install is unchanged (`default` is claude's copy), turning claude off and
  ollama on needs no rename, and `runtimeRef: claude` now works. Two flags fail
  the render. A `Pipeline` naming `runtimeRef: default` keeps resolving.

**Nothing changes for an install that does not enable the bundle.** The
rendered manifest is identical.
