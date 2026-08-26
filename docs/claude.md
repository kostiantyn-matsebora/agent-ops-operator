# Claude runtime (subchart)

The reference agent runtime, shipped as a bundle: `claude-code` implementing the
operator's work contract.

`chart/charts/claude/` renders exactly two objects — one `AgentRuntime` named
`claude` and, when you supply a token, its credential `Secret`. The PARENT
renders the `AgentRuntime` named `default` as a copy of it, unless another
runtime is flagged `default: true`.

**ON by default**, unlike every other bundle here. It is what a fresh install
executes on. The other runtime bundle, `chart/charts/ollama/`, is the local-model
one — see [runtimes/ollama](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/ollama/).

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

## What it does NOT ship

- **The floor ServiceAccount.** The parent always renders `agentops-runtime`,
  bound to nothing.
- **The runtime defaults.** Defaults differing per bundle would be one fact in
  as many places as there are vendors.
- **The context volume.** Persistence is WIRING and lives on the `Pipeline`.
- **Any RBAC.** The account this runtime names is a REFERENCE — see
  [installation](installation.md).
