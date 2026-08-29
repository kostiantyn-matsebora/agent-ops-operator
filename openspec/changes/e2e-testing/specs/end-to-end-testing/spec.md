## ADDED Requirements

### Requirement: The end-to-end pack runs against a real single-node cluster
An end-to-end pack SHALL exist that provisions a real single-node Kubernetes
cluster (k3s), installs the chart built from the working tree, waits for the
manager and every enabled adapter to become Ready, and asserts against the live
cluster. It SHALL live in the manager's Go module (`platform/manager/`) so it inherits
the existing Kubernetes client dependency, and SHALL NOT introduce a new Go
module.

The pack's subject is the **substrate** — the layer the existing envtest suite
structurally cannot reach, because envtest runs no kubelet, no scheduler and no
CSI. A test whose assertion holds under envtest SHALL NOT be duplicated here;
the pack exists for what envtest cannot decide, and every test in it SHALL be
justifiable by naming the component (kubelet, scheduler, informer, CNI, image
puller) whose participation makes the assertion possible.

#### Scenario: The chart under test is the working tree, not a published release
- **WHEN** the pack runs
- **THEN** it installs the chart from `chart/` in the working tree with images built from the same commit, so a template change and a code change are verified together

#### Scenario: A test that envtest could make is rejected from the pack
- **WHEN** a candidate test asserts only on CR fields written by a reconciler, with no kubelet, scheduler, informer or image-pull participation
- **THEN** it belongs in `internal/integration/`, not in the end-to-end pack

### Requirement: The pack asserts the substrate facts envtest cannot
The pack SHALL cover at minimum the following, each of which is unasserted
today because it is decided by a component envtest does not run:

1. **Credential projection** — a `Channel.credentialsSecretRef` reaches the
   adapter pod as environment under prefix `AGENTOPS_CRED_<CHANNEL>_`, resolved
   by the kubelet, while the manager performs zero Secret reads.
2. **RBAC as enforced** — the manager's ServiceAccount is denied `secrets`
   verbs, and an adapter ServiceAccount is denied everything the chart did not
   grant it. Assertions SHALL use `SubjectAccessReview` against the live
   authorizer, never the rendered Role, because the rendered Role is what
   envtest already checks and is not what enforces anything.
3. **Informer liveness** — every reconciled kind is genuinely watchable by the
   manager's ServiceAccount, so an `resources:` entry miscased to a Go type name
   fails the pack instead of producing a silent forbidden loop.
4. **Context continuity across a pod restart** — with
   `contextStorage: volume` and a context volume, a conversation's
   `runtimeContextId` survives deletion of its runtime pod and is handed back on
   the next work unit.
5. **Image pull** — every image the chart references pulls in-cluster,
   including through `imagePullSecrets` when the registry is private.
6. **Admission FIFO under real pod lifecycle** — with the cap saturated by
   pod-backed conversations, deleting a runtime pod promotes the oldest
   `Pending` conversation, driven by a real DELETE event.

#### Scenario: A miscased RBAC resource fails the pack
- **WHEN** a `resources:` entry is written as a Go type name (for example `AgentRuntimes`) instead of the lowercase plural
- **THEN** the informer-liveness check fails, rather than the manager starting healthy and silently reconciling nothing

#### Scenario: The no-secret-reads invariant is verified, not assumed
- **WHEN** the pack completes a conversation that used a channel credential
- **THEN** the adapter received the credential as environment AND a `SubjectAccessReview` confirms the manager's ServiceAccount cannot read Secrets at all

#### Scenario: Context survives losing the pod
- **WHEN** a conversation has completed a run under `contextStorage: volume`, and its runtime pod is deleted
- **THEN** the next input is dispatched with the same `runtimeContextId` and the run does not fail for lost context

### Requirement: The signal loop breaker is exercised with a genuinely broken runtime
The pack SHALL reproduce the conditions of the signal-loop failure mode —
`signal-k8s-events` observing a cluster in which a runtime pod cannot start —
and SHALL assert that the number of Conversations stays bounded. This test
exists because the loop's three guarding mechanisms cannot be exercised
anywhere else: reproducing it requires a real pod that really fails and a real
Event stream, and its failure mode is unbounded etcd growth that no downstream
cap arrests.

#### Scenario: A runtime that cannot start does not breed conversations
- **WHEN** a Pipeline is pointed at a runtime image that cannot start, a signal opens a Conversation, and its runtime pod fails repeatedly for several minutes while `signal-k8s-events` watches the namespace
- **THEN** the Conversation count remains bounded and no Conversation is created from an event about agent-ops' own machinery

#### Scenario: Exclusion holds before the object cache is warm
- **WHEN** `signal-k8s-events` is restarted while failing agent-ops pods are already emitting Warning events
- **THEN** no signal is emitted for them during startup, exercising the name-prefix mechanism against a cold cache

### Requirement: Inbound adapters are driven by fixtures, never by their upstreams
No third-party system SHALL be deployed by the pack. Adapters that host a port
and wait — the Alertmanager webhook receiver, the Telegram ingest adapter, the
channel adapter's update intake — SHALL be driven by POSTing captured payloads
from a fixture set. `signal-cron` SHALL be driven by its own schedule, and
`signal-k8s-events` by workloads the pack creates and genuinely breaks.

Fixtures SHALL be scrubbed of real chat identifiers, usernames, hostnames and
tokens, and SHALL be shared with the owning module's unit tests so a single
captured payload cannot drift between the two suites.

#### Scenario: The alerting lane needs no alerting system
- **WHEN** the pack exercises the VictoriaMetrics/Alertmanager lane
- **THEN** it POSTs an Alertmanager-format fixture to the adapter's `/webhook/{source}` and deploys neither VictoriaMetrics nor Alertmanager

#### Scenario: A fixture carries no real identifiers
- **WHEN** a captured Telegram update is added to the fixture set
- **THEN** chat ids, user ids and usernames are replaced with synthetic values before it is committed

### Requirement: The console is the end-to-end channel
Conversation-lifecycle coverage SHALL be driven through the console, because it
is a conforming `ChannelAdapter` with no third-party dependency, which makes the
whole path — origination, dispatch, delivery, close — production code with
nothing simulated. The pack SHALL drive it through its own HTTP API
(`POST /api/conversations`, `POST /api/conversations/{name}/messages`), the path
a person takes minus the browser, so the console's authentication gate and its
single write path are exercised too.

#### Scenario: A conversation completes with no test double in the path
- **WHEN** the pack starts a conversation through the console API and sends a follow-up message
- **THEN** the answer is delivered back to the console's bound thread, and no component in the path was replaced by a fake

#### Scenario: The console write path is authenticated
- **WHEN** the pack posts to a console write endpoint without a valid session
- **THEN** the request is refused, and the refusal is asserted rather than worked around by bypassing the console

### Requirement: The real agent runtime is the primary oracle
The default runtime under test SHALL be `runtime-claude` with a real API token,
because the questions worth asking end-to-end are ones only an agent can answer:
whether context survived a gap, whether the answer is correct, and whether the
bound toolset actually constrains it.

Assertions on agent output SHALL be **tolerant** — the presence of a required
fact, identifier or refusal — and SHALL NOT assert exact prose. Each SHALL carry
a bounded retry. A suite that goes red on phrasing trains its readers to ignore
red, which costs more than the coverage is worth.

#### Scenario: Continuity is proven by asking the agent
- **WHEN** the pack states a distinctive fact in one turn and asks for it back in a later turn separated by a runtime pod restart
- **THEN** the fact appears in the answer, asserted by presence rather than by exact wording

#### Scenario: Phrasing variance does not fail the pack
- **WHEN** the agent returns a correct answer worded differently from a previous run
- **THEN** the assertion passes

### Requirement: Toolset binding is proven end to end
The pack SHALL assert that a Pipeline's bound toolsets are the operative
allowlist, by asking the agent for an action outside what the Pipeline bound and
asserting the action does not occur. This is the only end-to-end evidence that
capabilities are wiring rather than profile fields, and it SHALL be asserted on
the agent's effect, not on the rendered `--allowedTools` value, which is already
covered by unit tests.

#### Scenario: An unbound capability is unavailable
- **WHEN** a Pipeline binds only a read-scoped toolset and the agent is asked to perform a mutating action
- **THEN** the mutation does not occur in the cluster, and the run completes rather than hanging on a permission prompt

#### Scenario: A bound capability is available
- **WHEN** the same Pipeline is asked for a read within its bound toolset
- **THEN** the read succeeds, so the previous scenario is evidence of a boundary rather than of a broken agent

### Requirement: The Telegram lane runs against a fake Bot API
A fake Bot API server SHALL implement `getUpdates`, `sendMessage`,
`sendDocument`, `createForumTopic` and `closeForumTopic`, and the Telegram lane
SHALL be pointed at it through the adapters' configured base URL. No test SHALL
contact `api.telegram.org`, and no bot token SHALL be required to run any tier.

The double is faithful because `gateway-telegram` forwards updates verbatim: a
replayed `Update` is byte-identical to what the real API would have produced.

#### Scenario: The full Telegram path runs with no bot
- **WHEN** the fake serves a captured `Update` on `getUpdates`
- **THEN** the router classifies and forwards it, the ingest or channel adapter handles it, and the resulting outbound send is observed as a recorded call on the fake

#### Scenario: The single-consumer rule is not violated by testing
- **WHEN** the Telegram lane runs
- **THEN** exactly one `getUpdates` consumer exists against the fake, matching the production invariant

### Requirement: Tiers are defined by what gates a pull request
The pack SHALL be split into tiers with an explicit gating rule:

- **Pull requests** SHALL be gated by contract conformance and a thin
  cluster smoke running on the stub runtime — deterministic, no API token, and
  bounded in wall-clock time — reported through `ci-green`'s `needs:`, never as
  a separately required check.
- **A release** SHALL run the same pull-request tier again on the tagged
  commit and SHALL publish nothing until it passes: a tag can land on a
  commit CI proved days earlier against a different cluster.
- **The pull-request tier** SHALL also be runnable on demand against any
  branch, so it can be met before a pull request exists.
- **The full pack, including the real-runtime lane,** SHALL run nightly when
  the default branch moved since its last successful run — a night with no
  change is a night with nothing to learn and tokens to spend — and on manual
  dispatch, and SHALL NOT gate pull requests.
- **Pull requests from forks** SHALL run every tier their secret access allows
  and SHALL NOT be failed for tiers they cannot run. Secrets being unavailable
  to forks is the intended access boundary and SHALL NOT be worked around.

#### Scenario: A fork pull request is not failed for missing secrets
- **WHEN** a pull request originates from a fork and the API token is unavailable
- **THEN** the real-runtime tier is skipped and reported as skipped, and the pull request is not marked failed on that account

#### Scenario: A release is gated by the smoke on the tagged commit
- **WHEN** a `<component>-v<semver>` or `chart-v<semver>` tag is pushed
- **THEN** the pull-request tier runs on that commit, and the image or chart is published only if it passed

#### Scenario: An unchanged night runs nothing
- **WHEN** the nightly full run finds the default branch at the commit its last successful run tested
- **THEN** it skips, and a dispatched run in the same state still runs

#### Scenario: A token-consuming tier never gates a pull request
- **WHEN** the real-runtime tier fails
- **THEN** no pull request is blocked by that failure alone

### Requirement: The pack's scope boundary with continuous integration is stated
The end-to-end pack SHALL own the definition of the cluster-based jobs only.
Per-module build, vet and unit test, the envtest suite, and chart lint/render
remain owned by the `continuous-integration` capability. Neither capability
SHALL restate the other's jobs, so that "what CI runs" has exactly one
definition per tier.

#### Scenario: A per-module job is not duplicated here
- **WHEN** a change adds a per-module build or lint step
- **THEN** it is specified under `continuous-integration`, not under end-to-end testing
