## Why

Thirteen containers, and the tree says nothing about any of them.

`signal-cron/`, `console/`, `telegram-router/`, `api/`, `chart/` and `docs/` sat
in one flat listing at the root, so a reader could not tell a signal adapter from
a platform component from the operator's own package directories without opening
them. A new module had no rule that placed it.

The one mapping that IS real — one container per directory — was invisible, and
load-bearing. `.github/components.sh` derives the release matrix from the
filesystem, so **a directory name is a published identity**: nothing said so, and
a tidy-up that renamed one would have renamed an image every install pins.

## What Changes

The directory tree becomes a projection of the C&C view: **one container, one
directory, grouped by component type.**

- **Five groups**, each named for what its members ARE at runtime:

  | Group | Members | The type |
  |---|---|---|
  | `platform/` | `manager` `console` `housekeeping` `context-sync` `egress-proxy` | the product's own components |
  | `runtimes/` | `claude` | client side of the work contract |
  | `signals/` | `cron` `alertmanager` `k8s-events` `ha` `telegram` | push to `/signal/inbound` |
  | `channels/` | `telegram` | serve `/channel/*` |
  | `gateways/` | `telegram` | speaks no agent-ops connector at all |

- **The operator moves to `platform/manager/`**, taking `api/`, `cmd/`,
  `config/` and `internal/` with it. Everything a container builds from lives
  inside its own directory, because the build context is derived from the
  Dockerfile's directory and `COPY ../api` is not legal.

- **The component name is derived from the PATH, not the directory name.** A
  PLURAL group names a kind of component and lends its singular as a prefix; a
  singular group is a namespace and lends nothing:

  ```
  signals/cron       -> signal-cron       -> agentops-signal-cron
  channels/telegram  -> channel-telegram  -> agentops-channel-telegram
  runtimes/claude    -> runtime-claude    -> agentops-runtime-claude
  platform/console   -> console           -> agentops-console
  ```

  So the kind is said once by the directory and once by the image, and never
  twice in either. Twelve of thirteen published names come out unchanged.

- **`components.sh` loses its one hardcoded name** — the `if [ "$dir" = "." ]`
  branch that existed because the root's basename is the checkout directory
  rather than an identity — and gains a uniqueness assertion, because a derived
  name is no longer unique by construction the way a flat one was.

- **ONE component is renamed, deliberately**: `telegram-router` becomes
  `gateway-telegram` — image `agentops-gateway-telegram`, workload
  `agentops-gateway-telegram`, values pin in `telegram-bundle`. **BREAKING** for
  an install that upgrades in place: Helm creates the new Deployment and deletes
  the old one, and for those seconds two consumers poll one bot token. The old
  image is left published, as `signal-vmalertmanager` was.

- **Every module path follows its directory.** `go.mod` declares where the module
  IS, so leaving the manager at `github.com/…/agent-ops-operator` while its
  `go.mod` sits in `platform/manager/` would make it unfetchable — the resolver
  looks for `go.mod` at the path the module claims.

- **`ci.yml` names `platform/manager` explicitly.** It splits "the root module"
  (which needs envtest) from "every other `go.mod`" — after the move there is no
  root module, so the envtest job names its module. One hardcoded string added
  where one was removed.

- **`dependabot.yml` paths change**, being the one hand-written module list in
  `.github/` by its own admission.

- **`.claude/rules/` scoping simplifies.** `signal-rules.md` names two adapter
  directories in its `paths:` frontmatter today; `structure.md`'s repository map
  is rewritten around the buckets.

**Nothing else is renamed or reconfigured.** No CRD field, no HTTP contract, no
runtime behaviour, and no other image name. Twelve of thirteen components keep
the identity they have published all along.

## Capabilities

### New Capabilities

- `repository-layout`: where a component's source lives and why — one container
  per directory, the leaf name as published identity, the bucket as component
  type, and the derivations that depend on both.

### Modified Capabilities

None. Seven specs name a module path in passing, and `telegram-ingest-router`
names the renamed component, but a path and a name are implementation anchors
rather than requirements — no behaviour those specs
describe changes. A task corrects the anchors directly.

## Impact

| Area | Change |
|---|---|
| every module directory | moved, contents untouched |
| `api/` `cmd/` `config/` `internal/` `go.mod` `go.sum` `Dockerfile` `.dockerignore` | moved into `platform/manager/` |
| `.github/components.sh` | root special case deleted, name derived from the path, uniqueness asserted |
| every `go.mod` | module path follows its directory |
| `chart/charts/telegram-bundle/` | the router's image, workload name and labels |
| `.github/workflows/ci.yml` | envtest job names `platform/manager` |
| `.github/dependabot.yml` | eleven paths rewritten, plus the npm one |
| `.claude/rules/` | `structure.md` rewritten, `paths:` frontmatter updated in `signal-rules.md` and `palette-and-mark.md` |
| `docs/contracts.md` | two relative links |
| `openspec/specs/` | seven stale path anchors |
| Dockerfile comment lines | the `docker buildx build ./<dir>/` line each one carries |

Not in scope: splitting or merging any module, renaming the `router:` values key
in `telegram-bundle` (bundle-local, and renaming it would break adopters' values
for nothing), or making `api/` importable by other modules — nothing imports it
today, and every submodule is dependency-free with zero requires.
