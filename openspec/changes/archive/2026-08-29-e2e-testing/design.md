## Context

The repository tests two layers well and a third not at all. Per-module unit
tests cover rendering, parsing and scheduling;
`platform/manager/internal/integration/` runs 22 files against a real API server. Neither reaches the substrate, because envtest
starts an API server and nothing else — no kubelet, no scheduler, no CSI, no
authorizer decisions exercised by real subjects. The suite's own comments record
the consequence: pods created by earlier tests remain Pending forever, so
capacity tests need their own reconciler with an explicit cap.

The change is also constrained by rules that are not negotiable here: every Go
module but `platform/manager/` holds no dependency outside its own directory
(`.github/components.sh modules` is the list); the manager reading no Secrets; exactly one `getUpdates` consumer per
bot token. A test harness that violates any of them to make testing easier would
be testing a system that is not the one that ships.

Two external facts shape the plan. `.github/` carries `ci.yml`, `release.yml`
and `build-image.yml`, and branch protection names one check, `ci-green` — so an
e2e gate is a line in that job's `needs:`. And `.github/components.sh` discovers
components from every Dockerfile and `go.mod` in the tree, so a test image
placed carelessly becomes a published component.

## Goals / Non-Goals

**Goals:**
- Assert the substrate facts that are currently unasserted, each justified by
  naming the component whose participation makes the assertion possible.
- Reproduce the signal-loop failure mode and prove it stays bounded.
- Make the Telegram lane runnable with no bot token and no Telegram account.
- Verify adapter conformance black-box without adding a dependency to any
  adapter module.
- Keep the pull-request gate deterministic, secret-free and time-bounded.

**Non-Goals:**
- Scoring agent answer quality. That is an eval harness — a different project
  with a different cost model and a different notion of pass.
- Browser automation of the console. Its HTTP API is the human path minus the
  renderer, and the renderer is not where these invariants live.
- Deploying any third-party system under test.
- Load, soak or multi-node behavior. One node cannot show a scheduling spread,
  and pretending otherwise would be worse than not claiming it.

## Decisions

### Cluster: k3d (k3s in Docker), with the bundled add-ons disabled

k3s was the stated preference and is the right one — it is a real distribution
with a real kubelet, a real authorizer and a real local-path provisioner, which
is precisely the set of components the pack exists to involve. k3d runs it in
Docker on a GitHub-hosted runner in roughly 30 seconds.

Traefik and servicelb are disabled at cluster creation. They add startup time
and a LoadBalancer implementation the chart does not assume, and a Service that
behaves differently under test than in a typical install is a source of false
confidence rather than coverage.

*Alternatives considered:* **kind** — equally viable and slightly more common in
the ecosystem, but k3s was asked for and its single-binary startup is faster;
nothing in the pack depends on kubeadm specifics. **A persistent cluster** —
rejected, because state carried between runs is how a suite starts passing for
reasons nobody can reconstruct.

### Images: import for speed, but pull for real through a local authenticated registry

Building images and importing them into the node with `k3d image import` is the
fast path and is used for everything. It has one consequence that must be
handled deliberately: **an imported image is never pulled**, so a pack that only
imports cannot assert that pulls work, and the `imagePullSecrets` requirement
would be vacuously satisfied.

So one dedicated test — and only one — runs against a local registry configured
with authentication, with the chart pointed at it and a pull secret supplied.
That test is the pull path; everything else uses imports.

*Alternative considered:* pulling everything from the local registry, which is
more uniform but pays registry round-trips on every image in every run for
coverage a single test already provides.

### The e2e pack is Go inside `platform/manager/`, behind a build tag

`platform/manager/test/e2e/` lives in the manager module, which already depends
on client-go and controller-runtime, so the pack costs no new dependency anywhere and can use the
project's own API types for assertions rather than unstructured YAML. A build
tag keeps it out of `go test ./...`.

*Alternatives considered:* **a shell/bats harness** — rejected because
assertions on CR status and `SubjectAccessReview` are far clearer with typed
clients, and because the existing integration suite is Go and reviewers should
not switch languages to read the next layer. **Its own Go module** — rejected;
it would be a new module for no isolation benefit, since the manager module's
dependencies are exactly the ones it wants, and `components.sh` would discover
its `go.mod` as a component.

### Conformance drives built binaries, not an imported package

Every adapter module is forbidden a dependency outside its own directory, so the
shared-test-package form is unavailable: importing it would add a `go.mod` entry
to all seven. The suite therefore builds each adapter and speaks HTTP to the
process, with a fake manager on the other side.

This is not a workaround. Adapters are out-of-process by definition — the
contract is the entire interface — so a test that speaks the contract to a
running binary *is* a conforming peer, not a simulation of one. It also verifies
the artifact that ships rather than a library inside it.

*Alternative considered:* duplicating the harness into each module. Rejected:
seven copies of a contract test drift, and the first drift is silent.

### Assertions on the agent are closed-form, not judged

The real runtime is the primary oracle, which raises the question of how to
assert on generated text without inheriting its variance. The answer is to ask
questions whose correct answer is checkable by string containment:

- **Continuity** is asserted with a nonce. The pack states a random identifier in
  one turn and asks for it back in a later one; the assertion is that the exact
  token appears. It cannot be satisfied by a plausible-sounding answer, which is
  the failure mode that makes prose assertions untrustworthy.
- **Toolset enforcement** is asserted on **effect, not on text**: the pack asks
  for a mutation, then reads the cluster to confirm it did not happen. What the
  agent said about refusing is not evidence; the object's unchanged state is.
- **Correctness** questions are closed-form ("reply with the name of the pod in
  namespace X") so the expected answer is known to the test.

*Alternative considered:* an LLM judge scoring free-form answers. Rejected here:
it puts a second nondeterministic component into the oracle, so a red result no
longer distinguishes "the system broke" from "the judge disagreed today" — and
that ambiguity is exactly what makes people stop reading failures. A judge is
the right tool for evaluating quality, which is a non-goal.

Every agent-dependent assertion carries a bounded retry, and the retry count is
reported, so a lane that is technically passing while retrying constantly is
visible rather than quietly degrading.

### The console is the channel for lifecycle coverage; Telegram gets a narrow lane

The console is a conforming `ChannelAdapter` with no third-party dependency, so
conversation lifecycle coverage runs through it with nothing faked, driven
through `POST /api/conversations` and `POST /api/conversations/{name}/messages`.

The Telegram lane then only has to prove the Telegram-specific half — the
router's classification, verbatim forwarding, and the adapter's rendering and
Bot API call shape. The fake Bot API runs **in-cluster** as an ordinary
Deployment, because the components under test resolve it by cluster DNS; running
it on the runner would require exposing the runner to the cluster, which is a
second networking mechanism to maintain for no gain.

### `TELEGRAM_API_BASE` is deployment-level, never `Channel.spec.config`

Making the endpoint per-surface would be more flexible and is wrong: anyone able
to edit a served channel's config could redirect that channel's bot token to a
host they control. Deployment-level env keeps the blast radius at "can edit the
Deployment", which is already total access.

The same reasoning generalizes into the standing rule the specs record — an
adapter's outbound base URL is configuration, never a constant — which costs
nothing at any other adapter, since a self-hosted upstream has no fixed address.

### Fixtures are canonical in `test/fixtures/`, read by relative path in module tests

A module test that wants a canonical payload reads it with a relative path in a
`_test.go` file. That adds no `go.mod` entry and no build-time dependency, so
the self-contained-module rule holds in the sense that matters — the shipped
binary depends on nothing outside its directory.

*Alternative considered:* per-module copies with a CI byte-match check.
Equivalent in outcome, more machinery. If the module rule is later read
strictly enough to forbid the relative read, this is the fallback.

### The stub and the fake live under `test/`, and `components.sh` excludes it

Both need a Dockerfile to run in-cluster, and `components.sh` unions every
Dockerfile-bearing directory into the component list. Without an explicit
`-not -path './test/*'` the next release tag publishes `agentops-stubruntime`
and the review matrix grows by two — which is exactly the tell
`worktree-delivery.md` warns reads as a new component. The exclusion is one
line; a test asserts `components.sh images` never lists either.

### Egress mediation stays on

The runtime pod's `NET_ADMIN` init container is on by default. k3d admits it,
so the pack runs the default posture and does not disable mediation to make the
cluster simpler — a pack that turns off the wall tests a pod that does not ship.

### Failure diagnostics are collected unconditionally

On any failure the pack dumps manager logs, adapter logs, the pod list, cluster
events and the full YAML of every agentops CR, as a workflow artifact. A remote
cluster that no longer exists is unreproducible, so a failure that did not
capture its own context costs a full re-run to learn anything.

## Risks / Trade-offs

- **Agent lanes go flaky and erode trust in the whole suite** → closed-form and
  effect-based assertions rather than judged prose; bounded retries with the
  retry count reported; the agent lane never gates a pull request, so its
  flakiness cannot block work while it is being tuned.
- **Token spend grows silently** → the real-runtime lane is dispatch- and
  schedule-only, its conversation count is fixed and small, and the pack fails
  fast rather than retrying a broken deployment through many turns.
- **The pull-request tier grows until nobody waits for it** → an explicit
  wall-clock budget for the gating tier; work that exceeds it moves to the
  nightly pack rather than slowing the gate.
- **The loop-breaker test asserts an absence, which is weak** → assert both a
  bounded Conversation count and a bounded creation *rate* over the window, so a
  slow leak fails rather than passing under a generous ceiling; and run the
  cold-cache variant, since the prefix mechanism is the one that holds when the
  others cannot.
- **k3s differs from the target distribution** → the pack claims substrate
  facts (kubelet resolution, authorizer decisions, PVC binding, informers) that
  are conformance-level, not distribution-specific; it does not claim
  multi-node or CNI-specific behavior, and should not be extended to.
- **A test image is published as a component** → `test/` is excluded in
  `components.sh` and the exclusion is asserted, so the release list and the
  review matrix are unchanged.
- **The `TELEGRAM_API_BASE` change touches shipping code for a test's benefit**
  → it defaults to the current constant and renders nothing when unset, so a
  default install is byte-identical; and it is a prerequisite `signal-ha` needs
  regardless, since Home Assistant has no fixed address.

## Migration Plan

Nothing ships to users, so there is no upgrade path and no `CHANGELOG.md`
entry. Sequencing is by dependency, not by risk:

1. **Conformance suite + fake Bot API + `TELEGRAM_API_BASE`.** Plain `go test`,
   no cluster, no workflow. Runs locally in Docker today.
2. **The e2e pack against the stub runtime.** Needs a cluster but no secret.
3. **The real-runtime lane.** Needs the repository secret and a decision on
   cadence.
4. **Workflows.** `e2e.yml` wires 1–3 into the two tiers, reporting the
   gating tier through `ci-green`.

Rollback is deletion: no production code path depends on any of it, and the one
shipping-code change is inert when its environment variable is unset.

## Open Questions

- **Cadence and spend for the real-runtime lane** — nightly, weekly, or
  dispatch-only to start? Dispatch-only is the safe default and can be
  tightened up rather than walked back.
- **Model pinning for cost** — should the e2e `AgentProfile` pin a smaller
  model? It reduces spend but means the nightly no longer exercises the model
  the product defaults to, which is part of what the lane is for.
- **Where the local-registry pull test gets its credentials** — a throwaway
  htpasswd generated per run is simplest; confirm that is acceptable rather
  than reusing the GHCR token.
- **Whether `signal-ha` adopts the outbound-base-URL rule as a spec requirement
  when it is written**, or inherits it as an `invariants.md` rule only.
