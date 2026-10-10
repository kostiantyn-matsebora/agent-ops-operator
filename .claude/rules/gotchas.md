## Gotchas (paid for in debugging, twice) (paid for in debugging)

- **RBAC `resources:` are lowercase plurals.** A blanket rename once produced
  `AgentRuntimes` and silently broke the informer — forbidden loops in the log,
  reconciler does nothing.
- **SSH deploy keys in Secrets must be LF-only with a trailing newline.** CRLF
  or flattened-to-one-line keys fail with `error in libcrypto`. Prefer building
  the Secret from base64 rather than shell `--from-literal` interpolation.
- **envtest needs `KUBEBUILDER_ASSETS`.**
- **`kubectl auth can-i` misparses the `pods/eviction` slash form.** Use
  `--subresource=eviction`.

**Tearing down a throwaway release: UNINSTALL FIRST, then clear the
`agentops.dev/close-topics` finalizer.**

- **Clearing it while the manager still runs achieves nothing.** The reconciler
  re-adds it within a second, and then `helm uninstall` removes the only thing
  that could ever release it, so the namespace hangs in `Terminating` forever.
- **The order is the whole trick**, and getting it backwards looks identical
  right up until it wedges.
- **Conversations carry the finalizer even with NO channels bound**, so "no
  chat, no problem" is not a reason to skip this.

**A rendered pod is not a running one, and a chart render test cannot tell the
difference.**

- **`mcpServers` shipped `PROMETHEUS_MCP_TRANSPORT` for a whole implementation
  pass.** The real name is `PROMETHEUS_MCP_SERVER_TRANSPORT`, so the server fell
  back to stdio — and a stdio process in a pod prints a banner and exits, giving
  a `Completed` pod behind a Service that answers nothing.
- **Every guard, every assertion and `--dry-run=server` passed.** Only starting
  the thing found it.
- **Pin env-var NAMES third-party images read**, and smoke any new workload
  before believing its values.

**`helm.sh/resource-policy: keep` protects nothing retroactively.**

- **Helm reads it off the LIVE object** when a resource leaves the manifest,
  never off the manifest dropping it. Adding the annotation in the same release
  that stops rendering the resource DELETES it. Verified against helm v4, all
  three cases.
- **Anything that stops being rendered needs the annotation on the object
  FIRST** — the generated credential Secrets are the case, which is why
  `agentops.generatedSecretGuard` fails the render rather than trusting a
  migration note.

**HELM INSTALLS A CRD FROM `crds/` ONLY WHEN ABSENT, AND NEVER UPGRADES ONE.**

CRDs are CLUSTER-scoped, so they survive everything an install tears down —
`helm upgrade`, `helm uninstall`, and `kubectl delete ns` alike. **A full
wipe-and-redeploy therefore lands on the OLD CRDs.**

- **The API server then PRUNES every field the new version added**, silently. No
  error, no warning, no event. `Pipeline.spec.persistence` and the conversation's
  claim snapshot vanished exactly this way on a redeploy that had deleted the
  whole namespace first — every conversation resolved to EPHEMERAL and answered
  normally.
- **The reinstall case is the one that catches people**, because deleting the
  namespace feels like it cannot leave a migration owed.
- **`kubectl apply -f chart/crds/` is the fix**, and it is a step for INSTALL as
  much as for upgrade.
- **Verify rather than assume** — the symptom of skipping it is silence:

  ```sh
  kubectl get crd pipelines.agentops.dev \
    -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.persistence.type}'
  ```

**A TAG THAT PUBLISHED NOTHING IS SILENT, AND THE PRIVATE REPOSITORY IS NO
LONGER THE REASON.** This repository is PUBLIC, so a `<component>-v<semver>` tag
runs the release workflow and CI publishes. The caveat that it did not — that a
tag pushed here publishes NOTHING and reports nothing — is WITHDRAWN, and
re-adding it describes a repository this is not.

- **The absence still looks identical to a build still queued**, so **check the
  registry, not the tag**, before believing an image shipped. The live reasons a
  tag ships nothing are a FAILED RUN and the package's ACTIONS ACCESS, both in
  `build-test.md`.
- **MORE THAN THREE TAGS IN ONE `git push` TRIGGERS NOTHING.** GitHub creates
  no `push` event for a push carrying more than three tags — documented, and
  silent: the tags land, `release.yml` never runs, and the run list looks like
  nobody tagged. Twelve rebuild tags went that way on 2026-08-27. Push release
  tags THREE AT A TIME; a tag that already landed is deleted and re-pushed,
  since nothing was built from it.
- **The hand build is the ordinary buildx push**, and it MUST stay multi-arch —
  the cluster is mixed x86/arm64, and a single-arch image fails at SCHEDULE
  time, possibly weeks later.
- **Never read a credential to test whether auth works.** `docker-credential-*
  get` PRINTS THE SECRET. Attempt the push and read the error instead; a leaked
  token costs a rotation and a re-login everywhere it was used.

**A RELEASE IS MANY TAGS ON ONE COMMIT, AND THE SMOKE USED TO RUN ONCE PER
TAG.** Chart 13.4.0 shipped fourteen component images from one commit on
2026-09-06: fourteen tags, fourteen identical ten-minute cluster smokes in
parallel on shared runners, then a fifteenth on the chart tag. Both failures of
that release were LOAD, not the code — a k3d image import deadlocked under the
concurrent runs, and the console lifecycle lane timed out on a step that takes
eight seconds when the runners are quiet — and a re-run changed nothing either
time.

- **The fix is a lookup, the same shape `ci_is_green` already uses**:
  `smoke-evidence.py` reads the tagged commit's CHECK RUNS before
  `release.yml` provisions a cluster, reuses a passed one, and waits —
  bounded — for one already in flight rather than racing it. The check run's
  name is `<caller job> / <called workflow> / <called job>`, and it really is
  `smoke / e2e / smoke` — the CALLER job named `smoke`, the reusable workflow
  `e2e.yml` displaying as `e2e`, the CALLED job inside it also named `smoke`
  — whichever of `release.yml` or `e2e-smoke.yml` produced it. See
  `.claude/rules/documentation.md`'s routing and `docs/testing.md`'s tier
  model for what it changed.
- **`gh api <path> -f k=v` WITH NO `--method GET` SENDS THE `-f` PARAMS AS A
  REQUEST BODY ON THIS ROUTE**, and `commits/<sha>/check-runs` answers a
  bodied GET with a plain 404 — not a permissions error, not an empty list.
  Caught live: a first verification push showed `smoke_is_green` logging the
  404 and falling back to "run one" on both a fresh tag and a same-commit
  retag, which is SAFE (never smoked on missing evidence) but silently
  defeats the whole point of the lookup. Always pass `--method GET`
  explicitly on this endpoint; do not trust that an unadorned `gh api` GET
  stays a GET.
- **A `concurrency` GROUP KEYED BY THE COMMIT WAS CONSIDERED AND REJECTED**
  for the same problem. The platform keeps one RUNNING and one PENDING run
  per group and CANCELS the rest — so fourteen tags would become one smoke,
  one wait, and TWELVE CANCELLED RELEASES, each publishing nothing, silently.
  A cancelled release looks identical to a slow one until someone checks the
  registry. Reach for the check-run lookup, never a concurrency group, for
  "the same work happening N times on one commit."

**A ROUTINE SESSION IS SLOWER THAN ITS LOG LOOKS, AND AN IDLE `worker_status`
IS NOT A FINISHED RUN.** The first live run of
`.github/routines/implement-issue.md` dispatched `e2e-smoke.yml`, said "I'll
stop here and wait for the background task notification", and sat at
`worker_status: idle`. Read at that moment it looks exactly like a session that
ended its turn and died — the failure `claude -p` really does have. It had not:
ten minutes later it opened its pull request, correctly labelled.

- **`list_runs` reports `status: active` with `worker_status: idle` for a
  session that is merely between turns.** Neither field distinguishes "waiting
  on a background task" from "over", so a log tail is a SNAPSHOT and not a
  verdict.
- **Check the ARTIFACT, never the transcript**, when asking whether an
  unattended run finished: the pull request, the branch, the comment. This
  repository's own rule for the review says the same thing about a tag and a
  registry.
- **It still argues for dispatch-and-carry-on** in an instruction file: those
  ten minutes bought nothing, and the pull request is what makes a dispatched
  run visible. But the session was not lost, and writing that it was would put a
  fiction in this file.

**`gh variable set` STORES WHATEVER IT IS HANDED, AND A URL COPIED OUT OF A
WRAPPED DISPLAY CARRIES THE BREAK.** The first live fire of `remote-implement`
failed with `http.client.InvalidURL: URL can't contain control characters`,
because `ROUTINE_FIRE_URL` held `trig_01UBwPZ\nb9cN68hvcKxZTx2WH/fire` — the id
split across two lines exactly where a terminal had wrapped it.

- **`.strip()` DOES NOT REACH IT.** The break is INSIDE the value, not at its
  ends, so anything validating a configured URL checks for control characters
  rather than trimming.
- **THE FAILURE SURFACES FOUR FRAMES DOWN, IN THE WRONG PLACE.** `urllib` raises
  from `http.client`, so the runner shows a traceback and the ISSUE — the one
  place a person looks — says nothing at all. A program acting on a label owes a
  readable refusal wherever the label was placed.
- **`printf` rather than `echo`, and read it back with `cat -A`:** a value that
  looks right in `gh variable list` may be wrapped for display rather than
  actually one line.

**A CLOUD ROUTINE'S PUSH RULES AND ITS ENVIRONMENT'S VARIABLES, MEASURED
2026-09-06 SO NOBODY RE-DERIVES THEM.** A routine clones the DEFAULT branch and
may push any branch that is NOT protected, has no open pull request by somebody
else, and carries no commits by somebody else — so a fresh `change/<name>` is
accepted and a branch a person is already working is not. Its GitHub triggers
are `pull_request` and `release` ONLY; there is no `issues` event, which is why
a label reaches a routine through an Actions workflow and its `/fire` endpoint
rather than directly (`.claude/rules/remote-session.md`).

- **THE ENVIRONMENT'S VARIABLES ARE PUBLIC TO EVERY SESSION IT RUNS**, and the
  form says so on its face. A token typed there is published; a credential goes
  under API credentials, where the proxy attaches it per host and makes that
  host reachable whatever the network level.
- **Fire text arrives wrapped in a `<routine-fire-payload>` block labelled
  untrusted**, and the saved prompt must opt in to reading it. That is why the
  payload here is an issue NUMBER and nothing else: a number can be validated
  before it is used, and the session reads the issue itself.
- **Runs count against the account's daily routine cap.**

**`lookup` returns empty on any renderer without a cluster** — `helm template`,
CI, a GitOps controller, `--dry-run=client`.

- **A template generating a value on the UPGRADE path APPLIES a new credential**,
  not merely shows one in a diff. Generate under `.Release.IsInstall` only.
- **A `lookup`-driven guard is silent under `helm template`**, so no chart render
  test can pin it. Verify with `helm upgrade --dry-run=server`.

**A HAND-PATCHED FIELD SURVIVES EVERY LATER `helm upgrade`.**

Helm's three-way merge patches only what differs between the PREVIOUS rendered
manifest and the NEW one, so an unchanged rendered value generates no patch at
all.

- **A `kubectl patch` made while debugging is therefore never corrected.**
  `k8s-ops` carried a debugging icon through five chart upgrades that way.
- **Every signal says it worked.** The release reports success and
  `helm get manifest` shows the DECLARED value, while the live object holds the
  other one.
- **A live patch is undone by ANOTHER live patch**, never by re-syncing.
- **Check the OBJECT, not the release**, when the cluster disagrees with the
  values.

**AN IMAGE ID PULLED ONCE WITH CREDENTIALS MUST BE PULLED WITH THEM FOREVER
— THE KUBELET REMEMBERS.** `KubeletEnsureSecretPulledImages` (on by default
from Kubernetes 1.35 as k3s ships it): once an image ID was pulled through an
`imagePullSecret`, a later pod referencing the SAME ID without matching
credentials is made to pull it again, whatever its `imagePullPolicy` says and
however present the image is.

- **An IMPORTED image shares its ID with a pushed copy of itself**, so the e2e
  pack's registry test pushing the stub as-is made every later stub pod pull
  `docker.io/library/agentops-test-stub-runtime` — and fail — while `crictl
  images` listed the image and `IfNotPresent` was set. The test pushes a
  DERIVED image (one `LABEL` on top) with its own ID.
- **The tell is `ErrImagePull` on an image the node visibly holds.** Before
  suspecting the import, ask whether that ID was ever pulled with a Secret.

**CILIUM ANSWERS A BACKEND-LESS SERVICE WITH EPERM, NOT ECONNREFUSED.**

Under `kube-proxy-replacement: strict` the socket load balancer fails
`connect()` in the pod's own kernel when a ClusterIP has no READY endpoint:

```
dial tcp 192.0.2.187:8080: connect: operation not permitted
```

- **It is a rollout race, not a denial**, and clears the moment an endpoint goes
  ready. kube-proxy would have said `connection refused` at the same instant.
- **`gateway-telegram` logs it once at startup**, reading the offset while
  `channel-telegram` is mid-rollout, then retries every 5s and recovers.
- **Confirm against the ENDPOINT LIST and the ReplicaSet timestamps before
  suspecting policy.** Three sessions read that line as a NetworkPolicy problem.

**A CONFIG-ENTRY SETUP FAILURE LOGS UNDER `homeassistant.config_entries`, NEVER
UNDER THE FAILING INTEGRATION'S OWN LOGGER.** Every other Home Assistant
message names its integration in the LOGGER — `homeassistant.components.tuya.*`,
`custom_components.frigate.*` — so `signal-ha`'s `integrationOf` strips a known
prefix and is done. `Error setting up entry ... for tuya`, `Setup failed for
'zwave_js': ...` and the `not ready yet` / `could not` family all come from the
core `config_entries` module instead, reporting on behalf of whatever failed —
so the domain that matters sits only in the MESSAGE TEXT, never the logger.

- **A prefix-stripping classifier silently mislabels every one of these.**
  `signal-ha` did exactly this until `signals/ha/config.go`'s
  `domainFromConfigEntryMessage` was added: every config-entry failure was
  labeled `"homeassistant.config_entries"` — a value that matches no real HA
  domain — so the rung-1 health predicate (`config_entries/get`, keyed by real
  domain) could never apply to the ONE failure class it exists to confirm, and
  every such record fell to log-recurrence-only verification.
- **The failure mode is silent and total, not degraded.** A config entry stuck
  failing forever, logging once per retry (commonly minutes apart, longer than
  most dwell windows), never recurs within its own window and is dropped as
  quiet — a real, permanent incident produces no signal at all. Confirmed live:
  every `homeassistant.config_entries` dwell entry the deployed adapter had
  recorded (going back weeks) had been dropped this way.
- **Any future log-based classifier keyed on logger-name structure owes this
  the same special case** — the domain has to come out of the message, with a
  fallback to today's identity when the message names none.

**`reply_to_message` IS ONE LEVEL DEEP AND NEVER NESTS.**

**A reply carries the message it answers, and no further.** That message holds
no `reply_to_message` of its own, so a chain walked two links up to recover an
original command finds nil, every time.

- **The menu prompt NAMES the addressed form in its own text** —
  `Reply with the task for /<pipeline>` — and `signal-telegram` reads the first
  slash-token back out.
- **Guarded on `from.is_bot`**, so quoting `/ha-ops` at a colleague starts
  nothing.
- **The question's WORDING is load-bearing**, and is stated on both sides for
  that reason.
- **A payload shape is settled by the live transport or not at all.** It shipped
  broken because the test hand-wrote NESTED JSON Telegram never sends — an
  assumption asserting itself, which a fixture cannot catch.

**Never run two getUpdates consumers against one Telegram bot token** — 409s and
stolen updates.

Migrating from another system, or from the old single-container adapter:

1. Stop its poller.
2. CONFIRM none remains.
3. Start `gateway-telegram`.

**`Channel.spec.config.pollingEnabled` is gone.** Ingest is on when the router
runs.

**The router's bot Secret is the SAME one the Channel uses**, since it polls the
bot the channel sends as, injected by the chart as `TELEGRAM_BOT_TOKEN`.

**It used to be an adapter with a signal-free `SignalSource`** purely to carry
that credential, which then sat at `Wired=False` until some Pipeline faked a
claim. Modelling plumbing as an adapter produced that whole chain.

### THE PARENT CHART IS WHERE WIRING IS DECLARED

**A bundle ships it only under the four conditions.**

**A subchart sees only itself**, while wiring names a profile, sources and
channels that routinely come from DIFFERENT bundles — so one that shipped wiring
could only ever wire ITSELF. Declare routes in the top-level `pipelines:`
values.

A bundle MAY ship its own only when ALL of:

1. **Rendering is behind an explicit wiring flag.**
2. **Every reference to an object the bundle does not itself render is a
   values-supplied NAME**, omitted when unset.
3. **Each Pipeline renders only with its own profile.**
4. **The flag DEFAULTS OFF**, forced on by nothing but a values path whose
   declared purpose is a turnkey install (`global.demo.enabled`), and then only
   the LEAST-PRIVILEGED route.

| Bundle | Qualifies | `enabled` default | Routes | `coordinator` mode |
|---|---|---|---|---|
| `kubernetes.pipelines` | yes — it owns its whole lane (source, profile, both toolsets), so channels are the only foreign name | **nullable**, so an explicit `false` can decline the route even under demo mode | one | SAME flag, SAME routes — `k8s-observe` / `k8s-operate` render as standalone `AgentCapability` objects instead of inline Pipelines, claimed by the chart-rendered Coordinator |
| `prometheus.pipelines` | yes, on the same grounds | plain `false` — demo mode never enables that bundle, so there is nothing for an explicit `false` to beat | one | one `AgentCapability` (`alert-investigator`, named for the PROFILE rather than the route — this bundle ships exactly one) |
| `home-assistant.pipelines` | yes, same plain `false` for the same reason | plain `false` | **two**, because its lane has two privilege levels | two `AgentCapability` objects (`ha-control` / `ha-ops`), never merged — same privilege split |
| `telegram` | **no — the counter-example.** Its routes genuinely span bundles, because a chat surface is answered by an agent from somewhere else | — | none | none — unaffected by `wiringMode`, same as `pipelines` mode |

**`home-assistant`'s acting route claims the log source and NO chat source**, so
reaching it is `/ha-ops <task>` and never an accident.

**`global.agentops.wiringMode` (coordinator-deployment-mode) PICKS WHICH OBJECT
A QUALIFYING BUNDLE RENDERS, NEVER WHETHER IT QUALIFIES.**

- **The same four conditions gate `capabilities.yaml` that gate
  `pipelines.yaml`** — same flag, same values-supplied foreign names, same
  per-route profile gate, same demo-mode exception.
- **Only the rendered KIND changes.** A `Pipeline` under `pipelines` mode, a
  standalone `AgentCapability` under `coordinator` mode, never both.
- **An `AgentCapability` carries no wiring of its own.** Claiming the
  bundle's source, and listing the capability in `agents[]`, is the
  CHART-RENDERED COORDINATOR's job (`chart/templates/coordinator.yaml`).
- **Re-derived, not restated.** The Coordinator reads each bundle's own
  `<bundle>.coordinatorContribution` helper through
  `agentops.coordinatorContributions` — the SAME bundle-registry pattern
  `agentops.defaultRuntimeGuard` already uses, so the Coordinator's claims
  cannot drift from what each bundle actually rendered.

**DEMO MODE'S CONSOLE WIRING DOES NOT CARRY OVER TO COORDINATOR MODE, AND
THAT IS A KNOWN GAP, NOT A DECISION.**

- **Under `pipelines` mode, `kubernetes/templates/pipelines.yaml` claims the
  console's source and binds its channel** on the SAME Pipeline the bundle
  renders (`chart.md`, "THE DEMO WIRES THE CONSOLE").
- **An `AgentCapability` carries no subscription at all**, so that claim has
  nowhere to attach under `coordinator` mode, and the chart renders no
  auto-console-wiring there.
- **`global.demo.enabled: true` with no `wiringMode` override now defaults to
  "coordinator"** (`wiring-mode`'s own spec), so a fresh demo install ships a
  console that cannot start a conversation until someone hand-claims the
  source on the chart-rendered Coordinator.
- **Five tests in `platform/manager/internal/integration/charttemplate_test.go`
  pinned the OLD demo-always-pipelines behaviour** —
  `TestDemoModeWiresTheObservingRoute`, `TestAllowMutationsPromotesTheRouteToActing`,
  `TestExplicitRouteValuesBeatTheDerivation`, `TestBothRoutesRenderWithoutConflict`
  and `TestWiringNamesOnlyWhatWasRendered`. Each now pins `--set
  global.agentops.wiringMode=pipelines` to keep testing the Pipeline path. That
  fixes the TEST SUITE, not the console-wiring gap above, which is still open
  for whoever designs the console's coordinator-mode wiring.

- **Name pipelines for their JOB**, not for the channel they answer on.
- **A SignalSource is NOT claimed by exactly one pipeline.** Sources are
  shareable, so a bundle's route and an install's route claiming one source both
  render and the source fans out to both.
- **That is reported in NOTES.txt, never refused.** Refusing it would be the
  deleted `sourceConflicts` guard returning one layer up.

### A REVIEW SUBAGENT UNDER THE ACTION

**A SUBAGENT ARRIVES WITH EVERY UNSCOPED RULE FILE ALREADY IN CONTEXT.**
Measured on 2026-08-26 for `parallel-component-review`: a `component-reviewer`
spawned from `claude -p` held `CLAUDE.md` and all fifteen unscoped
`.claude/rules/*.md` before reading anything; only the three `paths:`-scoped
rules loaded on demand.

- **So "route the rules to the reviewer that needs them" is free for scoped
  rules and impossible for the rest.** The fixed cost is paid per reviewer, in
  parallel. The lever is scoping more rules, which is a decision about the
  rules.
- **`claude-code-action` REFUSES ANY WORKFLOW FILE THAT IS NEW OR DIFFERS FROM
  THE DEFAULT BRANCH COPY — on every trigger, `workflow_dispatch` included.** A
  spike of the action on a branch cannot run at all. What the action passes
  through verbatim is `claude_args`, so a Claude Code fact — does this
  allowlist spawn, do these run concurrently, what is in a context — is
  settled locally with `claude -p` and the same flags, in a minute.
- **`claudeMdExcludes` MATCHES `**/` GLOBS AND ABSOLUTE PATHS, AND A RELATIVE
  PATH MATCHES NOTHING** — silently, the rule stays loaded. It is the only
  knob over what a subagent inherits: there is no per-agent opt-out, and
  `.claude/rules/` cannot be split from `CLAUDE.md` by any setting, but a glob
  naming a rule file drops that file alone and leaves `CLAUDE.md` in place.
  Session-wide, so it fits a job whose every context wants the same subset;
  the review passes it as `--settings` inline, on the guarded side.
- **THE REVIEW'S FAN-OUT IS A WORKFLOW SCRIPT, NOT A PROMPT, AND THE TWO RUNS
  THAT DECIDED IT ARE ON RECORD.** When the consolidator held the plan, it
  spawned readers as background `Agent` calls, one per message: on #74 it
  ended its turn to "wait" — under `claude -p` the turn is the process, the
  readers died, the run reported success with nothing posted — and on #77 it
  slept seven of ten minutes. A dynamic workflow runs under `-p`, holds the
  loop in code, runs `pipeline()` readers concurrently and returns their data
  validated by schema. Agent teams were not an option: `-p` spawns no
  teammates. If a plan must not be dropped, do not give it to a model turn.
  - **THAT SCRIPT IS GONE, AND THE LOOP IS THE ACTIONS MATRIX NOW — the plan
    still is not a model turn.** The dynamic-workflow runtime pools concurrent
    `agent()` calls at `Math.min(16, Math.max(2, availableParallelism() - 2))`,
    computed once at process start with NO override (read out of CLI 2.1.247);
    `ubuntu-latest` has four vCPUs, so the pool was TWO. On #106 eight readers
    started in pairs — 2:16, 2:16, 4:17, 5:02, 7:54, 7:57, 9:29 — the run was
    stopped at 600 s with the eighth never started, and the coordinator never
    ran. A bigger runner lifts the cap for money; a job per reading lifts it
    for free (twenty concurrent jobs on a public repository). So the unit of a
    reading is a JOB: `claude-review.yml`'s `read` matrix, one `claude -p` per
    component on its own runner — inside it two `file-reviewer` queue readers over a
    queue of the files, the pool's width — readings as artifacts, the queue
    built by a program. The measurement is what settles "just add more agents": more
    `agent()` calls join the queue behind the two that run.
    **KEPT AS HISTORY; IT NO LONGER APPLIES.** `review-built-delta` deleted the
    queue reader and its component session entirely: the unit of a reading is
    now a PROCESS, not a job's internal pool, and its width is the job's own
    shell (`xargs -P $REVIEW_READERS`) — there is no runtime pool to size at
    all, because nothing inside the job is a `claude -p` session spawning
    subagents any more.
  - **SIX MORE CLI FACTS, SETTLED LOCALLY ON 2026-09-05 BEFORE THAT DESIGN WAS
    WRITTEN, SO NOBODY RE-DERIVES THEM** (the review's model and effort, `claude
    -p`, no cluster):
    1. `agent({agentType: 'fork'})` in a workflow script is refused under `-p`:
       the type is not in the registry there.
    2. The Agent tool's `subagent_type: fork` from the model's own turn under
       `-p` is refused with the same message — the fork exists in an
       interactive session and not here, confirmed TWICE, two different ways
       of asking for one.
    3. A skill with `context: fork` runs under `-p` ("forked execution") and
       INHERITS NOTHING from the parent — asked for a nonce the parent had
       just read, it answered UNKNOWN. A fork with `agent: <project role>`
       runs as that role, its system prompt and its tools, and nothing else.
    4. One such fork with a 35 KB fixed body created 9.4 k cache tokens; three
       forks created 9.7 k — the body is written to cache once and read by
       the others.
    5. **Three separate `claude -p` PROCESSES with the same 35 KB in
       `--append-system-prompt`: the first created 22.9 k cache tokens and
       cost $0.096; the second and third created 3.6 k, read 37.8 k, and cost
       $0.023 each.** THE PROMPT CACHE IS SERVER-SIDE, KEYED ON THE PREFIX
       BYTES, AND CROSSES PROCESSES — which is the whole mechanism the
       per-file reader loop spends: the rules routed to a component are the
       same bytes for every file, so only the first process of a job pays
       for them.
    6. Under `-p` the workflow runtime's concurrent-agent pool is
       `min(16, max(2, cpus − 2))`, two on the runner, with no override — fact
       6 restates the measurement two bullets up, because it is the reason a
       shell loop replaced the pool rather than trying to widen it.
- **A RUNNER HAS NO CLAUDE SETTINGS, SO `claude -p` THERE RUNS WHATEVER THE
  DEFAULT IS — AND THAT WAS THE CAUSE OF EVERY SLOW REVIEW NUMBER.** Every
  CI session and every subagent ran sonnet-5 at its DEFAULT EFFORT. The same
  file reader, same prompt, same tools, on one machine: sonnet-5 default
  **198 s and ~16 k thinking tokens**; sonnet-5 `--effort low` **18 s and
  ~200**; fable-5 default 20 s; opus-5 default 83 s; sonnet-5 medium 79 s —
  same findings. Between its last read at +50 s and its answer at +191 s the
  default-effort reader emitted ~13 k thinking tokens and no action; not
  throttling. The coordinator's seven minutes were 55 turns of the same.
  - **The workflow sets `--model` and `--effort` on every `claude -p`, and
    the roles say `model: inherit`**, so the choice reaches every subagent.
    Do not remove the flags to "use the default": the default is the
    runner's, and the runner has none.
  - **ISOLATE THE VARIABLE BEFORE RESTRUCTURING.** A day was spent on the
    matrix, per-file readers, chunking and rule routing — each defensible,
    none the cause — before one local run with `--model` reproduced the CI
    time. The 20-second version of a reader was one flag away throughout.
  - **A SESSION FORCED TO ANSWER BEFORE ITS BACKGROUND WORKFLOW FINISHES
    INVENTS THE ANSWER.** With `--json-schema` the component session emitted
    an empty reading on its first turn, then the real one after the
    completion notification. `review-trace.py` takes only a result after
    that notification; a session that ran no workflow keeps its last.
  - **The CLI stops a background workflow at 600 s**, measured twice (#106,
    and a 15-file component two-wide). The queue emits one read job per two
    files so no job approaches it.
  - **Thread verdicts vary between runs of the same file** — `standing`,
    `fixed`, `gone` for the same five threads at every effort level. Not
    a speed problem; open.
- **A `workflow_dispatch` RUN'S CHECK RUNS NEVER REACH THE MERGE BOX —
  MEASURED ON #131, 2026-08-29, AND IT IS WHY THE FIXING LOOP PUSHES WITH A
  DEPLOY KEY.** A commit pushed with `[skip ci]` (so no `pull_request` run
  existed for it), then `gh workflow run ci.yml --ref <branch> -f pr=131`:
  the run's `ci-green` check run attached to the head sha (the commit's
  check-runs API listed 44 from github-actions, its check suite even listed
  the pull request), yet `gh pr checks` and the pull request's status rollup
  showed NONE of them — only SonarCloud's own sixteen. Branch protection
  reads the rollup, so the required check stays "expected" forever. With
  both a `pull_request` run and a dispatched run on one sha the rollup shows
  the `pull_request` one alone, which is the same fact seen from the other
  side. So a token push cannot be repaired by self-dispatch; the loop pushes
  through `AUTOFIX_DEPLOY_KEY` and lets ordinary events fire.
  - **Sonar analysed the dispatched run as pull request 131 only because the
    action was handed the number** (`-Dsonar.pullrequest.key`); without it a
    hand run submits a BRANCH analysis that decorates and gates nothing.
    That is why the sonar action waits on the quality gate itself
    (`sonar.qualitygate.wait`) instead of relying on Sonar's own check, and
    why `ci.yml`'s `workflow_dispatch` input is kept for hand runs.
- **`claude-code-action` EXITS AFTER THE MODEL'S FIRST TURN, SO A BACKGROUND
  `Workflow` NEVER RUNS UNDER IT.** The tool launches the run and returns at
  once; the CLI stays alive and gives the model a second turn with the result
  (measured locally: 3 min, ten agents, two `result` events). The action
  ended the process nine seconds after the first turn on #94 — the run never
  started and the step read "success". The review therefore runs `claude -p`
  itself, with the credential as env and the stream-json stdout as the
  execution file. The gate on the summary comment is what caught it. Still
  true with no background workflow left: each job is one `claude -p` whose
  one turn IS the reading, and the summary gate is unchanged.
- **An inline `--agents` definition sits inside single quotes in `claude_args`,
  which is split like a shell line.** One apostrophe in the reviewer prompt
  ends the argument early and reads as a JSON error somewhere else. The suite
  counts them.

**`apt-get upgrade` IN A CACHED LAYER PATCHES NOTHING — A CACHED LAYER RUNS NO
COMMAND.** The build cache keys a `RUN` on its text and the build arguments it
reads, so a layer built before a Debian security fix existed keeps the
vulnerable package for as long as the instruction is unchanged. libexpat1 was
the first time, and the upgrade line was added and read as the fix; libssh2
(CVE-2026-7598) was the second, on a layer cached with that line already in
it. The line only ever worked because adding it changed the text. Every
Dockerfile that runs apt now reads `ARG APT_REFRESH`, `ci.yml` and
`build-image.yml` pass the date, and `.github/tests/apt-refresh.test.sh`
pins both halves — the release path shares the cache scope, so without it a
stale layer reaches a PUBLISHED image. A same-day re-run still hits the
cache; the next day's build, or an edit to the `RUN` line, is the refresh.

**ON A `pull_request` RUN, `github.sha` IS THE MERGE COMMIT AND `base.sha` IS
FROZEN AT THE PULL REQUEST'S OPENING — SO `base.sha..github.sha` IS EVERYTHING
THE BASE GAINED SINCE, CHARGED TO THE PULL REQUEST.** `pr-closes` refused #174
for not closing the issues of two changes #179 and #180 had archived on master
days after its branch was cut. A rebase would not have helped: the head would
then hold the archives too. A job that judges "what this pull request changes"
diffs `origin/<base>...<head.sha>` — three-dot against the base branch AS IT
STANDS, to the pull request's own head — and `.github/tests/pr-closes.test.sh`
pins both the fixture and the workflow's range. `fetch-depth: 0` is what makes
the merge base computable.

**WRAPPING A TEST FUNCTION'S BODY IN `t.Run` DOES NOT LOWER SONAR'S
`go:S3776` COGNITIVE COMPLEXITY SCORE FOR IT.** Confirmed against
SonarSource's own answer on the mechanism (a maintainer, on the community
forum): a nested function literal — a `t.Run(name, func(t *testing.T)
{...})` closure, or any `func() {...}` assigned to a local var — increments
the NESTING level for whatever control flow sits inside it, and that cost is
charged to the ENCLOSING function, not reset for the closure as though it
were a separate unit.

- **The technique that actually works: extract the control-flow-heavy body
  into a genuinely separate, NAMED top-level function**, called BY NAME from
  the flagged function (`t.Run("case", assertFoo)` or
  `t.Run("case", func(t *testing.T) { assertFoo(t, ...) })`, the wrapper
  costing nothing since it holds no control flow of its own). A plain
  function call is not a nesting-increasing construct — only
  `if`/`for`/`switch`/`&&`/`||` and nested function bodies are.
- **`sonar-ratings-baseline` first tried this the wrong way, and caught it
  before it shipped**: two functions were reshaped into `t.Run` subtests,
  verified to compile and pass, and were about to be called done — until
  re-deriving how Sonar scores nested closures showed the reshape did nothing
  for the metric being fixed. Both were redone as named helpers before
  landing.
- **Reuse candidates across one file are common** — a repeated "find this doc
  by kind and name" search, a repeated table-driven assertion — so extracting
  the first flagged function in a file often produces a helper (`findDocByKindAndName`,
  `countDocsContaining`, …) that the NEXT flagged function in the same file
  can call directly, without writing its own extraction.

**A REMOTE SESSION CANNOT APPROVE ITS OWN WORK, AND IT COST A PULL REQUEST
ITS AUTOMATIC FIXING — MEASURED LIVE ON #201.**
`.github/routines/implement-issue.md` told a session to open its pull request
with `gh pr create --label autofix`. This was exactly what `change-delivery`'s
spec said to do at the time.

RETIRED: `conveyor-labels` renamed that label `conveyor:fix`. How it now
reaches the pull request without the session placing it is the second bullet
below.

`review-dispatch.yml`'s gate asked the collaborators API whether the labeller
may push here. The labeller was `claude[bot]`, an application with no write
access, and the answer was `none`.

The gate removed the label and posted a refusal. Everything behaved as
designed except the design: the pull request sat green, reviewed and
unlabelled, with no session left to do anything about it.

- **THE GATE WAS RIGHT AND DID NOT MOVE.** A label that decides code gets
  written to a branch counts only from someone the platform says may push,
  and a bot exempted from that check is the hole the whole mechanism exists
  to close.
- **THE FIX: A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER
  MINT ONE.** `conveyor-labels` retired `autofix`/`autoimplement` into the
  `conveyor:` vocabulary and made the session place NO label on its own work,
  ever.
  - Where a person's standing instruction (`conveyor:run`, placed on the
    tracking issue) authorises the next station, a WORKFLOW —
    `.github/scripts/carry.py` — reads that instruction again at the
    moment it matters.
  - It re-checks the placer still has write access, then places the
    station's label itself, recording whose grant it carried.
  - The gate then passes because the label was placed by
    `github-actions[bot]` acting on a CHECKED grant, not asserted by the
    thing being decided about.
- **A CARRIED LABEL IS RE-CHECKED, NEVER TRUSTED.** `review-dispatch.yml`'s
  gate accepts a label placed by `github-actions[bot]` only after re-reading
  the originating issue's `conveyor:run` and confirming it is still there and
  still a writer's — the same live-read property the gate already had for a
  person's own label.

**THE `open` JOB GATED ITS OWN CARRY ON THE GREEN STATE THE CARRY EXISTS TO
PRODUCE — MEASURED LIVE ON #51.** `conveyor:run` on issue #51 fired the
session, which opened pull request #220 correctly, unlabelled.

The review posted findings, and `ci-green` went `failure` — it `needs:
review-clean`, which fails the whole `ci` run whenever the review found
something. `conveyor:fix` never landed, because `remote-implement.yml`'s
`open` job only fired when the triggering `ci` run's conclusion was
`success`.

- **A CHICKEN-AND-EGG GATE, NOT A MISSING ONE.** `open` exists to carry
  `conveyor:fix` onto a pull request so the fixing loop can act on the
  review's findings. Requiring `ci` to already be green before that carry
  happens means it can never fire on exactly the pull requests that have
  findings to fix — the one case the whole mechanism is for.
- **THE FIX IS ONE BOOLEAN, NOT A REDESIGN.** `open`'s `if:` now accepts
  `conclusion == 'success' || conclusion == 'failure'`.
  `carry.py` reads no CI status at all. They
  already re-check the writer's `conveyor:run`, the `change/*` branch shape
  and `Refs #<n>`, independent of why `ci` concluded — so widening the
  trigger costs nothing the re-check does not already guard.
- **`archive` KEEPS `success`-ONLY, DELIBERATELY.** It fires on a `push` to
  the default branch, which only happens on an actual merge. A `push`-event
  `ci` run concluding `failure` on `master` is a different incident — a
  broken main branch — with nothing to do with this bug.
- **THE MISLEADING COMMENT WAS THE FIRST THING TO FIX.** The `open` job's own
  header argued at length that `success`-only was CORRECT: "this is what
  closes the gap a carried label used to fall into." That reasoning was
  backwards — it described the gate as protecting against a premature carry,
  when it was instead preventing the only carry that mattered. A comment
  defending the wrong behavior at length reads as considered, which makes it
  a bigger risk than no comment at all.

**THE CARRIED ROUND WAS REFUSED BY THE MODEL ACTION, AND THE REVIEW'S VERDICT
WAS FROZEN — MEASURED LIVE ON #220, 2026-09-13 TO 2026-09-16.** #221 made
the `open` job dispatch `review-dispatch.yml` after carrying `conveyor:fix`,
and its first live run died in the `fix` job before any model ran:

```
Workflow initiated by non-human actor: github-actions (type: Bot). Add bot to allowed_bots list
```

- **`claude-code-action` accepts NO bot by default**, and a `workflow_dispatch`
  fired by a workflow has `github-actions[bot]` as its actor. The fix job now
  names that one bot in `allowed_bots`. Not `*`: on a public repository that
  lets an external App start the step with a prompt it controls.
- **`land` ran only on a successful or skipped `fix`**, so the failure reached
  nothing a person reads. It now runs on a failed `fix` too and posts `fixing
  step failed`.
- **The review's `reconcile` job failed the review RUN on an open thread**, so
  `review-clean` — reading that run's conclusion — stayed red after the owner
  resolved the thread, since no event re-ran anything. **A CHECK MUST NEVER
  CARRY THE THREAD QUESTION**: branch protection's required conversation
  resolution already blocks the merge on an open thread, live at merge time.
  `reconcile` fails on nothing but its own work now, and `review-clean` asks
  only whether the review ran.
- **`pull_request_review_thread` IS A WEBHOOK EVENT, NOT AN ACTIONS TRIGGER.**
  The first repair — re-run `review-clean` on a thread resolution — shipped
  as `review-thread.yml` and was REFUSED by the platform: `Unexpected value
  'pull_request_review_thread'`. The tell is a failed run with ZERO JOBS,
  named by the file's path, on EVERY push to EVERY branch, with nothing in
  `gh run view --log`. The message is on the run's web page alone. The
  script suite parses each workflow's `on:` keys against that name now.
  Local YAML parsing proves nothing about which events exist.
- **A COMMENT IS AN EVENT, A THREAD RESOLUTION IS NOT.** `issue_comment` and
  `pull_request_review_comment` are Actions triggers, so the check that reads
  the conversation (`docs-task`, through `autofix-guard.py`) IS re-runnable
  on the answer it waits for — `dispute-answered.yml` does exactly that, and
  it is the last manual re-run #220 needed. The thread question stays with
  the platform's merge box.
- **A failed job of a review run cannot be re-run after a day.** Its
  `resolve-threads` artifact has expired (`retention-days: 1`), so a re-run
  of the failed job fails again, this time in the step named "The review's
  list actually arrived", with `Artifact not found for name: resolve-threads`.
  Re-running the review means a FRESH run: a push, or a hand dispatch on the
  branch so the head sha matches:

  ```sh
  gh workflow run claude-review.yml --ref <branch> -f number=<pr>
  ```
- **Nothing fired on `conveyor:archive`.** The carry placed it and
  `remote-implement.py` accepted the implement and run labels only, so the
  opsx lane's last station had no actor. It fires the same routine now,
  routed to `archive-change.md`.
- **A `workflow_run` job that reads labels RACES the job that places them.**
  The dispatch's own `workflow_run` gate on the same `ci` completion read the
  labels one second before the carry landed. Harmless only because the carry
  dispatches explicitly. Never rely on two `workflow_run` jobs ordering.

**A `run-name:` OVERRIDES `workflow_run.name` FOR EVERY LATER LISTENER, SILENTLY.**
`claude-review.yml` sets `run-name: "Review of #<n>"`. A receiving workflow's
`workflow_run.name` is the RUN's own name — the `run-name`, once one exists —
never the workflow's static `name:` field. `review-dispatch.yml`'s `gate` job
matched the literal `'claude-review'`, and that stopped matching anything the
moment `run-name` shipped.

- **Measured live on #233.** Two fixing rounds landed, a review completed
  clean after the second, and the condition stayed false forever — no round
  ever started against the review's next findings, and `loop:running` sat
  wrong on a pull request nothing was working on.
- **The FIRING TRIGGER (`on.workflow_run.workflows: [claude-review]`) still
  worked**, because THAT match is against the declared `name:`, a different
  field entirely. Only the payload read INSIDE the job's own `if:` used the
  wrong one — so the run fired, `gate` evaluated, and evaluated false, which
  looks identical to "nothing happened" from outside.
- **The fix is `workflow_run.path`**, the workflow FILE's path
  (`.github/workflows/claude-review.yml`), which carries no per-run wording
  and cannot drift when a `run-name` is added or reworded later.
- **Any future `workflow_run.name` match anywhere in this repository owes
  the same check**: does the fired workflow set `run-name`? If so, match on
  `.path` instead, or the condition is dead from the day one ships.

**A FIXING JOB WITH NO `timeout-minutes` CAN HANG FOREVER, AND `land`'S
FIX-FAILED PATH NEVER SEES IT — MEASURED LIVE ON #243.** The `fix` job's
`claude-code-action` step sat `in_progress` for 15+ hours.

`land` never ran. It `needs: [gate, collect, fix]`, and none of the three
`needs.fix.result` values it matched (`success` / `skipped` / `failure`) is
what a job with no terminal result reports — it has no result at all.

`loop:running` stayed posted with nothing behind it, reading exactly like an
active round to anyone checking the label.

- **This is a DIFFERENT gap from the one #220 fixed.** That fix covered a
  `fix` job that FAILED outright (the action refusing a bot actor). Nothing
  there helps a job that neither fails nor finishes — GitHub's own default
  timeout is 360 minutes, and the action can also hang inside that window on
  its own, with no bound of its choosing either.
- **The fix is a `timeout-minutes` on the `fix` job**, sized to what one
  round of fixes plausibly needs (30, here — multi-file, may run a build or
  reproduce a failing check) rather than left at the platform default.
- **`cancelled` IS NOT `failure`, AND THAT DISTINCTION IS GITHUB'S, NOT THIS
  WORKFLOW'S.** A job that hits `timeout-minutes` reports `conclusion:
  cancelled`. Adding the bound alone would have swapped one silent hang for
  a different silent stop: `land`'s `if:` still would not have matched, for
  the same reason it did not match "no result at all". Every place that
  reads `needs.fix.result` — `land`'s own `if:`, the step that substitutes
  an empty patch and report, and the flag passed to `land-dispatch.py` —
  needs `cancelled` added beside `failure`.
- **THE POSTED ENDING NAMES WHICH ONE HAPPENED.** `--fix-timed-out` is a
  distinct flag from `--fix-failed`, not the same flag renamed: a maintainer
  reading "fixing step failed" goes looking for a crash, and one reading
  "fixing step timed out" goes looking for a hang or a round that genuinely
  needed longer — conflating the two into one message would have answered
  a question nobody asked and left the real one open.

**THE FIXER'S SCRATCH LANDED IN ITS OWN COMMITS, AND THE PROMPT WAS THE CAUSE
— MEASURED ON #243, SIX ROUNDS RUNNING.** Every conveyor round on that pull
request committed a zero-byte helper at the repository root beside the real
fix.

The names say what happened: `.tmp_getenv.py`, `.tmp_hello.sh`,
`.tmp_copy_worklist.py`, `.tmp_read_worklist.sh`, `.worklist_reader.sh`,
`.scratch_read.sh`.

- **The prompt handed the model `$WORK_LIST` and `$REPORT`** — env names on
  the action's step, which a model holding Read, Write and a handful of
  `Bash(go:*)`-shaped grants CANNOT EXPAND. No shell, no `cat`, no `echo`. So
  it wrote a script into the checkout to read its own inputs, could not run
  that either, and eventually guessed the path.
- **`git add -N . && git diff --binary` cut the patch**, so every untracked
  file in the working tree crossed to the landing job and was committed.
- **THE CHAIN THAT MADE IT A RED CHECK:** the review found the scratch file,
  the next round DISPUTED that finding (the file was not part of any
  accepted item), and `autofix-guard.py` then failed `docs-task` until a
  person answered a dispute about a file nobody wrote on purpose.
- **THE FIX IS BOTH HALVES.** The prompt names real paths
  (`${{ runner.temp }}/dispatch/...`, expanded by the workflow) and a scratch
  directory outside the checkout. `dispatch-patch.py` then lands a new file
  ONLY where the report's `created` list declares it, deleting the rest
  before the cut and naming them on the pull request. Telling the model
  alone is trust. The program is the boundary.
- **A ROOT DOTFILE THAT IS EMPTY IS THE TELL.** `git show --stat` on a
  conveyor commit listing a `| 0` file at the root is this bug, whatever its
  name.

**TEN COPIES OF ONE RULE DRIFTED, AND THE FIX WAS THE MACHINE, NOT THE
COPIES — MEASURED ON #248, #254 AND #255, 2026-09-25 TO 2026-09-26.** Each
break was fixed where it showed, and the next one showed somewhere else.

- **#248: THE LOOP FED ON ITS OWN REFUSAL.** A round against more than a
  hundred open analysis issues hit the fixing job's thirty-minute limit and
  counted for nothing. While it ran, `docs-task` refused on "a round is still
  running", so `ci-green` was red, and the red started the next round. Six
  replies on review threads queued six dispatch runs, and each failed
  `docs-task` in its first ten seconds. The pull request merged only after
  `conveyor:run` was removed by hand.
- **REMOVING THE GRANT WAS THE WRONG FIX.** It stopped the cycle by taking the
  instruction away, which also removed the archive station's actor. The cycle
  was two rules disagreeing, and taking a person's instruction away never
  answered which of them was right.
- **#254: THE GATE AND THE CARRY READ DIFFERENT GRANTS.** The carry accepted
  `conveyor:archive` for the archive pull request. The gate grepped for
  `conveyor:run` alone, so it refused every round the carry had authorised.
- **#255: THE ARCHIVE CARRY FIRED ON A PROPOSAL MERGE**, and the fire record
  was permanent, so `conveyor:run` on a finished change started the implement
  station again. Two paths could also start two sessions for one issue.
- **THE FIX IS `conveyor.py`.** A rule has one definition and every program
  asks it. A test walks every state against every event, so the next
  disagreement is a failing row and not a Saturday night.
- **THE GUARD HAD TWO PURPOSES, AND THE `ci` ONE IS GONE.** `--purpose ci`
  made `docs-task` fail on an unanswered dispute. #259 showed that red was one
  no fixer could clear (below), so the archive command is the guard's only
  caller now and a check asks nothing about the loop's conversation.
- **A COMPLETION IS A START.** A review or CI completion on a carried label
  re-checks the grant like a label event does. Before, a completion trusted the
  label, so removing the instruction did not stop a loop that was running.
- **A PR THAT EDITS THE LOOP IS DRIVEN BY HAND.** The loop's workflows run from
  the base branch, so a fix to them is not exercised until it merges. Do not
  give such a pull request a `Refs` or `Closes` keyword on the issue whose
  grant would put the loop on it.

**THE LOOP STOPPED ON A RED IT MADE ITSELF, CALLED IT CLEAN, AND COULD NOT HEAR
THE ANSWER — MEASURED ON #259, 2026-09-26.** Two rounds, then `loop:stalled`
beside `docs-task` and `ci-green` red and a summary reading "clean". A
`conveyor:keep-going` round an hour later posted the same words.

- **THE DISPUTE WAS MANUFACTURED.** The fixer claimed a CRD finding fixed, the
  patch did not touch the file, and the landing step posted "reported fixed,
  but the patch does not touch" as a dispute for the person to answer. A failed
  fix is not a statement about the finding. It is UNADDRESSED now, eligible
  again, and a round that lands nothing starts the next round itself.
- **THE GUARD MADE THE RED, AND AN EXEMPTION HID IT.** `autofix-guard.py
  --purpose ci` failed `docs-task` on the unanswered dispute, and
  `check_is_work` classed a check that failed only on that step as "waiting",
  off the work list. The red was the loop's own, the open thread already held
  the merge through branch protection, and the check added nothing but the red.
  The guard is the archive hook's alone now, `dispute-answered.yml` and
  `rerun-ci-job.py` are deleted, and every failed required check is work.
- **THE ANSWER WAS NEVER HEARD.** The person resolved the thread. A thread
  resolution is not an Actions trigger, and the keep-going round read the stale
  check's failed step name instead of the live threads. A person's comment now
  starts a round on a `loop:waiting` pull request, and `conveyor-sweep.yml`
  re-reads waiting pull requests every fifteen minutes for the resolution
  nothing else can hear.
- **"0 FAILURES" 38 SECONDS INTO A TEN-MINUTE CI RUN.** A check run exists
  queued from the moment its run starts, so "every required job has a check
  run" was true at once. `failed-checks.py` counts the checks consulted only
  when every one has COMPLETED, and the gate starts a round only once the
  head's CI run and review run have both concluded.
- **THE STATE MACHINE WAS RIGHT ABOUT THE FACT IT WAS GIVEN.** `conveyor.py`'s
  table was exhaustively tested and behaved as specified. The adapter fed it a
  stale fact. A table test covers the table, and the scenario "dispute already
  dismissed, guard check still red from an earlier run" had no test anywhere.

**`claude-code-action` VALIDATES THE CHECKED-OUT WORKFLOW FILE, NOT THE ONE
RUNNING — MEASURED ON #259, 2026-09-26.** The message is "The workflow file
must exist and have identical content to the version on the repository's
default branch".

A `workflow_dispatch` round runs master's `review-dispatch.yml`, but the `fix`
job checks out the PULL REQUEST'S branch, and the action reads the copy there.

- **So every branch cut BEFORE the file last changed on master is refused**,
  without editing the file. The night the loop's own change merged, the one
  labelled pull request went from "waiting" to "disputed" with the reason "this
  branch edits review-dispatch.yml", which was false.
- **The fix is a checkout of the default branch's copy before the action** and
  the branch's own copy back before the patch is cut. The fallback that read
  the difference as an edit now says only that the copies differ.
- **The rule in `gotchas.md` above about a spike on a branch is the same
  fact from the other side**: a branch cannot run its own edit of the
  workflow, and a branch that lags cannot run the merged one until its
  checkout carries it.

**A CHANGE MERGED IN PHASES STOPPED AFTER EACH ONE, AND CONVEYOR:RUN STOOD THE
WHOLE TIME — MEASURED ON #53, 2026-09-27.**

`carry_archive`'s own docstring named the fix for #255: an unfinished merge (a
proposal or an apply) moves the line to `implement` instead of firing the
archive station. It stopped there.

Moving the station's LABEL is not the same as RESTARTING the session, and
nothing else ever restarted it. Two phases in a row needed a person to remove
and re-place `conveyor:run` before the next implement session would start.

- **`fire`'s OWN RESTART LOGIC ALREADY EXISTED, AND `carry_archive` NEVER
  REACHED IT.** A person re-placing a label restarts a station that fired
  before, when nothing is open from the change's branch — exactly the case an
  apply merge is, arriving through the merge instead of through a person
  noticing it. `carry_archive` returned `nothing` before that logic could ever
  run.
- **THE FIX IS NOT A SECOND RESTART RULE.** `carry_archive` now asks for the
  SAME check `fire` already does — fired before, nothing open, restart — and
  `fire` gained one flag, `restart_ok`, to let a BOT-CARRIED placement through
  that specific wall. Every other bot-carried path is unaffected: `restart_ok`
  defaults `False`, and the wall it opens is a station `carry_archive` already
  re-verified is safe to restart, never a general licence for a carry to
  restart anything.
- **THE PAYLOAD SAYS SO EXPLICITLY**, never inferred: `carry.py`'s `restart`
  action emits `fire_label` set to `archive_label` for a finished change, or
  `implement_label` for a restart, and the workflow sets `"restart":true` in
  the synthesized payload only when `fire_label` equals `implement_label`.
  `remote-implement.py` reads it and passes it straight to `fire` as
  `restart_ok`.
- **A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER MINT
  ONE — AND THIS DOES NOT MINT ONE EITHER.** The grant restarting the station
  is still `conveyor:run`, still re-checked against whoever placed it, still
  the same standing instruction a person gave once. What changed is which
  EVENTS get to ask `fire` for a restart, not who may authorise one.

**RAISING `manager.replicas` SILENTLY BROKE CHAT DELIVERY, AND THE OpQueue WAS
IN-MEMORY, PER-PROCESS, THE WHOLE TIME — MEASURED LIVE, 2026-10-08.**
`internal/chat.OpQueue` is a plain Go map, built once per process and
populated only by the leader-elected Conversation reconciler.

`/channel/ops` — what a channel adapter long-polls to claim ops — was
explicitly NOT leader-gated. With more than one manager pod behind one
Service, a poll landing on the non-leader saw a permanently empty queue.

- **`ensure-topic` never completed for roughly half of all new chat-bound
  conversations.** They sat in phase `Queued` forever, idling out and
  cycling their runtime pod on `RUNTIME_IDLE_TTL_M`, with no error anywhere
  — the same silent-capacity-loss shape as the 2026-08-20 incident two
  entries up, one layer over.
- **The stopgap was `replicas: 1`, hardcoded rather than a value** — an
  overridable number is how it reached 2 the first time, and a number
  someone can set back is not a fix for a bug that only shows up when they
  do.
- **`durable-chat-ops-broker` is the real fix, not a bigger hardcoded
  floor.** A claim on the Conversation CR itself, written only by the
  current leader-election Lease holder, recovers a crashed leader's
  in-flight work through the SAME failover that already gates every
  reconciler — no second, hand-rolled heartbeat. A poll on a non-leader
  rejects with 503, retried at once, rather than answering from a queue
  that replica's own reconciler never populated.
- **`replicas` is a value again, default 2**, on the same grounds the
  hardcoded version argued FOR safety: the number now means something,
  because the thing that made raising it dangerous is what changed.

**THE LEADER CHECK THAT SHIPPED FOR THE ABOVE NEVER MATCHED, EVEN FOR THE
REAL LEADER — MEASURED LIVE, 2026-10-10.** `isLeader()` compared
`ReplicaIdentity` (bare `os.Hostname()`) against the Lease's
`HolderIdentity` for EQUALITY.

- **The two strings can never equal, on any install, at any replica
  count.** controller-runtime's own `leaderelection.NewResourceLock` mints
  the Lease's holder as `os.Hostname() + "_" + a per-process uuid` —
  confirmed reading `leader_election.go` itself.
- **Every poll 503'd forever, unconditionally** — not the ~50% silent drop
  this whole feature exists to fix, a 100% chat outage.

- **CI never caught it because nothing exercises a real Lease.** Both the
  unit tests and envtest construct a `Server` with `ReplicaIdentity` set by
  hand, comparing against a hand-set `HolderIdentity` with no uuid suffix —
  the fixture matched the bug's own wrong assumption.
- **Fixed to a PREFIX match**: `holder == identity ||
  strings.HasPrefix(holder, identity+"_")`. Safe because a Kubernetes pod
  hostname is a DNS-1123 name — no `_` — so the delimiter rules out `pod-1`
  false-matching `pod-10_<uuid>`.
- **FOUND BY A SECOND, INDEPENDENT SESSION ON THE SAME MACHINE, CONCURRENTLY
  WORKING THE SAME WORKTREE.** `worktree-delivery.md`'s one-session-per-
  worktree rule is why that is noted rather than normal — the fix landed,
  verified, and pushed (`b49a617c`) before the session that first reported
  the live symptom had finished writing it up.

**FIXING THE IDENTITY CHECK UNMASKED A SECOND, DEEPER BUG THE FIRST ONE HAD
BEEN HIDING: A NON-LEADER'S CONNECTION STAYS PINNED TO THAT NON-LEADER
FOREVER.** "Retry at once" (section 4 above) assumes the retry reaches a
different replica.

- **It does not, over a kept-alive HTTP/1.1 connection.** A Kubernetes
  Service picks a backend per TCP CONNECTION, not per request.
- **A connection that happened to dial a non-leader keeps asking that SAME
  non-leader on every later poll.** kube-proxy is never consulted again
  until the connection closes for some unrelated reason.

- **Invisible at 2 replicas on good luck, certain to surface harsher.**
  Asked for explicitly: raising the live-verification install from 2 to 3
  replicas (to make the non-leader landing MORE likely, not less) turned an
  occasional stall into a reliable repro — 0/10 conversations delivered,
  for minutes, surviving a full pod restart of the adapter (a fresh process
  dials fresh too, and can pin just as badly).
- **Silent, because the first fix made 503 deliberately unlogged.** The two
  defects compounded: a livelock that never once reports an error.
- **Fixed with `m.HTTP.CloseIdleConnections()` on the 503 path**, in BOTH
  shipped adapters (`platform/console/manager.go`,
  `channels/telegram/manager.go`) — closing the just-used connection forces
  the NEXT poll to dial fresh, giving kube-proxy another chance. Verified
  with `httptrace.ClientTrace.GotConn.Reused`: false after the fix, true
  (and the regression test fails) without it.
- **A HAND-PATCHED DEPLOYMENT IMAGE REVERTS ON ITS OWN, AND IT IS NOT HELM'S
  DOING.** `kubectl set image deployment/agentops-adapter-console ...`
  appeared to work and then silently reverted to the old tag within
  seconds. The `ChannelAdapter` reconciler owns that Deployment
  (`adapters.md`) and re-applies the CR's own `spec.image` on its next
  reconcile — a different mechanism from the Helm three-way-merge gotcha
  above, same symptom. Patch the `ChannelAdapter` CR's `spec.image`, never
  the Deployment it renders.

**A THIRD BUG, ONLY REACHABLE ONCE THE FIRST TWO WERE FIXED: A RUN'S REPLY
CAN BE MARKED DELIVERED WITHOUT EVER REACHING THE THREAD.**
`deliverRunReplies` iterated `conv.Status.Threads` directly rather than
through `ThreadFor`, which already treats an empty `ThreadID` as "does not
exist."

- **`setClaim` writes a `ThreadBinding` carrying a `Claim` but no
  `ThreadID` BEFORE `ensureTopics` completes** — durable-chat-ops-broker's
  own mechanism, introduced by this same change.

- **The reply was enqueued with an empty thread id anyway.** The adapter's
  own fallback (`thread := "channel:" + op.Channel` when the id is empty,
  meant for a channel-level notice with no specific thread) accepted it,
  delivered it to a pseudo-thread nobody reads, and marked the run
  `Delivered` — which `deliverRunReplies` never re-checks once true. The
  real thread `ensureTopics` created moments later never received the
  answer the person was waiting for.
- **Measured at roughly 1 in 10** under the 3-replica parallel lane
  (`TestReplicasThreeDeliversEveryConsoleThreadInParallel`) — the specific
  hit rate needs both ops racing in the SAME reconcile pass, which plain
  sequential starts (the original `TestReplicasTwoDeliversEveryConsoleThread`)
  essentially never produces.
- **Fixed by skipping delivery while `ThreadID == ""`**, while still marking
  the channel `owed` in the `DeliveryPending` condition — the difference
  between "not yet" and "never coming."

**PATCHING A DEPLOYMENT'S POD TEMPLATE DOES NOT UNBLOCK A ROLLOUT AN
EXISTING POD'S HARD ANTI-AFFINITY IS BLOCKING — MEASURED LIVE, LOCALLY AND
IN CI.** Testing past the chart's own default (two replicas) needs a
single-node cluster to schedule a third pod.

- **Clearing `spec.template.spec.affinity` and waiting for `kubectl
  rollout status` stalled forever**, at `"1 out of N new replicas
  updated"` — on both a local Rancher Desktop cluster and the e2e pack's
  own k3d cluster in CI.

- **A pod's spec is immutable once created.** The EXISTING pod from before
  the patch still carries the OLD anti-affinity rule, and the scheduler
  still refuses to place a new pod beside it — the Deployment's template
  change only reaches pods created AFTER the patch, and none can be
  created while the old one still occupies the only node.
- **Deleting the blocking pods was tried next, and also failed.** Their
  OWNING ReplicaSet's desired count is untouched by a pod deletion, so it
  recreated them — identical, still carrying the rule — within seconds.
- **The fix: scale that ReplicaSet itself to zero**, identified by
  `spec.template.spec.affinity != nil` so a LATER call that only changes
  the replica count (affinity already cleared) never zeroes the
  Deployment's current, correct ReplicaSet and fights its own controller.
- **`TestReplicasTwoDeliversEveryConsoleThreadInParallel`'s dispatch to
  `e2e-smoke.yml` is what caught the second half of this**: the EXISTING
  `TestReplicasTwoDeliversEveryConsoleThread` lane's own precondition
  (`len(pods) >= 2`, no readiness check) had been passing this whole time
  against exactly ONE ready manager pod on this same single-node cluster —
  the chart's own default install never actually ran two REAL replicas in
  CI until this fix was written.

**SCALING THE OLD REPLICASET TO ZERO STILL WASN'T FAST ENOUGH, AND THE
FAILURE CASCADED — MEASURED ON THE VERY NEXT DISPATCH.** The fix above
only ASKS the old pod to terminate.

- **Gracefully, up to its 30s `terminationGracePeriodSeconds`** — and it
  keeps blocking scheduling the whole time it is still there.

- **The symptom changed, which is what gave it away.** The rollout no
  longer stalled at "1 out of N updated" (scheduling), it stalled at "0 of
  N updated replicas are available" (readiness) — the new pods WERE
  scheduling, just not before the old one finally left.
- **Fixed by force-deleting the old pods** (`client.GracePeriodSeconds(0)`)
  right after zeroing their ReplicaSet, instead of waiting out a shutdown
  nothing needs graceful. Verified by hand: 23s end to end, against a
  multi-minute hang before.
- **A SEPARATE bug turned that one failure into several.**
  `scaleManagerReplicas` registered its restore `t.Cleanup` AFTER the
  risky scale-up call. `t.Fatal` stops the calling goroutine at once via
  `runtime.Goexit`, so a failed scale-up skipped the registration
  entirely — the Deployment sat stuck mid-rollout for every test that ran
  after, and several unrelated lanes failed with it
  (`TestContextSurvivesLosingThePod`, `TestAdmissionFIFOOnPodDelete`,
  `TestStubMechanisms/*`).
- **Register `t.Cleanup` BEFORE the call it cleans up after, always** —
  the general form of the bug, not specific to this file. A cleanup
  registered only on a success path is a cleanup that does not run on the
  one path it matters most for.

