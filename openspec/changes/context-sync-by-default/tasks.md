## 1. The fallback, and its test first

D2 in `design.md` makes this ordering a correctness constraint, not a
preference: the defect is rare today and universal once the default flips.

- [ ] 1.1 Add a failing test to `platform/manager/internal/runtimepod/contextsync_test.go`
  constructing a config with `ContextSyncImage` set, a runtime declaring
  `contextSync.paths`, and `ContextPVC: ""`. Assert no volume in the built pod
  references a `PersistentVolumeClaim` by an empty name. Verify it FAILS against
  the current predicate — a test that passes before the fix is testing nothing
- [ ] 1.2 Add `&& cfg.ContextPVC != ""` to the `sidecar` predicate at
  `platform/manager/internal/runtimepod/podspec.go:287`, and verify 1.1 now passes
- [ ] 1.3 Assert the fallback is TODAY'S pod, not merely a pod without the store:
  extend 1.1 to check the agent container mounts `context` at `/data/context`
  from an `EmptyDir`, that no `context-sync` container is present, and that
  `CONTROL_URL` points at the manager rather than at `127.0.0.1`
- [ ] 1.4 Pin the agreement `design.md` D1 names: a test asserting that whenever
  `ContinuityPossible()` is false for a resolved runtime, `Build` produces a pod
  that references no persistent context claim. This is the invariant the defect
  broke, and it is the one worth holding
- [ ] 1.5 Confirm the missing-image and missing-claim fallbacks are the SAME rule
  — `TestContextSyncWithoutAnImageFallsBackSafely` and the new test should assert
  the same pod shape. If they diverge, one of them is describing an exception
- [ ] 1.6 Note the shared config at `contextsync_test.go:17` sets
  `ContextPVC: "agentops-context"` for every case, which is why this was never
  constructed. Leave it as the default and override per-test, so existing cases
  are untouched
- [ ] 1.7 Run the full manager suite with `KUBEBUILDER_ASSETS` and verify no
  existing context-sync test regressed

## 2. The default

Only after section 1 is green. See `design.md` D3 and D4.

- [ ] 2.1 In `chart/values.yaml`, set `runtime.contextSync.paths` to
  `[".claude/projects/-data-workspace/**"]` — the value currently commented out
  directly beneath it — and verify `helm template` now renders a `contextSync`
  stanza on the AgentRuntime with default values
- [ ] 2.2 Replace the comment explaining why the list was empty with one stating
  what declaring it means and that a different backend must replace it. The
  reasoning about only-the-runtime-knowing is KEPT — it is why a third-party
  runtime still declares its own — not deleted along with the empty default
- [ ] 2.3 Verify `chart/templates/runtime.yaml` needs no edit: it renders from
  `{{- with $rt.contextSync.paths }}` already. If it needed one, D3 is wrong and
  the design is revisited rather than the template patched
- [ ] 2.4 Verify the opposite direction still works: `runtime.contextSync.paths: []`
  renders no stanza, and the pod is the direct-mount pod
  (`TestClearingContextSyncRestoresTheDirectMount` covers the manager half)
- [ ] 2.5 Render with `persistence.context` disabled AND the new default paths
  set, and confirm the manifest is valid — this is the combination section 1
  fixed, now reachable by default
- [ ] 2.6 Run the chart render tests

## 3. Verify it runs, not that it renders

A rendered pod is not a running one. `gotchas.md` records what this repo paid to
learn that, and `2026-08-21-context-survivability` smoked the sidecar for the
same reason.

- [ ] 3.1 Install with default values against a cluster with a context volume,
  start a conversation, and confirm a `context-sync` container is present and the
  agent container holds NO mount of the durable claim
- [ ] 3.2 Confirm a checkpoint actually lands: inspect the durable volume for a
  generation under the conversation's subPath, and confirm `current` resolves
- [ ] 3.3 Confirm continuity across pods — let the pod idle out, send a second
  message, and confirm the answer continues rather than starting fresh
- [ ] 3.4 Install with persistence disabled, start a conversation, and confirm
  the pod STARTS, runs with ephemeral context, and the reply says the context is
  not promised. This is the fallback path end to end
- [ ] 3.5 Confirm the reset verb recovers a conversation whose handle predates
  the layout move, since that is the recovery the changelog will name

## 4. Close out

- [ ] 4.1 Every module builds, vets and tests — the loop in `.claude/rules/build-test.md`
- [ ] 4.2 `python3 .github/scripts/publication-guard.py` and
  `python3 .github/scripts/retired-vocabulary-guard.py` both pass. Record the
  VERDICT only, never a matched value
- [ ] 4.3 `openspec validate context-sync-by-default --strict`

## 5. Documentation

Both halves, listed separately because they are skipped independently: a
behaviour change feels done once `concepts.md` is right, and the adopter never
reads `concepts.md`.

### 5.1 The reference docs

- [ ] 5.1.1 `docs/CHANGELOG.md` — newest first. State that context
  synchronisation is now on by default where a context volume exists, that the
  durable layout moves from the claim root to a per-conversation path, that a
  conversation holding a context handle FAILS its next run rather than answering
  without memory, and name the reset verb as the recovery. State that no
  migration is provided and why
- [ ] 5.1.2 `docs/concepts.md` — `AgentRuntime.spec.contextSync` is no longer
  "absent by default". Document the no-volume fallback as part of how continuity
  resolves, beside `contextStorage`
- [ ] 5.1.3 `docs/installation.md` — the storage section. What a reader turning
  persistence off now gets, and the ephemeral-storage consequence of the default
  (`liveSizeLimit`, and that `$HOME` including caches is now pod-local on every
  conversation)
- [ ] 5.1.4 Re-run `python3 .github/scripts/docs-generate.py` — a chart value's
  default changed, so `docs/cr-reference.md` and every generated block are stale.
  CI fails on a difference
- [ ] 5.1.5 `.claude/rules/terminology.md` — the `context-sync` entry states
  "Opt-in per runtime … ABSENT means today's pod exactly". Correct it to say what
  absent means now, and that the fallback covers a missing volume as well as a
  missing declaration

### 5.2 The adopter site

Skipped independently of 5.1, and by an adopter who reads none of it.

- [ ] 5.2.1 `docs/getting-started.md` — the demo runs a real conversation. If it
  installs without a context volume it now takes the fallback path; the page must
  not imply continuity it does not have
- [ ] 5.2.2 `docs/guides/agent-runtime.md` — running an agent on another backend is
  exactly the case where paths are NOT known. The guide teaches declaring them,
  and states that a runtime declaring none gets no synchronisation
- [ ] 5.2.3 `docs/index.md` and `docs/introduction.md` — review. Edit only if
  either states or implies the old default
- [ ] 5.2.4 Confirm no site page still describes context synchronisation as
  something an operator turns on
