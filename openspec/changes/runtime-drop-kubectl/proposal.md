# runtime-drop-kubectl

## Why

`AgentRuntime` is supposed to differentiate only by vendor backend and trust level — the `mcp-toolset-crd` design states it directly: "runtimes become generic … one runtime per vendor × trust level". `runtime-claude` violates that by baking in `kubectl` (pinned v1.34.3), the single domain-specific dependency in an image whose other contents are either genuine runtime responsibilities (git, openssh-client, for the repo checkout the runtime owns) or generic shell utilities. Domain tooling belongs in wiring — `MCPConfig` and `MCPToolset` bound by a Pipeline — which is exactly where every other capability now lives. The pin also carries version-skew risk against the cluster and forces an image rebuild to track kubectl releases.

## What Changes

- **BREAKING (runtime image)**: `runtime-claude/Dockerfile` drops the kubectl download layer. Agents reach Kubernetes through the MCP tools `k8s-mcp-tooling` provides.
- `internal/dispatch/templates/task.md` stops telling every profile to observe "e.g. `kubectl` under this pod's ServiceAccount". This is the subtler half: that text is a manager-side lane template shipped to ALL users, so leaving it would advertise a tool the image no longer has. The wording becomes tool-agnostic — the agent uses whatever its allowlist grants.
- The `k8s-bundle` profile's tool grant moves fully to MCP: `Bash` no longer implies cluster access, which turns the bundle's `rbac.mode: full` warning from "the only wall is RBAC" into a genuine two-wall posture.
- A documented **escape hatch**: operators who need kubectl keep it by building a derived image or running a runtime whose image includes it — `AgentRuntime.spec.image` is values/CR-configurable precisely so the runtime is swappable. The README documents this as the supported path rather than pretending nobody needs it.
- Image tag bump and a migration note; this is the change that makes the runtime genuinely generic.

## Capabilities

### Modified Capabilities

- `k8s-bundle`: the bundle's cluster access requires its MCP component rather than kubectl-via-`Bash`; the RBAC-mode documentation changes accordingly, since the runtime SA's grants are no longer directly exercisable from a shell.

## Impact

- **Runtime image**: `runtime-claude/Dockerfile` (one layer removed), image tag bump. Anything relying on kubectl inside the runtime breaks — the reason this change is sequenced last.
- **Manager**: `internal/dispatch/templates/task.md` wording; pinned by `internal/dispatch/dispatch_test.go` fixtures, which change deliberately.
- **Chart**: `k8s-bundle` values/docs — the MCP component becomes effectively required for cluster access; the `rbac.mode: full` warning is rewritten.
- **Docs**: README (runtime genericity, the kubectl escape hatch, the demo flow's new requirements), CLAUDE.md (runtime-claude map entry).
- **Depends on**: `k8s-mcp-tooling` landing first and proving adequate in practice — this change is the point of no return, and should not be started on the strength of the design alone.
- **Composes with**: `builtin-toolsets`, which makes `Bash` separately bindable; together they allow a profile with cluster access and no shell at all.
