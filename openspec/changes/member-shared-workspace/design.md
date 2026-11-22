## Context

See `proposal.md` for the motivation. The current state that shapes the approach:

- The workspace is a claim (`WorkspacePVC`, off by default, read-write-many wanted) or an `emptyDir`, mounted at the workspace path with `subPath` set to the conversation's name (`runtimepod/podspec.go`).
- `createMember` (`chat/coordinate.go`) builds the member's record and resolves `WorkspaceClaimName` through `runtimepod.SnapshotFor` from the member's OWN capability, so it may differ from its parent's. `CausedBy{Parent, Entry}` is recorded. Refs and claim are snapshotted and never re-resolved.
- Every runtime today clones when `.git` is absent and otherwise fetches and runs `reset --hard` (`runtimes/pi/runtime.js`, `RT-1`). On a shared tree that destroys the parent's uncommitted edits.
- `invoke` attaches to any live non-closed member with the same `(parent, entry)`, and a member's `/work/done` appends its result to the parent without closing it. A member keeps its context across tasks. Invocation stays asynchronous and this change does not touch it.
- `pi` loads the checkout's `.claude` content through `pi-code` (`PI_CODE_ENABLED`, chart `pi.piCode`, default true). `runtimes/claude` passes no `settingSources`, so the Agent SDK default loads `CLAUDE.md` and `.claude/`. The only control today is chart-wide.

**Sequencing: apply after `multi-repo-workspace` and `conversation-identity`.** Both modify specs this change modifies.

| Relied on | From | Used for |
|---|---|---|
| D6, a completed clone is never touched | `multi-repo-workspace` | the shared tree is not reset when a member starts |
| the `workspace-sync` init container | `multi-repo-workspace` | the owner's tree is populated before any member exists, so a member needs no init container |
| the subdirectory is named by `ID`, not by name | `conversation-identity` | a shared directory has one stable owner key, and a reused name cannot alias it |

Header overlaps, and what this change restates:

| Spec | Also modified by | This delta |
|---|---|---|
| `runtime-contract`, `The specification is exhaustive` | `multi-repo-workspace` | restates THEIR version plus `repoContext`, so it must be rebased onto theirs if theirs moves |
| `conversation-housekeeping`, `An orphan directory is identified by ordering, not by a timeout` | `conversation-identity` | restates THEIR version plus the shared-directory rule |
| `runtime-workspace-persistence` | both | a different requirement from either, no collision |

**Path.** Both earlier changes' order matters for one more fact. `multi-repo-workspace` moves the workspace path to `/workspace`. This change's specs say "the workspace path" and the `WORKSPACE` variable and never name a path. Nothing here moves it, and the tasks use whichever path the applied tree has.

**New word.** "Workspace owner" is the conversation whose subdirectory is mounted. It is not "root" (`terminology.md` reserves that for the uncaused conversation), and an owner may itself be a member. The word is defined once, in `member-workspace-sharing`.

## Goals / Non-Goals

**Goals:**
- One declared exception to the isolated workspace, stated where the Coordinator author reads it.
- A per-conversation control of repository context that every shipped runtime either honours or refuses loudly.

**Non-Goals:**
- Write coordination, locking or merging.
- Read scoping inside the tree (`workingDir`, `multi-repo-workspace`).
- Any change to `invoke`'s asynchronous shape.
- Moving the workspace path.

## Decisions

### D1. The switch is an entry-level field, `workspace: own | shared`

Where it lives decides who can see it and who can vary it.

| Option | Verdict |
|---|---|
| Entry-level `workspace` | chosen |
| A Coordinator-level flag | rejected |
| Infer sharing from claim equality | rejected |

- **Entry-level** matches how the role is expressed. A development Coordinator wants `backend` and `tester` to share and `reviewer` not to. `description` already lives on the entry for the same reason (`coordinator-model`: two Coordinators may use one capability differently).
- **A Coordinator-level flag** is all or nothing. A read-only reviewer then gets write reach it does not need, and least privilege says that reach is stated per member.
- **Claim equality** shares by accident. Two members resolving one default claim would silently mount one tree, which the existing requirement forbids and which no author would notice. Silence must grant nothing.

The default is `own`, so no existing Coordinator changes. The value set is closed (`own`, `shared`) and CEL enforces it.

### D2. The owner is found along `causedBy`, and the owner is recorded at creation

Resolution at `createMember`, one hop:

1. Take the parent's record.
2. If the parent records a workspace owner, the owner is that value. Otherwise the owner is the parent.

One hop is enough because the parent's record already holds its own resolved owner, so a chain of sharing members is flattened as it is built and no walk is needed. This gives the proposal's "first non-sharing ancestor" by induction.

The result is stored on the member as `WorkspaceOwnerID` and the member's `subPath` and claim come from the owner's record. The owner is never re-resolved, in the way `SnapshotFor` and the refs are not. An absent field reads as the conversation itself, so existing records need no migration.

*Alternative:* walk `causedBy` at pod build. Rejected. It would re-resolve on every pod rebuild and let a mid-flight edit or a deleted ancestor move a pod's mount, which breaks the snapshot rule.

### D3. `shared` forces the owner's claim and refuses where none exists

A member's claim comes from its own capability, so two conversations in one composition may resolve different claims. Three options:

| Option | Verdict |
|---|---|
| `shared` forces the owner's claim and `subPath` | chosen |
| Require the member's own resolved claim to equal the owner's | rejected |
| Silently fall back to `own` when claims differ | rejected |

Forcing is the only option with a single meaning. Requiring equality makes a Coordinator's readiness depend on every capability's persistence block and fails with a message about two claim names the author never wrote. A silent fall-back breaks the explicit-error rule, since the author asked for sharing and got none.

What the member keeps is its own context volume. Only the workspace follows the owner.

An `emptyDir` is per pod and cannot be shared. Two places refuse:

- **Readiness.** A `Coordinator` with a `shared` entry whose own root resolves no workspace claim is `Ready=False` naming the entry. This runs where the Coordinator's other readiness checks already do, so the late error never arrives after admission and a slot.
- **`invoke`.** Readiness checks the Coordinator's own root, but a nested Coordinator is a member and its root may share an owner it does not resolve itself, or the reverse. The owner is only known at invoke time, so the manager re-checks there and refuses with the entry and the reason. No conversation is created.

*Consequence to state in the docs:* a nested Coordinator with `shared` entries and no claim of its own is `Ready=False` standing alone and could work under a sharing parent. This is the conservative direction, since the standalone case is real (it claims sources) and a false Ready would fail only at the first signal.

### D4. A sharing member gets no init container, no credential and no reset

`multi-repo-workspace` D6 says a completed clone is never touched. Relying on it alone would still let the member's own init container run against the shared tree, and if the member's capability lists different repositories it would clone them in. So a sharing member is built with NO `workspace-sync` init container, and its capability's `repositories` and credentials are ignored for the pod. The owner has already run at least one unit, because it invoked, so its tree is populated before any member exists.

This also keeps a lower-privilege member's git credential out of a tree a higher-privilege parent owns, and the reverse.

Today's reset behaviour (`RT-1`) is what makes this requirement necessary if the order is wrong. That is why the sequence is after `multi-repo-workspace` and the change is not archivable before it.

### D5. `repoContext` is an entry-level field, `load | omit`

| Option | Verdict |
|---|---|
| Entry-level `repoContext` | chosen |
| Capability-level | rejected |

The same capability may be a full-context agent in one Coordinator and a narrow member in another. A capability-level field makes the second Coordinator define a near-duplicate capability only to flip the flag, which is the drift the capability kind exists to prevent. It also sits oddly beside `workspace`, which is also entry-level. A task that shares a tree usually wants the narrow context too, and the two fields are read together on one entry.

Names: `repoContext` follows the contract's vocabulary of "repository" and "context" and uses no vendor noun. `load` is the default and `omit` the exception, so a reader sees what departs from today.

### D6. The setting rides the work unit, only when `omit`

A runtime receives per-conversation configuration on the work unit (`docs/contracts/runtime.md`, `internal/dispatch`). An env var or a pod label would fix the setting per pod, and `pi.piCode` already shows that fails: it is chart-wide. The conversation record snapshots the value at creation and dispatch copies it onto every unit.

The field is sent only for `omit`, and absent reads as `load`, the way `toolsMode` absent reads `merge`. An old runtime ignores the unknown field, which is why this is a MINOR change, and an old runtime would then load the context silently. That is the exposure `RT-4.1` closes for conformant runtimes, and a deployment mixing versions is not a supported state because the runtimes ship in the same release.

**Runtime obligation (proposed `RT-4`, `RT-4.1`).** `RT-1` is removed by `multi-repo-workspace`, `RT-2` and `RT-3` exist, so `RT-4` is the next free id.

| Item | Text |
|---|---|
| `RT-4` | With `repoContext: omit`, load none of the repository context. Absent or `load` behaves as before. |
| `RT-4.1` | A runtime that cannot guarantee it MUST fail the run naming `repoContext`, never load silently. |

"Repository context" is defined once, in `member-repo-context`: instruction files, rules, skills, commands, hooks, settings and every agent definition except the one named by `agent` or `agentFile`. The named definition is deliberately not repository context. It is the member's own identity, read under `RT-3.4`, and omitting it would defeat the point of a narrow member.

**Contract edit needs approval.** `docs/contracts/runtime.md` is not edited without the user's explicit approval (`runtime-spec.md`, `runtime-contract`: `The specification governs the agent's own changes`). This change does not edit it. The `runtime-contract` delta states what the page must say, and the tasks carry the edit behind an explicit approval step. Once approved, the same change MUST bring all five runtimes into conformance and move the claimed version on each page, bumping MINOR with a changelog row and two checklist items.

Per runtime. `claude` and `claude-cli` have no capability spec of their own, so the `runtime-contract` delta is their requirement, exactly as in `multi-repo-workspace`. The other three each carry a delta in this change.

| Runtime | How `omit` is honoured | Status |
|---|---|---|
| `pi` | skip `pi-code` and its approval for the conversation, the env stays a ceiling | `piCodeLoaderOptions` knows the env today, a per-unit input is new |
| `claude` | pass `settingSources: []`. Skills and `CLAUDE.md` come through setting sources in the SDK | to confirm that no other path loads them |
| `claude-cli` | the real CLI loads memory and settings its own way | UNVERIFIED, see Open Questions |
| `ollama` | loads none already, so conforming by construction | spec'd and tested, no behaviour change |
| `copilot` | SDK options for instructions, skills and custom agents | UNVERIFIED, see Open Questions |

### D7. Security: a shared filesystem is a path around ServiceAccount isolation

A member's ServiceAccount is a privilege boundary (`wiring.md`: tools and identity are the same decision). A shared tree is a channel across it:

- A low-privilege member can write a script, a git hook, a `Makefile` target or a CI config that a higher-privilege parent later executes under its own account.
- Content a member writes is read by the parent as if it were the parent's own.
- Neither direction is mediated by the manager, since the filesystem is not.

Controls in this design:

- `own` is the default and silence grants nothing (D1).
- The field is on the entry, so the author states the reach next to the member, and a reviewer sees it in the same object that names the capability.
- A sharing member's record names its owner (`member-workspace-sharing`), so the relation is readable after the fact.
- The claim ROOT stays unreachable. Only the owner's subdirectory is mounted.
- No member's git credential reaches the tree (D4).

What it does not control: the content. Treating a file a more-trusted conversation reads from the shared tree as untrusted is the caller's discipline. `docs/security.md` owns the control text and the open risk, and a moved trust boundary re-runs `threat-model.py`. The chart value that sets the claim stays in `docs/configuration.md`.

### D8. Lifecycle: a shared directory is live while any conversation mounts it

Housekeeping lists conversations through `/state/conversations` and decides an orphan by `ID`, phase-blind, scanning before listing. With sharing, a directory named by the owner's `ID` is mounted by members too. Deleting the owner's object would orphan it while a member runs.

The rule: a directory is live while any conversation, in any phase, carries its `ID` or records it as its workspace owner. The scan-then-list ordering is unchanged and the listing stays phase-blind. The record's owner field must be exposed on the `/state/conversations` response (the one contract surface housekeeping reads), which is named for `docs/contracts.md`.

Closing the owner: `/close` already tears down only the pod, ConfigMap and slot and leaves the object and its directory, so a member's tree survives with no new code. The change adds a test for it. The owner pod's release stays explicit under every backend, as before. Nothing depends on garbage collection.

*Alternative:* cascade a close from the owner to its sharing members. Rejected. It silently ends work and is a lifecycle decision `coordinator-model` does not make today.

### D9. No write coordination

Locking and merging were rejected. A lock needs a protocol every runtime and every tool the agent runs would have to honour, and the agent's own shell does not. Disjoint slices are the invoking agent's job, as for Claude Code subagents, and the docs say so beside the field.

### D10. Invoke is unchanged

The member's result returns to the parent as an input and an attach reuses a live member's context. `repoContext` and `workspace` come from the entry that opened the member, so an attach keeps the recorded values even if the entry changed since, matching how refs are snapshotted.

## Risks / Trade-offs

- [A parent executes what a low-privilege member wrote] → default `own`, the author states `shared`, the docs name the path-around-isolation consequence, and the content is the caller's to distrust (D7).
- [Concurrent writes to one path corrupt a file] → no coordination by design. Disjoint slices, documented (D9).
- [An old runtime ignores `omit` and loads silently] → runtimes ship in the same release, the contract's conformance rule requires all five to conform, and `RT-4.1` makes an unsupported case fail loudly.
- [`claude-cli` or `copilot` cannot suppress loading] → `RT-4.1` fails the run with a named reason. This is a degraded result, not a wrong one, and is surfaced by the open questions.
- [A shared directory is reclaimed under a live member] → the liveness rule (D8), with a housekeeping test of owner-deleted-member-live.
- [A nested Coordinator is `Ready=False` alone yet would work under a sharing parent] → conservative by design, stated in the docs (D3).
- [The owner's pod is idle-evicted while members work] → the directory is on the claim and survives it, and no member depends on the owner's pod, only its tree.
- [Two revisions of one shared spec file] → `runtime-contract` and `conversation-housekeeping` deltas restate the other changes' versions. If either of them changes before this one lands, rebase this delta.

## Migration Plan

1. Land after `multi-repo-workspace` and `conversation-identity`.
2. Apply `chart/crds/` as always, since `CoordinatorAgentEntry` gains two optional fields. No existing object needs an edit and no stored record needs a migration, since both new record fields read as their default when absent.
3. Rollback: an older manager ignores the unknown fields on the CR (CRD pruning would drop them) and ignores the work-unit field in a runtime. A member that was sharing would then mount its own tree, so roll back only with no sharing member open.

## Open Questions

- **`claude-cli` and `copilot` suppression is unverified.** Reading the code found no setting-source or instruction-directory option in either `runtimes/claude-cli` or `runtimes/copilot`. Whether the real CLI can skip `CLAUDE.md` and `.claude/` per invocation, and whether the Copilot SDK can skip repository instructions, skills and custom agents, is checked in the tasks before implementing. If either cannot, that runtime fails `omit` under `RT-4.1` and the user decides whether that is acceptable for the release. The specs stand either way.
- **Approval of the `docs/contracts/runtime.md` edit** is needed from the user before the tasks reach it. The delta spec is the interim record.
- **Name of the record field** (`WorkspaceOwnerID` is proposed) and of the `/state/conversations` JSON key is settled with `docs/contracts.md` when the surface is edited. The specs require only that it is readable and that absent reads as the conversation itself.
