---
title: Testing
permalink: /testing/
description: The three tiers a change is verified at, what each can and cannot decide, and how to run each one yourself.
---

# Testing

agent-ops is verified at three tiers, and each exists for what the one below
it structurally cannot decide.

| Tier | Runs | Decides | Cannot decide |
|---|---|---|---|
| **Unit and envtest** | every module's `go test`, and the operator's suite against a real API server | rendering, parsing, scheduling, and what a reconciler writes to a CR | anything the kubelet, the scheduler, a CSI driver or a live authorizer decides — envtest runs none of them |
| **Contract conformance** | every adapter's **built binary**, black-box, against a fake manager | that an adapter speaks the adapter contracts — long-poll, `contract=`, ack-once, inbound push, listing, status, no relay loop for a channel adapter, and normalized emission, bearer auth and a surfaced rejected post for a signal adapter | anything about the cluster |
| **End to end** | the chart from the working tree on a real single-node cluster (k3s under k3d), images built from the same commit | the substrate: credential projection by the kubelet, RBAC as the authorizer enforces it, informer liveness, context continuity across a pod restart, admission FIFO on a real pod DELETE, the signal loop breaker under a runtime that really cannot start | answer *quality* — that is an eval harness, a different project |

## What gates a pull request

**Contract conformance and a thin cluster smoke on the stub runtime.** Both are
deterministic, need no secret and are bounded in wall clock, so they run for
every pull request — from a fork too — and report through `ci-green`, the one
required check. The full pack, including the lane that drives the real agent
runtime with a real credential, runs nightly when master moved and on demand,
and never gates a pull request: its lane spends real tokens, and its flakiness must not block work
while it is tuned.

A fork pull request runs every tier its secret access allows and is not failed
for the rest. Secrets being unavailable to forks is the intended access
boundary, not a limitation to work around.

## The oracle split

**The real agent runtime is the primary oracle.** The questions worth asking
end to end are ones only an agent can answer: did context survive a gap, is the
answer right, does the bound toolset actually constrain it. Assertions are
closed-form rather than judged:

- a nonce stated in one turn and asked back after a pod restart
- a pod name the test already knows
- a mutation asked for outside the bound toolset, asserted on the OBJECT
  staying unchanged rather than on whatever the agent said about refusing

Every agent-dependent assertion carries
a bounded retry whose count is reported, so a lane that passes while retrying
constantly is visible rather than quietly degrading.

**A stub runtime exists for what no agent exhibits on cue** — a handle that
names nothing, a crash that reports nothing, a stall past the idle TTL, a
storage outage. These are manager mechanisms, and the stub is an instrument for
them, not a cost workaround: a test that could have been written against the
real runtime is. Its behaviour is scripted by the first word of the input
(`echo`, `fail`, `stale-context`, `no-context`, `die`, `stall`,
`storage-outage`), identical input giving an identical report. It is built by
the pack only, published by no release, and referenced by no chart default.

## No third party is deployed

Every inbound adapter is driven by a captured, scrubbed payload from
`test/fixtures/` — an Alertmanager webhook body POSTed to the adapter's
`/webhook/{source}`, a Telegram `Update` fed to a fake Bot API. The fake is
faithful because `gateway-telegram` forwards updates verbatim: what it replays
is byte-identical to what Telegram would have produced. The same fixtures are
the owning modules' unit-test inputs, so one captured payload cannot drift
between the two suites.

The console is the end-to-end channel: a conforming `ChannelAdapter` with no
third-party dependency, driven through its own HTTP API — the path a person
takes minus the browser — so origination, dispatch, delivery and close run
through production code with nothing simulated.

## Run it yourself

Conformance needs a Go toolchain and nothing else:

```sh
cd platform/manager && go test -tags conformance -count=1 -v ./test/conformance/
```

The pack needs Docker, `k3d`, `kubectl` and `helm` on the PATH:

```sh
cd platform/manager && go test -tags e2e -count=1 -timeout 45m -v ./test/e2e/
```

| Variable | Does |
|---|---|
| `E2E_TIER` | `pr` (default) or `full` — `full` adds the real-runtime lane and the slow lanes |
| `E2E_REUSE=1` | keep an existing cluster and leave it running afterwards — the loop for local iteration |
| `E2E_SKIP_BUILD=1` | with `E2E_REUSE`, images are already built and imported |
| `E2E_ARTIFACT_DIR` | where failure diagnostics land — manager and adapter logs, the pod list, cluster events, every agentops CR as YAML |
| `E2E_BUDGET` | wall-clock budget of the gating tier (default `20m`) — exceeding it fails the run, so growth is visible rather than gradual |
| `CLAUDE_CODE_OAUTH_TOKEN` | the real-runtime lane's credential — absent, that lane reports itself skipped |

On any failure the diagnostics are written unconditionally: a cluster that no
longer exists is unreproducible, and a failure that did not capture its own
context costs a full re-run to learn anything.

## What the repository must hold

Four workflows share one definition, `.github/workflows/e2e.yml`:

| Workflow | Runs | When |
|---|---|---|
| `ci.yml` | the pr tier | every pull request, reporting through `ci-green` |
| `release.yml` | the pr tier | on the tagged commit, before anything is published |
| `e2e-smoke.yml` | the pr tier | on demand, on any branch |
| `e2e-full.yml` | the full tier, real-runtime lane included | nightly at 03:17 UTC when master moved since its last successful run, and on demand |

```sh
gh workflow run e2e-smoke.yml --ref my-branch
gh workflow run e2e-full.yml
```

One thing exists already and one is a decision:

1. **The repository secret `CLAUDE_CODE_OAUTH_TOKEN`** — the same Claude Code
   token the review workflow already runs on, and the credential the claude
   bundle projects into the runtime. Read only by `e2e-full.yml`; pull
   requests never see it. Without it the real-runtime lane reports itself
   skipped and every other lane still runs.
2. **The cadence.** The cron lives in `e2e-full.yml` and nowhere else. A
   night on which master did not move is skipped, so the spend tracks the
   change rate rather than the calendar.
