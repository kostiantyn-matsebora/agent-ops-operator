## Why

**A Pipeline grants tools. A profile writes prompts. Yet the profile chooses the
ServiceAccount the agent executes as** — which is its actual power in the
cluster.

`AgentProfile.spec.runtimeRef` selects an `AgentRuntime`, and an `AgentRuntime`
carries `serviceAccountName`. So profile-edit rights are SA-choice rights.

**The stated reason does not survive the symmetry test.** `wiring.md` says:

> The SA stays runtime-level on purpose: a Pipeline choosing an SA would make
> pipeline-edit rights a privilege escalation.

That does not REMOVE the escalation. It moves it to the object you would trust
LESS. A profile is prompts, a repo ref and limits. A Pipeline already grants
capabilities through `toolsets` and `mcpConfigs`. **Whoever is trusted to grant
tools is more qualified to choose an execution identity, not less** — and today
neither object's editor can be reasoned about, because the two halves of what an
agent can do are split across both.

**Capabilities and execution identity are the same decision.** One says which
tools may be called, the other says with whose credentials. Splitting them means
no single object states an agent's power, and no single reviewer can approve it.

**Pipeline is the only place wiring happens.** That is already the model for
sources, channels, the profile and capabilities. The runtime and its identity
are the last two facts violating it.

## What Changes

- **`Pipeline.spec.runtimeRef`** selects the `AgentRuntime`. Absent, the
  runtime named `default` — the one the parent chart renders — as today.
- **`Pipeline.spec.serviceAccountName`** names the identity that runtime
  executes under, OVERRIDING the runtime's own. Absent, the runtime's
  `serviceAccountName`, which the chart still defaults to `agentops-runtime`.
  **Nothing changes for an install that sets neither.**
- **`AgentProfile.spec.runtimeRef` is RETIRED**, dual-read for one release so a
  profile applied before the upgrade keeps dispatching to the runtime it named.
  Same posture the retired `sessionId` got.
- **The Conversation SNAPSHOTS both** at creation, beside `toolsets` and
  `mcpConfigs`. A Pipeline edit must not re-wire a running conversation onto a
  different identity — the sharpest case of the rule that already governs
  bindings.
- **More than one runtime ServiceAccount may exist per release.** The chart's
  is the DEFAULT, not the only one, and `runtime-rbac.yaml` may grant several.
- **`runtimes are generic` stops being a consequence and becomes a choice.** It
  followed from the SA being runtime-level. With the SA on the Pipeline, one
  runtime image can serve several trust levels.
- **NO RUNTIME IDENTITY IS `cluster-admin`, AND NONE MAY READ SECRETS.**
  `rbacMode: full` currently binds the runtime ServiceAccount to
  `cluster-admin`, so an agent with a shell can read every credential in the
  cluster. It is replaced by an ENUMERATED ClusterRole the chart owns, carrying
  no verb on `secrets` and no wildcard that reaches them.

**BREAKING, TWICE.**

- A profile that names a `runtimeRef` keeps working for one release, then stops.
  An install relying on it must move the ref to every Pipeline that routes to
  that profile.
- **An install running `rbacMode: full` LOSES `cluster-admin`.** An agent that
  depended on a grant the enumerated role omits stops being able to use it, and
  the install must render its own ClusterRole and name its account on the
  Pipeline. That is the point: the grant becomes visible and reviewable instead
  of being a mode name.

## Capabilities

### Modified Capabilities
- `pipeline-model`: the Pipeline gains `runtimeRef` and `serviceAccountName`,
  and its "carries no runtime selection" requirement is inverted.
- `profile-is-identity`: `runtimeRef` leaves the profile. Identity stops
  including execution preference.
- `agent-runtime-ownership`: the parent's runtime SA becomes the DEFAULT rather
  than the only one, "exactly one runtime ServiceAccount per release" no longer
  holds, and a new requirement forbids `cluster-admin` and any `secrets` verb on
  every runtime identity the chart renders.
- `mcp-toolset-model`: the Conversation's materialization rule extends to the
  runtime and the SA — the same snapshot, for the same reason.

## Impact

- `api/v1alpha1` — `PipelineSpec.RuntimeRef` + `.ServiceAccountName`,
  `ConversationSpec` gains the materialized pair, `AgentProfileSpec.RuntimeRef`
  marked deprecated. Deepcopy + CRD regeneration.
- `internal/runtimepod/podspec.go` — `ResolveFor` takes the conversation's
  materialized runtime rather than reading `profile.Spec.RuntimeRef`, and the
  pod's `ServiceAccountName` prefers the conversation's over the runtime's.
- `internal/controller/conversation_controller.go` — the resolution comment at
  the admission path, and whatever reads the profile for it.
- `internal/httpapi/signals.go` and the chat origination path — snapshot both
  fields at creation, every origination path alike.
- `chart/` — `runtime-rbac.yaml` DROPS the `cluster-admin` binding and gains an
  enumerated acting ClusterRole; may bind more than one SA;
  `global.agentops.runtime.serviceAccountName` becomes a default;
  `pipelines:` values in the parent and in `k8s-bundle`, `prometheus-bundle`,
  `ha-bundle` gain the two optional fields.
- Docs: `docs/concepts.md` (both fields and the snapshot),
  `docs/installation.md` (the SA is no longer singular), `CHANGELOG.md`.
- Rules: `wiring.md`'s "the SA stays runtime-level" and "runtimes are generic",
  `invariants.md`'s substrate section ("no subchart renders a runtime SA"), and
  `terminology.md`'s `AgentProfile` row, which lists what identity holds.

## Open questions

- **Does the profile keep a VENDOR hint?** An `AgentRuntime` is a vendor
  implementation, and a profile written against claude-code's `.claude/agents/`
  layout cannot run on an arbitrary runtime. Moving the ref to the Pipeline puts
  vendor choice in wiring, where the person wiring may not know the profile's
  requirement. Options: a `spec.runtimeClass`-style hint on the profile that
  Ready validates against, or accept that a mis-wired Pipeline fails visibly on
  its first run. **Resolved in design, not here.**
- **Does the SA belong on the Pipeline or on the runtime it names?** Naming both
  a runtime and an SA on the Pipeline is two fields where one composite ref
  might do. Weighed in design.
