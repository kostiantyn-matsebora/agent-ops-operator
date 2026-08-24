## 1. Verify every claim before writing it

D7 in `design.md`: each of these becomes a public statement about a security
property. Verify against the tree, not against this change's own research. Record
the VERDICT, never a matched value.

- [x] 1.1 Manager holds no `secrets` verb — confirm no `secrets` resource appears
  in the manager's ClusterRole in `chart/templates/rbac.yaml`
- [x] 1.2 Adapter tokens are derived, not stored — confirm
  `DeriveAdapterToken` / `DeriveSignalAdapterToken` in
  `platform/manager/internal/chat/token.go` and that the manager reads no Secret
  to validate one
- [x] 1.3 Runtime pods are non-root with per-conversation workspace isolation —
  confirm the pod `SecurityContext` and the workspace `SubPath` in
  `platform/manager/internal/runtimepod/podspec.go`
- [x] 1.4 Context isolation, per mode — confirm that under `contextSync` the
  agent container mounts only an `EmptyDir` and the durable claim is mounted
  solely by the sidecar, and that a pod short of any of the three conditions
  (declared paths, sidecar image, durable claim) is the unsynchronised one, whose
  claim is mounted whole into the agent container. **Both halves**, since the
  page states both
- [x] 1.5 The default install IS the isolating mode — confirm in the shipped
  values that `claude.enabled`, that bundle's `contextSync.paths`,
  `persistence.context.enabled` and the sidecar image are all set. The page's
  context section rests on this, and it was the OPPOSITE before
  `context-sync-by-default` landed
- [x] 1.6 No component logs message content — confirm across the manager, the
  channel adapters and the runtime that a message body never reaches a log line.
  The spec requires the page to state it and D7 requires every public claim to be
  checked; this was the one claim with no check behind it
- [x] 1.7 Supply chain — confirm `provenance: mode=max` and `sbom: true` in
  `.github/workflows/build-image.yml`, that no `actions/attest-*` step exists
  anywhere, and that `build-chart.yml` grants no `attestations:` permission
- [x] 1.8 The three walls' defaults — confirm in `chart/values.yaml` that
  `networkPolicy.enabled` and `allowPodExecution` are false, that
  `egressMediation.enabled` is **true**, and that no preset posture exists:
  `rbacMode` is DELETED at every level and the chart FAILS the render on it. The
  page states the posture, not the keys, but a wrong posture claim is the worst
  failure available here — and "they all default off" is now wrong in both
  directions
- [x] 1.9 Record each result as a verdict in this task list. A findings list that
  quotes what it matched is a local artifact and is never committed

### Verdicts

Recorded per `publication.md`: the verdict, never the matched value.

| Claim | Verdict |
|---|---|
| 1.1 manager holds no `secrets` verb | **CONFIRMED** — `chart/templates/rbac.yaml` names the resource nowhere; the manager's non-test code performs no Secret API read |
| 1.2 adapter tokens derived, not stored | **CONFIRMED** — HMAC-SHA256 over two distinct contexts, validated by re-derivation against the CRD list at four `httpapi` call sites |
| 1.3 runtime pods non-root, workspace isolated | **CONFIRMED, with a limit** — the pod runs at a fixed non-root uid/gid/fsGroup and the workspace claim mounts with the conversation name as `SubPath`. `RunAsNonRoot`, drop-`ALL` and no-privilege-escalation are asserted ON THE CONTAINER only under egress mediation — on by default, so the default install has them, but they are not unconditional |
| 1.4 context isolation, per mode | **CONFIRMED, both halves** — the gate is exactly the three conditions; under it the agent's context volume is an `EmptyDir` and the durable claim is a separate volume mounted only by the sidecar, per conversation. Without it the claim mounts whole at the live path into the agent container, with NO subPath |
| 1.5 the default install IS the isolating mode | **CONFIRMED against a render** — `helm template` of the default chart yields `AgentRuntime/default` declaring context paths, the manager's sidecar image and context claim env, and the claim itself |
| 1.6 no component logs message content | **REFUTED** — see below |
| 1.7 supply chain | **CONFIRMED** — provenance and SBOM are set on the image build; zero `actions/attest-*` steps anywhere; the chart workflow grants no `attestations:` permission; no signing tooling in the tree |
| 1.8 the three walls' defaults | **CONFIRMED, and not as the task originally read** — network policy off, pod execution off, **egress mediation ON**. No preset posture exists: both `rbacMode` forms FAIL the render, verified by two renders |

**1.6 is refuted and the spec depends on it.** The manager, both Telegram
components, the signal adapters and the console log identifiers, counts, op ids
and errors only. **The reference runtime does not**: `formatEvent` in
`runtimes/claude/runtime.js` writes the agent's assistant text verbatim, its
tool-call arguments truncated, and the whole result block to the pod's stdout —
so conversation content is in the runtime pod's log, readable by anyone holding
`pods/log` in the operator's namespace.

D7 is what caught it: the claim was in the spec and in no verification list until
this change's `/opsx:update` pass added one.

## 2. Draft the page

`docs/security.md`, permalink `/security/`. **Shaped as a threat model**, in a
security reviewer's vocabulary, illustrated throughout.

**The first draft was rejected on review and is recorded in D8**: it satisfied
every requirement and was still a wall of prose with no picture, calling the
three controls "walls". What follows is what shipped.

- [x] 2.1 Front matter: title, permalink, description, and a `next` card pointing
  at Installation
- [x] 2.2 **The threat model drawing** — `docs/diagrams/threat-model.py`, both
  themes, trust boundaries dashed, six numbered crossings. Landscape at the
  frame's width. **Two compositions were built and rejected first** — see D8 for
  the measurements
- [x] 2.3 **The register** — one row per crossing, keyed to the drawing by
  number, naming the threat and the control. A lead-in sentence sits between the
  drawing and the table, because the theme gives a diagram no bottom margin and
  a table butts against it
- [x] 2.4 **Defence in depth**, the three controls named as a reviewer names
  them — network segmentation, egress control, cluster authorization — each in
  the SAME four parts: threat, control, cost, residual risk
- [x] 2.5 **An illustration at each hard claim**: the unauthenticated surfaces,
  the allowlist an agent with a shell walks around, the Secret reached through
  the kubelet, and the context mount that is absent by design. Specs in
  `docs_diagrams.py` with a new `dir` key, so they land outside
  `assets/img/guides/`
- [x] 2.6 **Secret exposure** as a subsection of authorization — no `secrets`
  verb is not the same as cannot read Secrets, because the kubelet resolves a
  Secret when it builds a pod
- [x] 2.7 **The platform's own posture** — 1.1 through 1.7, including the
  three-line supply-chain answer. Context isolation stated per mode, the default
  install's mode first, with the three conditions that mode needs. Logging stated
  PER COMPONENT — never as "no component logs message content", which 1.6
  refuted
- [x] 2.8 **Residual risk** — the unauthenticated surfaces, the control whose
  enforcement the chart cannot verify, the isolation a runtime that declares no
  context paths does not get, conversation content in the runtime pod's log,
  ungoverned tool arguments, and the two supply-chain gaps. **Crossing 6 is on
  the drawing too**, marked distinctly, because a threat model showing only the
  mitigated crossings is D6's failure in picture form
- [x] 2.9 **Reporting** — three lines linking `SECURITY.md`. No reporting process
  restated
- [x] 2.10 Verify the page carries no values table, no default value and no YAML
  setting a control. This is the observable form of D2, and it is what stops the
  page becoming a second source of truth
- [x] 2.11 Delete any generated diagram the page does not reference. The first
  top-of-page drawing (`layers`) was replaced by the threat model, and a spec
  left behind means CI regenerates an asset nothing uses

## 3. Publish it into the site

D3: these land together or the *Start here* chain is inconsistent.

- [x] 3.1 One entry in `docs/_data/nav.yml`, in *Start here*, between the Console
  page and Installation
- [x] 3.2 Repoint `docs/console-guide.md`'s `next` card at `/security/`, replacing
  its current Installation target
- [x] 3.3 Verify the chain by following only the what-next cards from the landing
  page: every *Start here* entry is visited, in navigation order, with none
  skipped
- [x] 3.4 Confirm `docs/adr/0001-bound-component-reach.md` is linked where it
  lives, gained no front matter and no nav entry, and was not edited

## 4. The two entry points

- [x] 4.1 `docs/index.md` — one line naming the page. **Placed in *Where to
  start*, not beside the areas table**: that list enumerates the whole *Start
  here* group, so leaving Security out of it would skip an entry there too. Same
  one line, the stronger position
- [x] 4.2 `README.md` — one line in the links-onward index. Verify `wc -l` is
  still at most 215; it is at 203 and this adds no section
- [x] 4.3 `docs/installation.md` — links out to `/security/` from *The agent's
  power*, *Who may reach what* and *Enforcing the toolset*. **Links only.**
  Verify by diff that no prose was removed from it

## 5. Close out

- [x] 5.1 Build the site and confirm the page renders with the assigned layout,
  the on-this-page column, and correct light and dark rendering
- [x] 5.2 Every link on the page resolves — the `installation.md` anchors, the
  ADR, `SECURITY.md`, `console-guide.md`
- [x] 5.3 `python3 .github/scripts/publication-guard.py` and
  `python3 .github/scripts/retired-vocabulary-guard.py` both pass. A security
  page names surfaces and components, which is exactly where a real hostname
  reaches the tree
- [x] 5.4 `openspec validate publish-security-page --strict`

## 6. Documentation

Both halves, listed separately because they are skipped independently. This
change's deliverable IS a documentation page, which makes it easy to believe the
documentation is done when the two documents that ROUTE future writers are not.

### 6.1 The reference docs

- [x] 6.1.1 `docs/CLAUDE.md` — the site's-pages table gains a row for
  `docs/security.md` stating what it owns: the threat-indexed posture, not the
  values
- [x] 6.1.2 `.claude/rules/documentation.md` — the routing table gains a row, in
  the shape it already uses for the console pair, so the next writer does not
  choose between the security page and `installation.md` at random. **It also
  names `threat-model.py`**, because CI does not run it and a moved boundary
  would otherwise ship a stale drawing
- [x] 6.1.5 `docs/.claude/site.md` — its `diagrams/` section said "the drawio
  source plus `export.py`" and there are now THREE generators. It gains the
  table, which of them CI checks, the frame-width rule and the portrait failure
- [x] 6.1.3 No CRD field, chart value or api doc comment changed, so
  `docs-generate.py` is NOT required. Confirm that rather than assume it — a
  generated block stale for another reason still fails CI
- [x] 6.1.4 `docs/CHANGELOG.md` gets no entry: no behaviour changed and there is
  nothing to upgrade. Confirmed deliberately, not skipped

### 6.2 The adopter site

- [x] 6.2.1 Re-read `docs/getting-started.md` and `docs/introduction.md` against
  the finished page. Edit only if either now says something the security page
  contradicts. **Neither does, so neither was edited** — both already state that
  an agent carries no permissions of its own and that a route runs as an account
  bound to nothing
- [x] 6.2.2 Confirm the finished page matches what section 2 planned. Written
  last, it documents what the change ACTUALLY did — if a wall or a gap moved
  during drafting, the spec is updated to match, not the memory of it.
  **It caught one.** The supply-chain answer was drafted with its own `###`
  heading, which the spec forbids while the answer is partial. The heading was
  removed and the three lines now sit directly under *What agent-ops itself
  holds*, per D5 — the page was brought to the spec, not the spec to the page
- [x] 6.2.3 Confirm the not-addressed section survived drafting as a SECTION, not
  a callout beside a control. D6 predicts this is the first thing to erode, and
  it eroded before anyone reviewed it or it did not. **It did not** — *What is
  not addressed* is a top-level section with a seven-row table
