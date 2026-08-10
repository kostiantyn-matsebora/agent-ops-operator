# runtime-drop-kubectl — design

## Context

`runtime-claude/Dockerfile` installs, in order: `git openssh-client curl jq ca-certificates procps`, the `@anthropic-ai/claude-code` npm package, and then a pinned `kubectl` v1.34.3 binary with the comment "kubectl for the bounded cluster-apply lane (RBAC of the worker SA is the wall)". The runtime pod automounts its ServiceAccount token, so `Bash` plus kubectl equals full exercise of whatever the runtime SA may do.

Every other item in that image is defensible at the runtime layer: git and openssh-client serve the repo checkout at `/data/workspace`, which is a runtime responsibility; curl/jq/procps are generic shell utilities. kubectl is the only entry that encodes a *domain*.

That matters because of a principle the project already adopted (`mcp-toolset-crd`, archived 2026-08-08): with tooling moved to wiring and persona moved to profiles, `AgentRuntime` should differentiate only by vendor backend and by trust level. A Kubernetes CLI in the vendor layer is the same category error as an MCP server would be.

A second coupling is easy to miss: `internal/dispatch/templates/task.md` — a MANAGER-side lane template rendered for every profile without a custom prompt — instructs the agent to answer "using what you can observe (e.g. `kubectl` under this pod's ServiceAccount)". The assumption is baked into the operator, not just the image.

## Goals / Non-Goals

**Goals:**

- Remove the last domain-specific dependency from the runtime image, so the vendor-per-image model actually holds.
- Remove the kubectl version pin and its skew risk against arbitrary clusters.
- Leave a supported, documented path for operators who genuinely need kubectl.
- Turn the k8s bundle's `rbac.mode: full` posture from one wall into two.

**Non-Goals:**

- Not touching git/openssh-client/curl/jq/procps — those are runtime responsibilities or generic utilities, not domain tooling.
- Not removing `Bash` from any profile. `Bash` remains useful for the workspace; it simply stops implying cluster access. (Making `Bash` separately bindable is `builtin-toolsets`.)
- Not building or maintaining a kubectl-bearing image variant in this repo — the escape hatch is a documented derived build, not a second artifact to keep in sync.
- Not changing the `/work` contract, `AgentRuntime` schema, or how runtimes are selected.

## Decisions

### D1: Sequenced last, and gated on evidence

This is the only irreversible step in the arc, so it should not start on the strength of `k8s-mcp-tooling`'s design alone. The entry condition is that the MCP path has actually been run: an operator (or the maintainer's own cluster) has answered real cluster questions through `mcp__kubernetes__*` tools without falling back to kubectl. Recorded as a gate, because the failure mode — shipping an image that cannot inspect a cluster and discovering the MCP tools are inadequate afterwards — is a bad one to hit in someone else's release.

### D1a: Gate result — GO (2026-08-10, reference cluster)

The MCP path was exercised directly against the live `agent-ops` install, calling
`kubernetes-mcp-server` over its HTTP transport rather than through an agent, so
the evidence is about the TOOLS rather than about one model's choices.

The server registers **20 tools**. Output is kubectl-shaped text, not raw JSON —
`pods_list` returns the `get pods -o wide --show-labels` table, `nodes_top`
returns the `top nodes` table — so nothing is lost in readability.

Against the three questions the demo advisor is documented to answer:

| Question | Tools | Result |
|---|---|---|
| Crashlooping pods | `pods_list` (restarts, status), `pods_get`, `pods_log`, `events_list` | covered |
| Node pressure | `nodes_top`, `nodes_stats_summary`, `resources_get` (conditions) | covered |
| Failed workloads | `resources_list` (READY/UP-TO-DATE/AVAILABLE), `events_list`, `pods_get` | covered |

Verified live: `pods_list`, `nodes_top`, `events_list`, `resources_list` for
Deployments, and `pods_log` with `tail`. All returned correct data for this
cluster.

**What is genuinely lost, and accepted:**

- **No text processing.** There is no pipe, `grep`, or `jq`. `pods_list` returns
  every pod and the agent filters in context, where `kubectl get pods
  --field-selector=status.phase=Failed` filtered server-side. On a large cluster
  that is a token cost, not a capability loss.
- **No `auth can-i`, no `port-forward`, no first-class `rollout restart`.** The
  last is reachable by patching through `resources_create_or_update`; the first
  two are not reachable at all. This is the escape valve D3 exists for.

**Two dead entries found in the shipped toolset.** `k8s-observability` names
`configuration_contexts_list` and `targets_list`, neither of which this server
registers. Allowlist entries for non-existent tools are inert, so nothing is
broken, but they are noise that implies capability the install does not have.
Fixed as part of this change.

Verdict: GO. The documented demo questions are all answerable without kubectl,
and the gaps are the ones the proposal already accepted rather than new
surprises.

### D2: The lane template becomes tool-agnostic

`task.md` currently names kubectl. The replacement says the agent should use whatever its allowlist grants and observe before acting, without naming an instrument. Rationale: that template is shipped to every user of the operator, including those whose agents have nothing to do with Kubernetes, so naming kubectl was always over-specific — it just became wrong rather than merely odd.

`internal/dispatch/dispatch_test.go` pins template content; those fixtures change deliberately in the same commit, per the repo's rule that dispatch semantics change by changing tests on purpose.

### D3: The escape hatch is a derived image, documented

`AgentRuntime.spec.image` exists exactly so the runtime is swappable. Operators who need kubectl write:

```dockerfile
FROM kmatsebora/agentops-runtime-claude:<tag>
USER root
RUN curl -fsSL -o /usr/local/bin/kubectl https://dl.k8s.io/release/<ver>/bin/linux/amd64/kubectl \
 && chmod 0755 /usr/local/bin/kubectl
USER node
```

and point an `AgentRuntime` at it. Three lines, and the version pin becomes theirs to own against their cluster — which is the correct place for it. Shipping a second official variant was rejected: two images to build, tag, and keep in sync forever, to serve a case that a three-line Dockerfile already serves.

### D4: The bundle's RBAC story is rewritten, not just re-pointed

Today `rbac.mode: full` means an LLM-driven agent with cluster-admin and a shell that can use it. After this change the runtime SA's grants are no longer directly exercisable — reach flows through the MCP server's own identity and the tools the allowlist names. The README's warning changes from "this is the only wall" to an accurate description of two walls, and `rbac.mode` documentation notes that it now governs what MCP-independent paths (none, by default) can do.

This is the concrete safety payoff of the arc, and it should be stated plainly rather than left implicit in a changelog.

## Risks / Trade-offs

- [Someone's working setup breaks on upgrade] → The entry gate (D1), a major image tag bump, a README migration note, and the three-line derived image in D3. This is a real break and the proposal marks it BREAKING rather than softening it.
- [MCP tools turn out less capable than kubectl for open-ended debugging] → Exactly what D1's gate exists to discover, and the reason this change is separate from `k8s-mcp-tooling` rather than folded into it. If the gate fails, this change is abandoned or narrowed, and the arc still delivered value.
- [Agents lose an escape valve for cluster operations the MCP server does not expose] → True and accepted; that is the boundary being asked for. Operators who want the valve keep it via D3.
- [The lane-template edit affects users with no Kubernetes involvement] → It affects them positively: the template stops naming a tool their agent never had.
- [Image still carries curl, so an agent could fetch kubectl itself] → Out of scope and not worth chasing here; a sandbox that leaks by `curl | sh` is a different problem than a dependency the image ships and blesses. Worth noting so it is not mistaken for an oversight.

## Migration Plan

1. Confirm the D1 gate: the MCP path has answered real questions in a live cluster.
2. Drop the Dockerfile layer, reword `task.md`, update its test fixtures, bump the image tag.
3. Update `k8s-bundle` docs (RBAC modes, the MCP component's new necessity) and the README runtime section with the derived-image recipe.
4. Release with the BREAKING note; operators pin the previous image tag until they migrate, since `AgentRuntime.spec.image` makes that a one-line hold.

Rollback = point `AgentRuntime.spec.image` back at the previous tag. Nothing in the CRDs or the manager depends on the change.

## Resolved Questions

- **Hard-fail vs warn when the MCP component is off** — RESOLVED: warn, in the
  parent `NOTES.txt`. Implementation notes worth keeping:

  The question as posed was about `rbac.mode: full` with MCP disabled, but
  dropping kubectl makes the plainer case more serious: with `mcp.enabled=false`
  the k8s-engineer agent has NO cluster access at all, whatever the RBAC mode.
  The warning fires on that condition and mentions the unusable `full` grant only
  as an additional line, which is the right order of severity.

  Not a render failure, because `mcp.enabled=false` is legitimately reachable:
  an operator running their own MCP server sets `mcp.url` and disables only
  `mcpServers`, and someone mid-migration may disable the component deliberately.
  Failing the render would block those. This is the same test applied to the
  console's plaintext Ingress — fail only what CANNOT work (a sub-path on a
  root-only SPA), warn where the chart cannot see the whole picture.

  It lives in the PARENT NOTES.txt because Helm 3 does not render a subchart's
  NOTES.txt at all — a warning placed in `chart/charts/k8s-bundle/templates/`
  would never be seen.
