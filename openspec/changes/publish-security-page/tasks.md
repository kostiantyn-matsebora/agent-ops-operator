## 1. Verify every claim before writing it

D7 in `design.md`: each of these becomes a public statement about a security
property. Verify against the tree, not against this change's own research. Record
the VERDICT, never a matched value.

- [ ] 1.1 Manager holds no `secrets` verb — confirm no `secrets` resource appears
  in the manager's ClusterRole in `chart/templates/rbac.yaml`
- [ ] 1.2 Adapter tokens are derived, not stored — confirm
  `DeriveAdapterToken` / `DeriveSignalAdapterToken` in
  `platform/manager/internal/chat/token.go` and that the manager reads no Secret
  to validate one
- [ ] 1.3 Runtime pods are non-root with per-conversation workspace isolation —
  confirm the pod `SecurityContext` and the workspace `SubPath` in
  `platform/manager/internal/runtimepod/podspec.go`
- [ ] 1.4 Context isolation, per mode — confirm that under `contextSync` the
  agent container mounts only an `EmptyDir` and the durable claim is mounted
  solely by the sidecar, and that in the default mode the claim is mounted whole
  into the agent container. **Both halves**, since the page states both
- [ ] 1.5 Supply chain — confirm `provenance: mode=max` and `sbom: true` in
  `.github/workflows/build-image.yml`, that no `actions/attest-*` step exists
  anywhere, and that `build-chart.yml` grants no `attestations:` permission
- [ ] 1.6 The three walls' defaults — confirm `networkPolicy.enabled`,
  `egressMediation.enabled`, `rbacMode` and `allowPodExecution` all default off
  or empty in `chart/values.yaml`. The page states the posture, not the keys, but
  a wrong posture claim is the worst failure available here
- [ ] 1.7 Record each result as a verdict in this task list. A findings list that
  quotes what it matched is a local artifact and is never committed

## 2. Draft the page

`docs/security.md`, permalink `/security/`. Section order is the spec's, and the
default posture comes before any individual control.

- [ ] 2.1 Front matter: title, permalink, description, and a `next` card pointing
  at Installation
- [ ] 2.2 **What you are trusting** — the frame: an agent runs model output in
  your cluster, and three independent controls decide what that reaches
- [ ] 2.3 **The default install grants nothing** — the floor identity bound to
  nothing, the runtime RBAC mode off, pod execution off. Before any control is
  described, per the spec
- [ ] 2.4 **Wall 1, who may connect** — the threat, the control, and the limit:
  network restriction applies cleanly on a cluster that does not enforce it and
  protects nothing. Link the key in `installation.md`
- [ ] 2.5 **Wall 2, what a connected agent may do** — why an allowlist configures
  a cooperating agent, what enforcing it outside the agent costs, and that stdio
  and `https` MCP endpoints are not covered. Decision in brief, ADR linked
- [ ] 2.6 **Wall 3, what it may do to the cluster** — the runtime's RBAC, and the
  Secrets boundary: no `secrets` verb is not the same as cannot read Secrets,
  because the kubelet resolves a Secret when it builds a pod
- [ ] 2.7 **What agent-ops itself holds** — 1.1 through 1.5, including the
  three-line supply-chain answer. Context isolation stated per mode with the
  default stated first
- [ ] 2.8 **What is not addressed** — the unauthenticated surfaces, the control
  whose enforcement the chart cannot verify, the isolation that is opt-in,
  ungoverned tool arguments, and the two supply-chain gaps
- [ ] 2.9 **Reporting** — three lines linking `SECURITY.md`. No reporting process
  restated
- [ ] 2.10 Verify the page carries no values table, no default value and no YAML
  setting a control. This is the observable form of D2, and it is what stops the
  page becoming a second source of truth

## 3. Publish it into the site

D3: these land together or the *Start here* chain is inconsistent.

- [ ] 3.1 One entry in `docs/_data/nav.yml`, in *Start here*, between the Console
  page and Installation
- [ ] 3.2 Repoint `docs/console-guide.md`'s `next` card at `/security/`, replacing
  its current Installation target
- [ ] 3.3 Verify the chain by following only the what-next cards from the landing
  page: every *Start here* entry is visited, in navigation order, with none
  skipped
- [ ] 3.4 Confirm `docs/adr/0001-bound-component-reach.md` is linked where it
  lives, gained no front matter and no nav entry, and was not edited

## 4. The two entry points

- [ ] 4.1 `docs/index.md` — one line naming the page, in or beside the areas
  table. Its job is to prove the question was considered and route the reader
- [ ] 4.2 `README.md` — one line in the links-onward index. Verify `wc -l` is
  still at most 215; it is at 203 and this adds no section
- [ ] 4.3 `docs/installation.md` — links out to `/security/` from *The agent's
  power*, *Who may reach what* and *Enforcing the toolset*. **Links only.**
  Verify by diff that no prose was removed from it

## 5. Close out

- [ ] 5.1 Build the site and confirm the page renders with the assigned layout,
  the on-this-page column, and correct light and dark rendering
- [ ] 5.2 Every link on the page resolves — the `installation.md` anchors, the
  ADR, `SECURITY.md`, `console-guide.md`
- [ ] 5.3 `python3 .github/scripts/publication-guard.py` and
  `python3 .github/scripts/retired-vocabulary-guard.py` both pass. A security
  page names surfaces and components, which is exactly where a real hostname
  reaches the tree
- [ ] 5.4 `openspec validate publish-security-page --strict`

## 6. Documentation

Both halves, listed separately because they are skipped independently. This
change's deliverable IS a documentation page, which makes it easy to believe the
documentation is done when the two documents that ROUTE future writers are not.

### 6.1 The reference docs

- [ ] 6.1.1 `docs/CLAUDE.md` — the site's-pages table gains a row for
  `docs/security.md` stating what it owns: the threat-indexed posture, not the
  values
- [ ] 6.1.2 `.claude/rules/documentation.md` — the routing table gains a row, in
  the shape it already uses for the console pair, so the next writer does not
  choose between the security page and `installation.md` at random
- [ ] 6.1.3 No CRD field, chart value or api doc comment changed, so
  `docs-generate.py` is NOT required. Confirm that rather than assume it — a
  generated block stale for another reason still fails CI
- [ ] 6.1.4 `docs/CHANGELOG.md` gets no entry: no behaviour changed and there is
  nothing to upgrade. Confirmed deliberately, not skipped

### 6.2 The adopter site

- [ ] 6.2.1 Re-read `docs/getting-started.md` and `docs/introduction.md` against
  the finished page. Edit only if either now says something the security page
  contradicts
- [ ] 6.2.2 Confirm the finished page matches what section 2 planned. Written
  last, it documents what the change ACTUALLY did — if a wall or a gap moved
  during drafting, the spec is updated to match, not the memory of it
- [ ] 6.2.3 Confirm the not-addressed section survived drafting as a SECTION, not
  a callout beside a control. D6 predicts this is the first thing to erode, and
  it eroded before anyone reviewed it or it did not
