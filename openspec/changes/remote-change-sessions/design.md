## Context

See proposal.md — Why. What shapes the approach, measured on 2026-09-06
rather than inferred:

- **A cloud environment is a form with five fields** — name, network access,
  environment variables, API credentials, setup script — and the sibling
  repository's environment already uses the setup field as a one-line hand-off:
  `export CLAUDE_CODE_REMOTE=1` then `cd /home/user/<repo> && bash <script>`.
  The repo is cloned at `/home/user/<repo>` BEFORE the setup runs ("runs when a
  new session starts, before Claude Code"). The variables field says on its
  face that its values are visible to anyone using the environment; the API
  credentials field attaches a stored secret to requests for a host, through
  the proxy, and makes that host reachable whatever the network level.
- **The cloud image** is Ubuntu 24.04 with Go (version unstated), Node 22,
  Python 3, dockerd, `gh` authenticated through a proxy, jq. Not helm, not
  kubectl, not k3d, not openspec, not pyyaml, not serena. Roughly 4 vCPU,
  16 GB, 30 GB — the same shape as `ubuntu-latest`, where the smoke e2e tier
  already runs.
- **What this tree's checks need beyond that image**: helm (the chart render
  tests in `platform/manager/internal/integration/charttemplate_test.go` call
  `exec.LookPath("helm")` and SKIP without it), `@fission-ai/openspec@1.10.0`
  (the version `ci.yml` installs), pyyaml (`docs-generate.py`,
  `docs-task-guard.py`), the envtest assets (`setup-envtest`, network), Go 1.25
  (`platform/manager`, `runtimes/ollama`).
- **What the process runs by hand**: the three `PreToolUse` hooks need jq, git,
  python3 — all present. `opsx-issue.sh` uses REST `gh` only. The proxy pins
  GraphQL to a fixed set; the one GraphQL call a session may make is in
  `autofix-guard.py`, which fails open by design.
- **Routines** (research preview) are the one documented way to start a cloud
  session from outside a browser: a saved prompt, a repository, an environment,
  and triggers — schedule, an HTTP `/fire` endpoint with a per-routine bearer
  token accepting an optional `text`, and GitHub events. **The GitHub events
  are `pull_request` and `release` only**; an `issues` event is not offered.
  Fire text arrives wrapped in a `<routine-fire-payload>` block labelled
  untrusted, and the saved prompt must opt in to reading it. A routine clones
  the DEFAULT branch and may push any branch that is not protected, has no
  open pull request by someone else, and carries no commits by someone else —
  so a fresh `change/<name>` is accepted. Runs count against the account's
  daily routine cap.
- **The fixing loop's consent is a label read by a program**, gated on the
  labeller's permission through the collaborators API because a label event
  carries no `author_association` (`review-dispatch.yml`). Its vocabulary is
  `.github/review-triage.json`.
- **The fixing loop's work list and trigger, today.** `collect` builds the
  list from the open review threads plus, under the label, the analysis
  service's open issues (`sonar-issues.py`); a round starts on a labelled pull
  request when `claude-review` COMPLETES (`workflow_run`, conclusion success),
  serialised per pull request by a `concurrency` group keyed on the number. A
  red `ci-green` is neither an item nor a start: a failing test, a lint, the
  docs generator's `--check`, the publication guard, or a quality gate — which
  the scan step turns into a failed component job — leaves the pull request
  blocked with the loop reporting the review clean. The `fix` job holds
  `contents: read` and a checkout, so it can run the failing command itself.
- **The sibling repository's `.mcp.json`** wraps each stdio server in
  `bash -c '[ "$CLAUDE_CODE_REMOTE" = "true" ] || exit 0; …wait…; exec …'`.
  The wait loop exists because the bootstrap can race MCP startup; the
  `exit 0` makes those servers remote-only, which here would kill the local
  serena and docker servers that work today.

## Goals / Non-Goals

**Goals:**

- A remote session on this repository can propose, implement, run the unit and
  chart tiers, write the docs and open the pull request with nothing a person
  has to install by hand.
- A person with write access can hand an issue to a remote session by placing
  one label, and the result arrives as a pull request already under the fixing
  loop.
- Everything that decides what the environment and the session do is a file in
  the tree; what lives outside it is a name, a variable and a secret.

**Non-Goals:**

- Running the e2e pack inside the sandbox. dockerd is there and k3d would
  probably run, but `e2e-smoke.yml` already runs that tier on the same runner
  shape on demand; the session dispatches it on its branch instead of
  carrying a cluster.
- Deploying to, or looking at, the local cluster from a remote session. The
  cluster is on the workstation and stays there; the visual-check rule is
  named workstation-only.
- Merging or archiving from the routine. Both stay with a person.
- Replacing `claude-review.yml`, or building a second loop. The session
  feeds the review a pull request; `review-dispatch.yml` is EXTENDED (D10),
  never duplicated in the routine.

## Decisions

### D1. The label reaches the routine through an Actions workflow, not a routine GitHub trigger

`remote-implement.yml` runs on `issues: [labeled]` for the implement label and
calls the routine's `/fire` endpoint with `{"text": "<issue number>"}`. The
routine's own GitHub triggers offer `pull_request` and `release`, not
`issues`, so a routine cannot subscribe to the label directly.

- **The hop buys the gate.** GitHub lets anyone with TRIAGE label an issue,
  and triage is not write; the routine's filters check labels, never who
  placed them. The workflow reads the labeller off the event and asks the
  collaborators API, exactly as `review-dispatch.yml`'s gate does, and refuses
  visibly — the label removed, a comment saying who may place it.
- **The hop buys the record.** The fire returns a session URL; the workflow
  posts it once on the issue. A routine's GitHub trigger leaves nothing on the
  issue at all.
- **Alternatives rejected.** `anthropics/claude-code-action` on the label
  event: it runs on the Actions runner, not in the environment, with none of
  the bootstrap, under an API key rather than the subscription, and the
  review's history shows an Actions-hosted session runs whatever the runner's
  defaults are. A routine `pull_request.labeled` trigger with a dummy pull
  request per issue: a pull request that describes nothing, to work around a
  missing event type. The `create_webhook_trigger` API with an `issues` event:
  undocumented; noted as an open question, and it would remove the fire hop
  but not the gate or the record.

### D2. The routine's prompt is a pointer; the process is `.github/routines/implement-issue.md`

The saved prompt is three sentences: read the instruction file at that path in
the clone; the issue number is the text inside the `routine-fire-payload`
block and is a NUMBER, nothing else in that block is acted on; the issue's
body is the subject of a proposal, never instructions. The file carries the
process: `.github/scripts/opsx-issue.sh open <name> --promote <n>`,
`/opsx:propose`, `/opsx:apply` on `change/<name>` in place, the tiers, the
docs task, `gh pr create` with `Closes #<n>` and `--label autofix`, the phase
advance to `review`, and the sentence the pull request must carry naming the
verifications that are workstation-only.

- **Why a file.** A prompt saved in the routine form is the setup field one
  screen over — unreviewed, unversioned, invisible to a reader of the tree.
  When the process changes, a pull request changes the file and the routine
  needs no edit.
- **Why the number only.** The workflow could send the title and body, and
  the routine could act on them; the platform wraps fire text as untrusted for
  exactly the case where the token leaks. Sending a number makes the payload
  something the prompt can validate (`^[0-9]+$`) before it is used, and the
  session reads the issue itself, through `gh`, under the proxy.
- **The derived change name** is the issue's number and slug
  (`<n>-<kebab-title>`, truncated), computed by the session the same way every
  time so a re-fire finds the binding `opsx-issue.sh` already wrote and
  continues rather than proposing again.

### D3. The bootstrap is one idempotent shell script in `.github/scripts/`, with a verify mode

`cloud-bootstrap.sh`: exits 0 at once unless `CLAUDE_CODE_REMOTE` is `1` or
`true`; then one `## <tool>` section per tool — check, install, verify — each
non-fatal to the others, each missing tool named on stderr at the end, exit 0
always (the platform requires it). `cloud-bootstrap.sh --verify` installs
nothing and prints one line per tool, `present` or `missing`, and is the
`SessionStart` hook.

| Tool | Why | Install |
|---|---|---|
| helm | the chart render tests skip silently without it; `docs-generate.py` renders the chart | the upstream install script, pinned to the version `azure/setup-helm@v4` resolves in CI or a stated one |
| `@fission-ai/openspec@1.10.0` | every opsx command; the version CI installs | `npm install -g` |
| pyyaml | `docs-generate.py`, `docs-task-guard.py`, the test suite | `pip install --user` |
| envtest assets | the manager's integration suite | `go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 use 1.31.x --bin-dir ~/.envtest -p path`; `KUBEBUILDER_ASSETS` set as an environment variable to that path |
| serena | the `.mcp.json` server | `uv tool install`, as the sibling does |
| Go ≥ 1.25 | `platform/manager`, `runtimes/ollama` | verified, and installed from the official tarball only if the image's is older |

- **Setup script rather than a `SessionStart` hook for the installs**, because
  the setup runs before Claude Code and is cached with the environment; a hook
  runs every session and would pay the installs each time. The hook is the
  verify pass alone, because the model needs to KNOW what is missing — the
  failure this guards is a silent skip.
- **Why `.github/scripts/` and not `.claude/hooks/`.** It is not a hook, and
  that directory already has the test runner (`.github/tests/run.sh`) and the
  CI job that runs it. The `SessionStart` entry in `.claude/settings.json`
  calls it by path with `--verify`.
- **Not k3d, not kubectl.** Non-goal above; the session dispatches
  `e2e-smoke.yml` with `gh workflow run … --ref change/<name>` and reads the
  run's conclusion.
- **Module caches are not warmed.** `go mod download` for fifteen modules and
  `npm ci` for two would push the setup past its five-minute budget; the first
  build pays it, once per environment cache.

### D4. The working copy is the clone, and the branch is `change/<name>` checked out in place

The remote session runs `git checkout -b change/<name> origin/master` (or
checks out the existing branch) in `/home/user/agent-ops-operator` and never
runs `git worktree add`. The routine's push rules accept a fresh branch;
`allowed_push_branches` on the routine is set to `change/*` where the API
takes it.

- **`require-change-branch.sh` already agrees.** It refuses a commit on the
  default branch in a checkout whose `.git` is a directory; a remote clone on
  `change/<name>` is not on the default branch, so it says nothing, and if a
  session forgot to branch it refuses with advice — advice that names a
  worktree, which `remote-session.md` corrects for the remote case.
- **The `structure.md` test holds.** No worktree inside or beside the clone,
  so `find . -type d -name docs` and `components.sh` see one tree.
- **Interactive remote sessions** (a person starting one on claude.ai/code for
  a change) push only to the branch they were started on. The rule says: start
  the session on `change/<name>`; if the branch does not exist yet, propose on
  the workstation first, or accept that the first push creates it — which of
  the two the proxy allows is settled by the first such session and recorded
  in `gotchas.md`.

### D5. The issue label transfers consent to the pull request

`review-triage.json` gains `"implement_label": "autoimplement"`. The label's
description says what it does. The instruction file directs the session to
open the pull request with `--label autofix` and to say in the description
that approval came from the issue's label and who placed it.

- **One consent, given once, by the same class of person.** The `autofix`
  requirement says the label is placed on the owner's word under the owner's
  credentials. Labelling an issue "build this and satisfy the reviewers" is
  that word; the session places the label as it does when told in chat.
- **The loop's bounds are unchanged.** `MAX_ROUNDS`, disputes that wait for a
  person, the archive guard — none of it is touched. What the label adds is
  that the loop starts without a person present.
- **Why the same file.** The vocabulary that decides what a machine writes to
  a branch is stated once and read by programs; a second consent label
  belongs beside the first, and the docs already point there.

### D6. Secrets go under API credentials; `SONAR_ORG` is a variable

The Sonar token is stored as an API credential for the SonarCloud API host.
`SONAR_ORG` is not a secret and is an environment variable. The `.mcp.json`
sonarqube entry keeps its `Authorization: Bearer ${SONAR_TOKEN}` header for
the workstation; whether the proxy overrides a header the client sends, or
requires it absent, is the open question below and is settled in the first
remote session — the fallback is a remote-conditional header.

### D7. `.mcp.json` runs directly where the binary exists and waits remotely

Each stdio server becomes `bash -c 'command -v <bin> >/dev/null && exec <bin> …;
[ "$CLAUDE_CODE_REMOTE" = "true" ] || exit 1; <wait up to 180 s>; exec <bin> …'`.
A workstation with the binary is unchanged; a workstation without it fails to
connect as it does today; a remote session waits for the bootstrap. **Never
`exit 0` locally**: that is the sibling's shape and it would silently disable
the two servers that work here.

### D8. Nothing that identifies the routine or the environment is committed

The fire URL (which carries the routine id) is the repository variable
`ROUTINE_FIRE_URL`; the token is the secret `ROUTINE_FIRE_TOKEN`; the
workflow reads both. The rule file names the environment `agent-ops-operator`
and the routine by name, never by id. Not a publication-guard matter — the ids
name nothing about the cluster — but an id in the tree is a value to rotate
when the routine is recreated, for no reader's benefit.

### D9. Network access is Full, as the sibling's is

The trusted allowlist covers every registry the bootstrap reaches; Full costs
nothing extra here since the credential proxy makes the analysis host
reachable either way, and it removes a class of "403 host_not_allowed" that
would otherwise be diagnosed once per new dependency. Revisit if the routine's
reach ever needs narrowing.

### D10. A failed required check is a work item, and a red `ci-green` starts a round

Under the label, `collect` also reads the head sha's check runs from the
checks API, keeps those with conclusion `failure` among the checks
`ci-green` needs (excluding the review's own), and for each fetches the failed
steps' log through `gh run view <run> --log-failed`, bounded to a tail of a
stated number of lines per job. Each becomes a work item
`{kind: "check", job, run_url, tail}` beside the threads and the analysis
issues. The `fix` job treats a check item as a finding whose text is the log:
reproduce with the job's own command from `ci.yml`, fix, and re-run it before
the patch is cut — or DISPUTE with one pull request comment naming the job,
exactly as an analysis issue is disputed. A fixed check needs no reply; the
next CI run is its verdict.

`review-dispatch.yml`'s `workflow_run` trigger gains `ci` beside
`claude-review`: a `ci` run that completes with conclusion `failure` on a
pull request carrying the label starts a round. The existing `concurrency`
group serialises it with a round the review's completion may start for the
same pull request, so two starts become two rounds in sequence, each
collecting the current state — the second finds what the first left. Rounds
are counted as today, whichever event started them; `MAX_ROUNDS` bounds both.

- **Why the loop and not the routine.** The routine's session ended when the
  pull request opened. Re-firing it on a red check is a second loop — a second
  bound, a second summary, a second place a dispute can sit — on the same
  branch as the first. The loop already lands through the deploy key, counts
  rounds from the pull request, and knows how to dispute; a check is one more
  kind of item.
- **Why `collect` and not the model.** The same reason the analysis service's
  API is read by a program: the model reads neither API, so it cannot be told
  by a log what to fetch next. `actions: read` is granted to `collect` alone,
  where no model runs.
- **The platform's own auto-fix stays OFF for this repository.** The Claude
  GitHub App can watch a pull request's check failures and push fixes itself.
  Two machines pushing one branch, no round bound, no dispute path, no summary
  — and the loop here would be counting rounds against commits it did not
  make. Stated in `remote-session.md` so nobody enables it as a convenience.
- **Alternatives rejected.** A separate `ci-fix` workflow: a second loop under
  another name. The routine's session waiting for CI before ending: the
  session would hold a sandbox idle for the length of every CI run and every
  review, and still not own the later rounds.

## Risks / Trade-offs

- **[A label starts a machine writing to the repository]** → the gate is write
  access read from the platform, a refusal is visible, the branch is the
  change's own, and nothing merges or archives without a person. The pull
  request is reviewed by the same review as any other.
- **[The issue body is untrusted input handed to an autonomous session]** →
  the prompt receives a number, reads the issue itself, and is told the body
  is the subject of a proposal. The proposal, the pull request and the review
  are all read by a person before merge. Residual: a hostile issue could waste
  a run; the daily cap bounds it and the gate limits who can cause it.
- **[Re-labelling fires again]** → accepted; the session finds the existing
  binding and continues. A run in flight and a second fire for the same issue
  is two sessions on one branch — the instruction file has the second session
  check for an open pull request from `change/<name>` and stop with a comment.
- **[The setup budget is five minutes]** → the bootstrap installs six things,
  each a download of seconds to a minute; caches are not warmed (D3). If the
  envtest download ever pushes it over, it moves to the verify hook as a
  lazily installed tool.
- **[The environment cache rebuilds after about seven days]** → the bootstrap
  is idempotent and re-runs unchanged; nothing is stored in the environment
  that the tree does not know how to recreate.
- **[A remote session reports a chart change verified when helm was missing]**
  → the verify hook names the missing tool in context before work starts; the
  instruction file requires the pull request to list what was not run.
- **[The routine API is a research preview under a dated beta header]** → the
  header is one line in the workflow; a breaking change fails the fire
  visibly on the issue, and two previous header versions keep working.
- **[Runs count against the account's daily cap and subscription usage]** →
  one label is one run; nothing schedules or retries.
- **[A check fails for a reason not in the tree — a runner outage, a rate
  limit, a flaky download]** → the fixer finds nothing to fix and DISPUTES the
  check with the log's reason; a round of disputes only ends the loop, the
  summary names the job, and a person re-runs the check. Bounded by design,
  and a false "fix" for a flake is what the reproduce-first rule prevents.
- **[A log tail is handed to a model]** → the tail is what the pull request's
  author already reads on the checks tab; GitHub masks registered secrets in
  logs, and the `fix` job holds `contents: read` only, so nothing the log could
  say lets it push.
- **[Two rounds in sequence for one push — the review's and CI's]** → the
  concurrency group serialises them and each collects the live state; the
  second is short when the first left nothing, and both count toward the cap.

## Migration Plan

1. Merge the tree changes (bootstrap, workflow, script, instruction file,
   rules, `.mcp.json`, label vocabulary). Nothing fires yet: the workflow
   needs the variable and the secret.
2. Create the `agent-ops-operator` cloud environment: Full network; variables
   `SONAR_ORG` and `KUBEBUILDER_ASSETS`; the Sonar API credential; the setup
   field `export CLAUDE_CODE_REMOTE=1` then
   `cd /home/user/agent-ops-operator && bash .github/scripts/cloud-bootstrap.sh`.
3. Create the routine on that environment and this repository, prompt per D2,
   add an API trigger, generate the token once.
4. Store `ROUTINE_FIRE_URL` (variable) and `ROUTINE_FIRE_TOKEN` (secret) on
   the repository; create the `autoimplement` label with its description.
5. Label a throwaway issue, watch one session end to end, delete the issue's
   branch and pull request.

**Rollback:** delete the label and disable the routine; the workflow then
never matches and never fires. The bootstrap is inert without the environment.

## Open Questions

- Does the credential proxy override an `Authorization` header the MCP client
  sends, or must the header be absent remotely? Settled in the first remote
  session; either answer is a one-line change to `.mcp.json`.
- Does `create_webhook_trigger` accept an `issues` event source? If it does,
  the fire hop could go; the gate and the record stay in the workflow either
  way, so the answer changes nothing in this change.
- Which Go version does the image ship? The bootstrap verifies and installs
  the floor if needed, so the answer decides only whether that section ever
  does anything.
- For an interactive remote session started on `master`, does the proxy
  accept the first push of a new branch? Recorded in `gotchas.md` when known
  (D4).
