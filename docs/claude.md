# Claude runtime (subchart)

The reference agent runtime, shipped as a bundle: `claude-code` implementing the
operator's work contract.

It conforms to the [runtime specification](contracts/runtime.md), version
1.0.0.

`chart/charts/claude/` renders exactly two objects — one `AgentRuntime` named
`claude` and, when you supply a token, its credential `Secret`. The PARENT
renders the `AgentRuntime` named `default` as a copy of it, unless another
runtime is flagged `default: true`.

**ON by default**, unlike every other bundle here. It is what a fresh install
executes on. The other runtime bundles:

- `chart/charts/ollama/`, the local-model one — see
  [runtimes/ollama](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/ollama/).
- `chart/charts/copilot/`, GitHub Copilot through its SDK — see
  [runtimes/copilot](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/copilot/).

| Renders | When |
|---|---|
| `AgentRuntime` (named `claude`) | always, while `claude.enabled` |
| `AgentRuntime` named `default`, a copy of it | rendered by the PARENT while this is the flagged or first configured runtime |
| `Secret` (the model credential) | only when a token is supplied |

## Why a bundle at all

`AgentRuntime` used to be the parent chart's alone. A second vendor meant a
hand-written CR, and an install using one still carried this one's image
reference and credential shape.

**A vendor is domain, not substrate.** What stays exclusively the parent's is
the runtime DEFAULTS every runtime inherits and the FLOOR ServiceAccount — see
[concepts](concepts.md#the-substrate-the-runtime-defaults-and-the-floor).

## Turning it off

```yaml
claude:
  enabled: false
```

**The render then FAILS if anything still resolves to `default`**, naming the
missing runtime and the routes that needed it. That is deliberate: the
alternative is conversations reaching `Pending` forever with the reason in the
manager's log and nowhere an operator looks.

Two ways to satisfy it:

```yaml
# either declare a replacement answering to the same name...
runtimes:
  - name: default
    image: registry.example.com/my-runtime:1.0.0

# ...or give every route a runtime of its own
pipelines:
  - name: house-ops
    runtimeRef: my-runtime
```

The check reads no cluster, so it protects a GitOps render exactly as it
protects an interactive one.

## Values

**Everything is inherited from `global.agentops.runtimeDefaults`.** The keys
below override it for THIS runtime only, and any key those defaults accept is
accepted here.

| Key | Default | Is |
|---|---|---|
| `enabled` | `true` | whether this runtime renders at all |
| `name` | `default` | the `AgentRuntime` CR name — what a Pipeline declaring no `runtimeRef` resolves to |

The ordinary install sets nothing else. The parent's defaults already pin the
reference image, the resources, the idle TTL and the egress posture.

```yaml
claude:
  image: registry.example.com/my-agentops-runtime:1.4.0
  idleTtlMinutes: 10
  nodeSelector:
    kubernetes.io/arch: amd64
  egressMediation:
    enabled: false
  credentialsSecret:
    name: my-claude-secret
```

**Renaming it means every route needs a `runtimeRef`**, and the guard above will
say so rather than letting conversations queue.

## Deriving your own image

What an agent may REACH is wiring, so adding tooling to an image never grants it
anything — see
[concepts](concepts.md#runtime-images-are-generic).

Point `claude.image` at your derived image, or declare a second runtime beside
this one under the parent's `runtimes:` and select it per route.

## A tool call the model cannot FORM

A model writes its tool arguments as text, and that text is not always valid
JSON. When it is not, `runtime-claude` **discards the call before anything
runs** — no MCP server sees it, no allowlist refuses it, no request leaves
the pod.

That failure is invisible from outside. The run looks busy, and the model
tends to write the same broken call again, because nothing tells it which
character was wrong.

`runtime-claude` therefore counts them:

| Situation | What happens |
|---|---|
| a call whose arguments do not parse | logged, and counted |
| the run ANSWERS having made some | the answer carries a line saying how many never ran, and which tool |
| the SAME tool, the SAME arguments, `RUNTIME_UNPARSED_REPEAT_LIMIT` times in a row (default 5) | the run is ended and reported **failed**, naming the tool and quoting what was written |

**Consecutive and identical, both deliberately.** A model that varies its
arguments is trying something, and one whose next call parses has
recovered. The failure worth ending is the loop that cannot end, because
nothing about it changes.

**A run that recovers still says so.** Recovery usually means ABANDONING
the tool, not fixing the call. Twice out of twice observed, the model then
answered from what the session already held and the run was reported a
success.

The notice is appended to the answer, never substituted for it, because the
answer itself does not mention it.

**Failing is the point of the breaker.** Without one, a spin ends the same
way: a successful-looking run, answered from memory, presented as current.

**A schema with no declared type is the usual cause.** A parameter typed
`string` gets quoted, and one whose schema says nothing gets a bare word.
Set the limit to `0` to disable the breaker without disabling the
counting.

**This breaker is `runtime-claude`'s own mitigation**, not a manager
obligation or a requirement on every runtime — see
the [runtime specification](contracts/runtime.md) for what the contract
itself requires.

## What it does NOT ship

- **The floor ServiceAccount.** The parent always renders `agentops-runtime`,
  bound to nothing.
- **The runtime defaults.** Defaults differing per bundle would be one fact in
  as many places as there are vendors.
- **The context volume.** Persistence is WIRING and lives on the `Pipeline`.
- **Any RBAC.** The account this runtime names is a REFERENCE — see
  [installation](installation.md).
