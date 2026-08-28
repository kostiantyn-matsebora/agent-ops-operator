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

| Bundle | Qualifies | `enabled` default | Routes |
|---|---|---|---|
| `kubernetes.pipelines` | yes — it owns its whole lane (source, profile, both toolsets), so channels are the only foreign name | **nullable**, so an explicit `false` can decline the route even under demo mode | one |
| `prometheus.pipelines` | yes, on the same grounds | plain `false` — demo mode never enables that bundle, so there is nothing for an explicit `false` to beat | one |
| `home-assistant.pipelines` | yes, same plain `false` for the same reason | plain `false` | **two**, because its lane has two privilege levels |
| `telegram` | **no — the counter-example.** Its routes genuinely span bundles, because a chat surface is answered by an agent from somewhere else | — | none |

**`home-assistant`'s acting route claims the log source and NO chat source**, so
reaching it is `/ha-ops <task>` and never an accident.

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
    component on its own runner, readings as artifacts, the queue built by a
    program. The measurement is what settles "just add more agents": more
    `agent()` calls join the queue behind the two that run.
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
