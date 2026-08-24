## Gotchas (paid for in debugging)

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

**PUBLISHING IS A GIT TAG ONLY WHILE CI RUNS, AND ON A PRIVATE REPO IT DOES
NOT.** `build-test.md` says nothing is pushed by hand; that holds for the public
path and is false here. A tag pushed against a private repo publishes NOTHING
and reports nothing — the absence looks identical to a build still queued.

- **Check the registry, not the tag**, before believing an image shipped.
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
| `k8s-bundle.pipelines` | yes — it owns its whole lane (source, profile, both toolsets), so channels are the only foreign name | **nullable**, so an explicit `false` can decline the route even under demo mode | one |
| `prometheus-bundle.pipelines` | yes, on the same grounds | plain `false` — demo mode never enables that bundle, so there is nothing for an explicit `false` to beat | one |
| `ha-bundle.pipelines` | yes, same plain `false` for the same reason | plain `false` | **two**, because its lane has two privilege levels |
| `telegram-bundle` | **no — the counter-example.** Its routes genuinely span bundles, because a chat surface is answered by an agent from somewhere else | — | none |

**`ha-bundle`'s acting route claims the log source and NO chat source**, so
reaching it is `/ha-ops <task>` and never an accident.

- **Name pipelines for their JOB**, not for the channel they answer on.
- **A SignalSource is NOT claimed by exactly one pipeline.** Sources are
  shareable, so a bundle's route and an install's route claiming one source both
  render and the source fans out to both.
- **That is reported in NOTES.txt, never refused.** Refusing it would be the
  deleted `sourceConflicts` guard returning one layer up.
