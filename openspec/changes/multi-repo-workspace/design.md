## Context

See `proposal.md` for the motivation. The current state that shapes the approach:

- `AgentProfile.spec.repository` is one `{url, ref, auth}`. The pod builder sets `REPO_URL` and `REPO_REF` and mounts the workspace claim at `/data/workspace` with `SubPath=conv.Name`.
- Each of `runtimes/claude`, `claude-cli`, `copilot`, `ollama` and `pi` clones for itself and holds the git credential. Today's https clone rewrites the URL to `x-access-token:<token>@…`, so the token lands in `origin`.
- The pod builder reads the `AgentProfile` live at every build. A pod rebuilt after an edit follows the new profile.
- Stored transcripts are keyed by the working directory. Any move of the workspace path strands them.

The requirements live in `specs/multi-repo-workspace` and `specs/workspace-sync`. This document records the choices behind them.

**Dependency.** `agent-definition-adopted-by-runtime` makes the runtime read the definition's body and tools. This change gives that definition an explicit location and does not repeat its requirements.

## Goals / Non-Goals

**Goals:**
- One place that clones, one place that holds a git credential, and neither is a runtime.
- A runtime contract that says "a populated `/workspace`" and nothing about git.
- A repository list fixed at conversation creation.

**Non-Goals:**
- Conversation identity. The workspace `subPath` is `conv.Name` and a name can be reused after a delete. That is a separate fault, fixed by `conversation-identity`, and this change neither depends on nor solves it.
- Merging lists by name.
- Per-repository write policy beyond `keepGit`.
- Migrating stored transcripts across the path move.

## Decisions

### D1. `repositories[]` replaces, never merges

A capability's list REPLACES the profile's. A merge by name needs a rule for conflicting `ref`, `auth` and removal, and every rule surprises someone. Replacement is one sentence. `agentFile` and `workingDir` ride with the list because both name entries of it, so a replaced list with a kept `agentFile` would dangle by construction.

*Alternative:* merge by `name`. Rejected, listed as out of scope.

*Alternative:* a repository field on `Pipeline` and `Coordinator`. Rejected, since both already reach an `AgentCapabilitySpec` and a third place for one fact is the drift the capability kind exists to prevent.

### D2. Snapshot the resolved list onto the Conversation

The list, `agentFile` and `workingDir` are written at creation, in the way `runtimepod.SnapshotFor` carries persistence claim names. Pod builds read the snapshot. Two reasons:

- A pod rebuild after an edit must not re-wire an in-flight conversation, and a layout change from 1 to N repos would move the agent's cwd under its stored transcript.
- It fixes today's live read of the profile for repositories.

What is snapshotted is references and strings. A `secretRef` is a name, so the manager still reads no secret. Reopen reuses the snapshot and resolves nothing again.

### D3. The layout is a function of the count

| Count | Result |
|---|---|
| 0 | empty `/workspace`, no init container |
| 1 | the clone is `/workspace` |
| 2+ | `/workspace/<name>/` each |

One repo as the root keeps every existing single-repo setup working, including `CLAUDE.md` at the root. The cost is that a one-repo prompt that says `app/src/x` is not portable to two repos, and an alias `/workspace/<name>` for one repo was rejected because it would make that path work by accident and then break on the first second repo. The documentation states the non-portability.

Conversation immutability (the spec's last requirement) is what makes the count-dependent layout safe. The layout cannot flip under a running conversation.

### D4. The init container owns cloning

A new component `platform/workspace-sync`. Its path is its published name, `agentops-workspace-sync`, per `.claude/rules/structure.md`, and it is multi-arch like every image.

| Choice | Decision | Why |
|---|---|---|
| Where | init container, not a sidecar | it must finish before the agent can start, and nothing needs it afterwards |
| Input | `WORKSPACE_REPOS` plus one `REPO_CRED_<n>` per credentialed repo | the manager reads no secret, so each `secretRef` becomes `valueFrom` on the init container only |
| Credential in the agent container | none | the agent can run arbitrary commands and a credential in its environment is one `env` away |
| Component | its own image | one clone routine instead of five, and git is not needed in a runtime image for cloning |

Runtimes keep `git` itself in the image only if they want the agent to run it. The contract no longer requires it.

### D5. A credential never lands in git config

| Auth | Mechanism |
|---|---|
| https token | `GIT_ASKPASS` helper reading the per-repo variable, or a per-command `http.extraHeader`. `origin` keeps the declared URL |
| ssh key | written to a mode-0600 file outside `/workspace`, used through `GIT_SSH_COMMAND` with `IdentitiesOnly=yes` |

Rewriting the URL, as today's runtimes do, persists the token in `.git/config` on a volume that may outlive the pod and that the agent reads. Forbidden. The same constraint covers every file under `/workspace`. Termination messages and logs name the repository and the cause and never an env value.

### D6. Re-run rule: a completed clone is never touched

The user's stated starting point was fetch plus `reset --hard` for a kept `.git`, as today's runtimes do. The recommended rule is different: with a completion marker, leave the repository alone, whatever `keepGit` says.

- `reset --hard` discards uncommitted edits. The existing persistence spec promises "restart mid-conversation keeps the tree, including work not yet committed", and a silent reset on restart breaks that promise.
- The agent owns the tree after the first clone. A ref that moved upstream is not a reason to rewrite an in-flight conversation, which fits D2's rule that a conversation does not follow edits.
- No fetch means a restart needs no network or credential to succeed.

The cost is that a restarted pod does not pick up new upstream commits. A new conversation does.

The marker is `/workspace/.agentops/sync/<name>.done`, written last. Its absence means the clone was cut short, so that repository's contents are removed and cloned again. For a single repo the removal spares `.agentops`, and `.agentops/` goes into `.git/info/exclude` when `.git` is kept so it never shows as untracked. A marker in a git file was rejected because `keepGit: false` has no `.git`.

### D7. How the runtime learns the layout

| Carrier | Content | Why there |
|---|---|---|
| env `WORKSPACE` | `/workspace`, always | replaces the hard-coded path the runtimes carry |
| env `START_DIR` | `/workspace/<workingDir>`, only when set | a pod fact, fixed by the snapshot |
| work-unit field `agentFile` | absolute path, exclusive with `agent` | beside `agent` it replaces, so a runtime picks one |

The manager resolves the path and the runtime decides what to do with it, so the contract stays vendor-neutral. The init container also verifies the file exists, is regular and stays inside its repository after symlinks, because failing at pod start with a named path is a clearer error than a runtime that quietly contributes nothing. The runtime checks again, since a file under `/workspace` can change between init and run.

`prompt` and `replyPrompt` take an optional `<repo>:<path>` qualifier. A repo name is a DNS label and holds no `:`, so the first colon splits unambiguously, and a plain path stays valid with one repo.

### D8. Validation is on the holders

CEL on each CRD covers what one object can express: unique names, safe names, `agent` against `agentFile` exclusivity, no `..` or leading `/` in a path, `repository` against `repositories`. The cross-object cases need the resolved list, which exists only on the holder. `Ready` on `Pipeline` and `Coordinator` reports them. A failure at pod start would arrive after admission and a slot, which is the late error the reaping invariant exists to bound.

### D9. A failed clone rides the existing start-deadline path

The init container exits non-zero and the pod stays `Init:Error`, so it never reaches Running. The start deadline already reaps pods that never start and carries the kubelet's reason verbatim, so no new condition is invented. The termination message is the reason's source, which is why it must name the repository and omit credentials.

### D10. The path moves with no migration

`/workspace` replaces `/data/workspace` because a clone that is the workspace root should not sit under a `data` prefix shared with the context mount, and because one short path serves every runtime. Stored transcripts are keyed by cwd, so every context stored before the upgrade is stranded. Operators delete conversations before upgrading and the chart's major version and `docs/CHANGELOG.md` say so. A migration that renames transcript directories was rejected as one-shot code for a pre-1.0 API.

**Retired vocabulary.** Add to `.github/retired-vocabulary.json` in this change:

| Retired | Say instead |
|---|---|
| `/data/workspace` | `/workspace` |
| "the runtime clones the repository" | the workspace-sync init container clones it |
| `REPO_URL`, `REPO_REF`, `GIT_AUTH_TYPE`, `GIT_SSH_KEY`, `GIT_TOKEN` | the repository list in the init manifest |

`terminology.md`'s "`/data/workspace` never moves" section and the matching statements in `runtime-workspace-persistence` are retired in the same change.

## Risks / Trade-offs

- [Every stored context is stranded by the path move] → delete before upgrade, major chart version, changelog entry, a scenario in the persistence spec.
- [Init container adds startup latency to every pod] → the clone ran inside the runtime before, so the total is similar. An existing marker skips the work on restart.
- [A one-repo prompt breaks on a second repo] → documented, and the layout is fixed per conversation so nothing breaks mid-flight.
- [A restart does not refresh upstream] → intended, see D6.
- [The `.agentops/` marker dir appears in a single-repo root] → excluded in git, and visible when `keepGit` is false.
- [The workspace `subPath` is `conv.Name`, reusable after delete] → out of scope, `conversation-identity`. A reused name could present an old workspace with a marker to a new conversation, which is that change's problem to close.
- [Chart and manager rollout skew] → the manager that emits the init container and the runtimes that stop cloning ship in one release, so a half-upgraded install is not a supported state.

## Migration Plan

1. Delete conversations on the old release.
2. Upgrade the chart. Apply `chart/crds/` as always.
3. Move `repository` to `repositories` at leisure. The deprecated field keeps working.
4. Rollback means the old release plus deleting the conversations created under the new one, since their transcripts sit under `/workspace`.

## Open Questions

- None. The manifest is `WORKSPACE_REPOS` and each credential is `REPO_CRED_<n>`, as `workspace-sync` states.
