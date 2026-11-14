# end-to-end-testing Specification

## Purpose
TBD - created by archiving change e2e-testing. Update Purpose after archive.

## Requirements

### Requirement: The system pack runs against a real single-node cluster
A system pack SHALL exist that provisions a real single-node Kubernetes
cluster (k3s), installs the chart built from the working tree, and waits for
the manager and every enabled adapter to become Ready.

It SHALL assert against the live cluster. It SHALL live in the manager's Go
module (`platform/manager/`) so it inherits the existing Kubernetes client
dependency, and SHALL NOT introduce a new Go module.

The pack's subject is the **substrate** — the layer the existing envtest
suite structurally cannot reach, because envtest runs no kubelet, no
scheduler and no CSI.

A test whose assertion holds under envtest SHALL NOT be duplicated here. The
pack exists for what envtest cannot decide, and every test in it SHALL be
justifiable by naming the component (kubelet, scheduler, informer, CNI, image
puller) whose participation makes the assertion possible.

#### Scenario: The chart under test is the working tree, not a published release
- **WHEN** the pack runs
- **THEN** it installs the chart from `chart/` in the working tree with images built from the same commit, so a template change and a code change are verified together

#### Scenario: A test that envtest could make is rejected from the pack
- **WHEN** a candidate test asserts only on CR fields written by a reconciler, with no kubelet, scheduler, informer or image-pull participation
- **THEN** it belongs in `internal/integration/`, not in the system pack

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

- **Pull requests** SHALL be gated by contract conformance alone — a minute,
  no cluster, no secret — reported through `ci-green`'s `needs:`, never as a
  separately required check. No pull request provisions a cluster.
- **The cluster smoke** — the smoke tier: a thin k3s run on the stub runtime,
  deterministic, no secret, bounded in wall-clock time; conformance is not
  restated in it, since the release already requires the commit's CI — SHALL
  gate a RELEASE and SHALL be runnable on demand against any branch.
- **A release** SHALL publish nothing until the cluster smoke passed on the
  tagged COMMIT: a tag can land on a commit CI proved days earlier against a
  different cluster. The smoke is keyed to the commit, never to the tag: a
  release SHALL look up the commit's existing smoke result before provisioning
  a cluster, SHALL reuse a passed one from any earlier run on that commit,
  SHALL wait — bounded — for one still running on that commit rather than
  race it, and SHALL run its own only where none passed. A commit tagged
  fourteen times is smoked once.
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

#### Scenario: A pull request runs no cluster
- **WHEN** a pull request changes a component or the chart
- **THEN** contract conformance runs and gates it through `ci-green`, and no cluster is provisioned

#### Scenario: A release is gated by the smoke on the tagged commit
- **WHEN** a `<component>-v<semver>` or `chart-v<semver>` tag is pushed and no run has smoked that commit
- **THEN** the cluster smoke runs on that commit, and the image or chart is published only if it passed

#### Scenario: A commit already smoked is not smoked again
- **WHEN** a tag is pushed on a commit whose smoke passed in an earlier release run or an on-demand run
- **THEN** no cluster is provisioned, the lookup finds the commit already smoked, and the image or chart publishes

#### Scenario: A smoke in flight on the commit is waited for
- **WHEN** a tag is pushed while another run's smoke on the same commit is still running
- **THEN** the release waits for that verdict, publishes on a pass, and runs its own smoke on a failure

#### Scenario: A re-run after a flake smokes again
- **WHEN** a release run whose smoke failed is re-run
- **THEN** the lookup finds no passed smoke and the cluster smoke runs

#### Scenario: An unchanged night runs nothing
- **WHEN** the nightly full run finds the default branch at the commit its last successful run tested
- **THEN** it skips, and a dispatched run in the same state still runs

#### Scenario: A token-consuming tier never gates a pull request
- **WHEN** the real-runtime tier fails
- **THEN** no pull request is blocked by that failure alone

### Requirement: The pack's scope boundary with continuous integration is stated
The system pack SHALL own the definition of the cluster-based jobs only.
Per-module build, vet and unit test, the envtest suite, and chart lint/render
remain owned by the `continuous-integration` capability.

Neither capability SHALL restate the other's jobs, so that "what CI runs"
has exactly one definition per tier.

#### Scenario: A per-module job is not duplicated here
- **WHEN** a change adds a per-module build or lint step
- **THEN** it is specified under `continuous-integration`, not under system testing

### Requirement: The full tier drives the console through a real browser

The full tier SHALL run `platform/console/ui/e2e-live/` (the real-browser,
real-backend Playwright suite) against the cluster and the console
Deployment the same pack run already stood up, after the pack's own
assertions pass.

This tier SHALL NOT gate a pull request, on the same footing as the
real-runtime lane: it runs only where `end-to-end-testing`'s existing
nightly-when-moved and on-demand rule already admits the full tier.

The pack SHALL supply this suite's required fixtures — a reachable console
URL, a `kubectl` context, and the namespace the pack deployed into — rather
than the suite arranging its own cluster, which stays true to
`global-setup.ts`'s existing contract of pointing at an already-running
install.

#### Scenario: The live browser suite runs inside the nightly full pack

- **WHEN** the full tier runs and the pack's own cluster-based assertions
  pass
- **THEN** `e2e-live` runs against that same cluster's console, and the
  tier fails if any spec fails

#### Scenario: A pull request never provisions this tier

- **WHEN** a pull request runs, whatever files it touches
- **THEN** no job runs `e2e-live`, and no cluster is provisioned for it

#### Scenario: The pack's own failure skips the browser pass

- **WHEN** the pack fails before the console Deployment becomes reachable
- **THEN** `e2e-live` does not run, and the failure is reported as the
  pack's, not as a browser-suite failure

### Requirement: The pack asserts the command bus's cluster facts
The system pack SHALL cover these facts about the command bus, each decided by
a cluster the conformance suite does not run:

1. **Multi-replica claim and complete.** A task claimed on one manager replica
   is completed through another, with the claim token the first replica was
   given. A channel message acknowledged on one replica is reported done on
   another.
2. **Restart durability.** A task accepted before every manager replica
   restarts, and before the bus server restarts, runs once to completion
   afterwards.
3. **Ordering under parallel consumers.** Tasks sharing an ordering key run one
   at a time in submission order while tasks of other keys run alongside, with
   more than one consumer replica claiming.
4. **Backoff, then dead letter.** A task on the `conv` queue
   (a `close` or `reopen`) whose handler always fails retryably is retried with growing gaps and then goes dead. It
   appears in the dead-letter listing, and its id is resubmittable after the
   cool-off.
5. **A channel failure is final.** A channel message the adapter reports failed,
   or never reports before its delivery TTL, is not redelivered. It appears in
   the dead-letter listing, and reconciliation derives it again once its
   id is released after the park cool-off.

Assertions SHALL read the task surface of the manager (`/state/tasks` and
`/state/dead-letters`), never the bus server directly, because no component
but the manager reaches the server.

#### Scenario: A claim crosses replicas
- **WHEN** an adapter reads and acknowledges a channel op on one replica and reports it done through another
- **THEN** the task is `succeeded` and no replica refused the completion

#### Scenario: A restart loses nothing accepted
- **WHEN** a command is accepted and every manager replica and the bus server are then restarted
- **THEN** the task completes once and its effect appears on the conversation exactly once

#### Scenario: Ordering holds with several consumers
- **WHEN** several `close` and `delete` tasks of one root and tasks of other roots are accepted together with more than one replica serving
- **THEN** the log shows the one root's tasks never overlapping and in submission order

#### Scenario: A failing channel message is not redelivered
- **WHEN** an adapter acknowledges a channel message and reports it failed
- **THEN** the bus never offers it again, the dead-letter listing carries it, and a later reconciliation delivers it once its id is released after the park cool-off

#### Scenario: A failing task ends in a dead letter
- **WHEN** a task whose handler fails retryably on every attempt is accepted
- **THEN** its attempts are spaced further apart each time, it goes dead, and the dead-letter listing carries its code

### Requirement: The full tier drives the asynchronous flows end to end
The full tier SHALL drive these flows through the real HTTP surface of the
manager and the console, with the task stream observed throughout:

1. a signal opens a conversation, the conversation is closed, and it is then
   deleted
2. a bulk close of N conversations, observed as N independent closes, each with
   an invocation of its own, since the console fans a bulk close out as N
   ordinary `/close` messages
3. a cascade delete of a root with closed members, observed in member-first
   order
4. an inbound chat message delivered through the Telegram fake Bot API lane,
   and one through the console channel, each becoming one input
5. the console's stream dropped while entries are produced: after it
   reconnects every message arrived once, in order, and none needed a refresh
6. a manager restart mid-flight that loses no accepted command

Each flow SHALL assert on the task read surface and on the conversation
record. A flow SHALL NOT be satisfied by the reply code alone, because a
`202` means accepted.

#### Scenario: A conversation's whole life is observable
- **WHEN** a signal opens a conversation that is then closed and deleted
- **THEN** the task stream shows the ingest, close and delete tasks succeed in order, and the conversation is gone at the end

#### Scenario: A bulk close is N independent closes
- **WHEN** a console closes N conversations in one gesture
- **THEN** each close is accepted with its own invocation id, all succeed independently, and every conversation is `Closed`

#### Scenario: A cascade deletes members first
- **WHEN** a root with closed members is deleted
- **THEN** the log shows each member's `delete` task settle before the root's, and no conversation of the tree remains

#### Scenario: A Telegram message and a console message each become one input
- **WHEN** an inbound message arrives through the fake Bot API lane and another through the console channel
- **THEN** each is accepted, each becomes exactly one input of its conversation, and a repeated delivery of either adds no second input

#### Scenario: A dropped console stream loses nothing
- **WHEN** the console's stream drops while messages are produced and the console reconnects
- **THEN** every message the console never acknowledged is offered again, each arrives once in order, and no refresh is needed

#### Scenario: A restart mid-flight loses nothing
- **WHEN** the manager restarts while accepted tasks are queued or running
- **THEN** every accepted task reaches a terminal state and no command is lost or applied twice
