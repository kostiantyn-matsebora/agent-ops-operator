## 1. The fallback, and its test first

D2 in `design.md` makes this ordering a correctness constraint, not a
preference: the defect is rare today and universal once the default flips.

- [x] 1.1 Add a failing test to `platform/manager/internal/runtimepod/contextsync_test.go`
  constructing a config with `ContextSyncImage` set, a runtime declaring
  `contextSync.paths`, and `ContextPVC: ""`. Assert no volume in the built pod
  references a `PersistentVolumeClaim` by an empty name. Verify it FAILS against
  the current predicate — a test that passes before the fix is testing nothing
- [x] 1.2 Add `&& cfg.ContextPVC != ""` to the `sidecar` predicate at
  `platform/manager/internal/runtimepod/podspec.go:287`, and verify 1.1 now passes
- [x] 1.3 Assert the fallback is TODAY'S pod, not merely a pod without the store:
  extend 1.1 to check the agent container mounts `context` at `/data/context`
  from an `EmptyDir`, that no `context-sync` container is present, and that
  `CONTROL_URL` points at the manager rather than at `127.0.0.1`
- [x] 1.4 Pin the agreement `design.md` D1 names: a test asserting that whenever
  NO DURABLE CONTEXT VOLUME is configured, `Build` produces a pod referencing no
  persistent context claim, whatever the runtime declares. This is the invariant
  the defect broke. It is NOT keyed on `ContinuityPossible()` being false: that
  is also false for `contextStorage: none`, which correctly keeps the claim it is
  given, so the broader reading fails on a case nothing is wrong with
- [x] 1.5 Confirm the missing-image and missing-claim fallbacks are the SAME rule
  — `TestContextSyncWithoutAnImageFallsBackSafely` and the new test should assert
  the same pod shape. If they diverge, one of them is describing an exception.
  The shape they share is no sidecar container, no store volume, `CONTROL_URL` at
  the manager, `context` at `/data/context` and no extended grace period; the
  context volume ITSELF is stated per case, since claim-where-there-is-one and
  ephemeral-where-there-is-not is the ordinary unsynchronised rule
- [x] 1.6 Note the shared config at `contextsync_test.go:17` sets
  `ContextPVC: "agentops-context"` for every case, which is why this was never
  constructed. Leave it as the default and override per-test, so existing cases
  are untouched
- [x] 1.7 Run the full manager suite with `KUBEBUILDER_ASSETS` and verify no
  existing context-sync test regressed

## 2. The default

Only after section 1 is green. See `design.md` D3 and D4.

- [x] 2.1 In `chart/charts/claude/values.yaml`, set `contextSync.paths` to
  `[".claude/projects/-data-workspace/**"]` — the value currently commented out
  directly beneath it — and verify `helm template` now renders a `contextSync`
  stanza on the AgentRuntime with default values.
  **THE VENDOR'S BUNDLE, NOT `global.agentops.runtimeDefaults`.** The key is
  spelled `contextSync` in both places, so the wrong one renders and looks
  correct until a second vendor is installed
- [x] 2.2 Replace the comment explaining why the list was empty with one stating
  what declaring it means and that a different backend must replace it. The
  reasoning about only-the-runtime-knowing is KEPT — it is why a third-party
  runtime still declares its own — not deleted along with the empty default
- [x] 2.3 Verify the render needs no edit: `agentops.renderRuntime` in
  `chart/templates/_helpers.tpl` already reads
  `{{- with (($rt.contextSync | default dict).paths) }}`, and it is the ONE
  helper the parent and the `claude` bundle both call. If it needed an edit, D3
  is wrong and the design is revisited rather than the template patched
- [x] 2.4 Verify the opposite direction still works: `claude.contextSync.paths: []`
  renders no stanza, and the pod is the direct-mount pod
  (`TestClearingContextSyncRestoresTheDirectMount` covers the manager half)
- [x] 2.5 Render with `persistence.context` disabled AND the new default paths
  set, and confirm the manifest is valid — this is the combination section 1
  fixed, now reachable by default
- [x] 2.6 Run the chart render tests.
  **ONE FAILED, AND IT WAS THE TEST RATHER THAN THE CHANGE.**
  `TestPersistenceOptOutRemovesEverything` asserted `!Contains(out,
  "agentops-context")`, and `agentops-context` is a PREFIX of
  `agentops-context-sync` — the sidecar IMAGE, which the manager's bootstrap env
  carries as soon as any runtime declares context paths. Turning the default on
  made a test about a missing CLAIM fail on an install that has no claim at all.
  Tightened to match the claim as a claim

## 3. Verify it runs, not that it renders

A rendered pod is not a running one. `gotchas.md` records what this repo paid to
learn that, and `2026-08-21-context-survivability` smoked the sidecar for the
same reason.

- [x] 3.1 Install with default values against a cluster with a context volume,
  start a conversation, and confirm a `context-sync` container is present and the
  agent container holds NO mount of the durable claim
- [x] 3.2 Confirm a checkpoint actually lands: inspect the durable volume for a
  generation under the conversation's subPath, and confirm `current` resolves
- [x] 3.3 Confirm continuity across pods — let the pod idle out, send a second
  message, and confirm the answer continues rather than starting fresh
- [x] 3.4 Install with persistence disabled, start a conversation, and confirm
  the pod STARTS, runs with ephemeral context, and the reply says the context is
  not promised. This is the fallback path end to end
- [x] 3.5 Confirm the reset verb recovers a conversation whose handle predates
  the layout move, since that is the recovery the changelog will name.
  **THE VERB IS VERIFIED; THE PRECONDITION COULD NOT BE MET ON THIS CLUSTER, AND
  THAT IS THE FINDING.** The reference install had set
  `runtimeDefaults.contextSync.paths` BY HAND long ago, so every conversation
  back to 09:17 already carried the `gen-*/current` layout — there was no
  pre-move handle to strand, and a pre-move conversation resumed cleanly rather
  than failing. The migration risk in the changelog is real for installs that
  never opted in; it is NOT a risk for one that already had.
  - The verb itself was exercised on a throwaway persistence-off release:
    `POST /channel/conversations/{name}/reset-context` with `{"channel": …}`
    returned `cleared`, emptied the handle, and left the conversation, its
    thread and both recorded runs intact
  - **`channel` is REQUIRED in the body** — omitting it answers
    `need {"channel"}`. Worth knowing before reaching for it mid-incident

## 4. Close out

- [x] 4.1 Every module builds, vets and tests — the loop in `.claude/rules/build-test.md`
- [x] 4.2 `python3 .github/scripts/publication-guard.py` and
  `python3 .github/scripts/retired-vocabulary-guard.py` both pass. Record the
  VERDICT only, never a matched value
- [x] 4.3 `openspec validate context-sync-by-default --strict`

## 5. Documentation

Both halves, listed separately because they are skipped independently: a
behaviour change feels done once `concepts.md` is right, and the adopter never
reads `concepts.md`.

### 5.1 The reference docs

- [x] 5.1.1 `docs/CHANGELOG.md` — newest first. State that context
  synchronisation is now on by default where a context volume exists, that the
  durable layout moves from the claim root to a per-conversation path, that a
  conversation holding a context handle FAILS its next run rather than answering
  without memory, and name the reset verb as the recovery. State that no
  migration is provided and why
- [x] 5.1.2 `docs/concepts.md` — `AgentRuntime.spec.contextSync` is no longer
  "absent by default". Document the no-volume fallback as part of how continuity
  resolves, beside `contextStorage`
- [x] 5.1.3 `docs/installation.md` — the storage section. What a reader turning
  persistence off now gets, and the ephemeral-storage consequence of the default
  (`liveSizeLimit`, and that `$HOME` including caches is now pod-local on every
  conversation)
- [x] 5.1.4 Re-run `python3 .github/scripts/docs-generate.py` — a chart value's
  default changed, so `docs/cr-reference.md` and every generated block are stale.
  CI fails on a difference
- [x] 5.1.5 `.claude/rules/terminology.md` — the `context-sync` entry states
  "Opt-in per runtime … ABSENT means today's pod exactly". Correct it to say what
  absent means now, and that the fallback covers a missing volume as well as a
  missing declaration. It now names all THREE conditions of the one fallback
  rule — no declaration, no sidecar image, no volume — and says the paths ship in
  the vendor's bundle

### 5.2 The adopter site

Skipped independently of 5.1, and by an adopter who reads none of it.

- [x] 5.2.1 `docs/getting-started.md` — the demo runs a real conversation. If it
  installs without a context volume it now takes the fallback path; the page must
  not imply continuity it does not have
- [x] 5.2.2 `docs/guides/agent-runtime.md` — running an agent on another backend is
  exactly the case where paths are NOT known. The guide teaches declaring them,
  and states that a runtime declaring none gets no synchronisation
- [x] 5.2.3 `docs/index.md` and `docs/introduction.md` — review. Edit only if
  either states or implies the old default
- [x] 5.2.4 Confirm no site page still describes context synchronisation as
  something an operator turns on
