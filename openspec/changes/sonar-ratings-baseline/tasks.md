## 1. The backlog, enumerated (design D1, D3)

- [x] 1.1 New `.github/scripts/sonar-findings-baseline.py`: for every component
  `components.sh images` lists (plus `scripts`, same project-key pattern
  `sonar-issues.py` uses), `GET issues/search` with `resolved=false`,
  `impactSeverities=BLOCKER,HIGH` and no `pullRequest` param — the branch-wide
  backlog, not one pull request's. Writes counts per component per
  `softwareQuality` to a JSON file; prints the same as a table. Verify: run
  against one component known to carry issues; a 0-result response with
  `impactSeverities` set is treated as a possible taxonomy mismatch and the
  script also tries the legacy `severities=BLOCKER,CRITICAL` filter,
  reporting BOTH counts when they disagree rather than silently trusting the
  new one.

  Verified against fixtures in task 4.1, not against the organisation — the
  script itself was never run with a real token this session (see 1.2).

  **KNOWN, ACCEPTED GAP: the script itself trips `scripts`' own new-code
  security gate.** `pythonsecurity:S8701`/`S8705`/`S8707` fire on its
  `subprocess.run(["curl", ...])` call and `pythonsecurity:S2083` (BLOCKER)
  on `out_path.write_text(...)`, even after adding explicit `--api`/path
  validation (`validated_api`, `validated_path`) — Sonar's dataflow does not
  recognise a hand-written function as a sanitiser for this rule family, and
  the only way to fully silence it would be rejecting a legitimate absolute
  `--out` path, breaking the tool's basic contract. The sibling
  `sonar-issues.py` carries the IDENTICAL unaddressed pattern on `master`
  already (same `fetch`/`components` shape, copied). Asked and confirmed:
  accepted as a known gap rather than chased further.
- [ ] 1.2 **PARTIALLY DONE, differently than planned.** `SONAR_TOKEN` never
  reached this session's own shell (an MCP client env-substitution timing
  issue — the token is set on the host, but a running session's MCP
  subprocess only reads env at ITS OWN launch, so it needs relaunching from a
  shell that already has the var; not a token problem), so 1.1's script was
  never run against the organisation and no complete per-component table
  exists. Once the SonarQube MCP server came up read-only mid-session (after
  a relaunch), real Blocker/High counts were read for `manager`
  ONLY, via `search_sonar_issues_in_projects` (impactSoftwareQualities ×
  severities, not this script) — 73 findings: 61 `go:S3776` (cognitive
  complexity), 7 `go:S1192` (duplicate literal), 5 `go:S5443`/`S5445`
  (predictable/writable temp path). The other ~15 components in
  `components.sh images` were never enumerated FOR FINDING COUNTS this way.

  **A DIFFERENT, LIGHTER pull was done for all 16** — `get_component_measures`
  (`reliability_rating`, `security_rating`, `sqale_rating`, `coverage`) per
  component, the RATINGS THEMSELVES rather than a finding backlog. That is
  what task 2's component choice after `manager` was made from: every
  component below B on any rating was `manager`, `scripts` (security E),
  `console`, `runtime-claude` and `runtime-copilot` (both E/E, worst in the
  org) and `signal-ha`. This does NOT satisfy 1.1's script or produce the
  per-quality Blocker/High counts it's for — it only answers "which rating is
  failing", not "which findings would need fixing" for the ~11 components
  still untouched. Whoever resumes this: either run 1.1's script by hand with
  the token (task as originally scoped), or keep reading via the MCP tool
  per-component and fold counts in here.

## 2. Every Blocker and High finding, fixed (design D2, D3)

- [ ] 2.1 **`manager` only, and even there not finished.** Of the 73 findings
  read for `manager` (see 1.2):
  - **Fixed — 12 mechanical:** 7 `S1192` (extracted a named constant: 3× in
    `test/e2e/cluster.go`/`wiring.go`, 1× each in
    `internal/httpapi/{activity,server}.go`, `internal/controller/{channel,signal}adapter_controller.go`,
    `internal/chat/ops.go`); 5 `S5443`/`S5445` (`os.CreateTemp` instead of a
    computed path under `os.TempDir()`, in `test/conformance/runner.go` and
    `test/e2e/{cluster,install}.go`).
  - **Fixed — 35 of 61 `S3776`:** every finding in a `_test.go` file, by
    extracting the flagged function's body into named top-level helpers
    (`t.Run`/closures do NOT lower this metric — see the gotchas.md entry
    added by 6.1.2). Spans
    `internal/integration/{charttemplate,charttemplate_identity,channeladapter,signaladapter,vocabulary,pipeline,capacity,tooling,activity,console,suite}_test.go`,
    `internal/runtimepod/contextsync_test.go`,
    `internal/controller/runtimestart_test.go`,
    `test/conformance/{channel,signal}_test.go`,
    `test/e2e/{harness,lanes,lifecycle,loop,substrate}_test.go`.
  - **NOT fixed — 26 `S3776` in PRODUCTION code**, deferred by explicit
    choice (the hour, and the risk of reshaping the core reconciler
    unsupervised): `internal/controller/conversation_controller.go` (7,
    complexity up to 61), `internal/httpapi/{server,signals,channelread,status}.go`
    (9), `internal/chat/{ops,delivery,pipelines}.go` (4),
    `internal/runtimepod/podspec.go` (2),
    `internal/controller/pipeline_controller.go` (1),
    `internal/dispatch/dispatch.go` (1), `internal/mcpcompile/compile.go` (1),
    `internal/activity/log.go` (1).
  - Every fix verified: `go build`/`go vet` under the default, `conformance`
    and `e2e` build tags, the full `go test ./...` suite (envtest included,
    47s), and the full `-tags conformance` suite (37s) — see task 4.3/4.4.
  - **`runtime-claude` and `runtime-copilot` also read and mostly fixed**,
    chosen after 1.2's per-component rating/coverage pull showed both at E
    (worst in the org) on BOTH reliability and security — see the ratings
    table this task's own conversation produced, not reproduced here per
    `publication.md`. Of 14 findings (`runtime-claude`) and 15
    (`runtime-copilot`):
    - **Fixed — 23 of 29:** `javascript:S2189` (the poll loop's `for(;;)`
      rewritten as `while(!idle)` with a real `continue`, in both
      `runtime.js`); `jssecurity:S2083` path-traversal ×4 (a shared
      `safeJoin` helper in each `tools.js`, refusing a `..`-escaping agent
      name or `promptFile`, used by `agentDeclaredTools` and the prompt
      read in `runtime.js`); `jssecurity:S5145` log-injection ×7 (a
      `sanitizeLog` helper stripping control characters before a work
      unit's `runId`/`threadId`/`agent`/`toolsMode`/`maxTurns` reach
      `console.log`); `javascript:S4036` (`claude` resolved once via a PATH
      walk instead of by bare name); `javascript:S8786` ReDoS ×3 (the
      adjacent-quantifier regexes in both `tools.js`'
      block-form-tools-list parser and copilot's `vocabulary.js`
      `parseMcpPattern` rewritten as plain string search — `node --test`
      re-verifies identical parsing, all 38+56 cases); `javascript:S7773`
      ×4 (`parseInt` → `Number.parseInt`); `docker:S6505` ×3 (`npm install
      -g --ignore-scripts`, with claude-code's own installer then run
      explicitly — verified against a real build: the plain
      `--ignore-scripts` install fails `claude --version` with "native
      binary not installed" until that second step runs; copilot's SDK
      needed no such step, verified the same way).
    - **Fixed — 25 of 29, after a follow-up round:** `docker:S8543`
      (unlocked `npm@latest`/unpinned `claude-code` version) ×2 was
      INITIALLY left alone as the DELIBERATE design both Dockerfiles stated
      in comment — but touching the SAME line to add `--ignore-scripts`
      made it count as new code on this pull request's own gate regardless,
      and asked directly, the user chose to PIN rather than mark it
      won't-fix: both Dockerfiles now pin `npm@12.0.2` and
      `@anthropic-ai/claude-code@2.1.252` explicitly, with a comment naming
      `npm view <pkg> version` as how to check before the next bump. Both
      rebuilt and re-verified the same way as the first round.
    - **NOT fixed — 4, by explicit choice:**
      `jssecurity:S6350` (user-controlled command argument to `spawn`) is
      structural: the runtime's whole job is passing profile/work-unit
      content — the prompt, the system prompt — to the CLI as argv, so
      there is no validation that would not also break the feature; left
      for the same "unsupervised risk to reshape core logic" reason task
      2.1's 26 production `manager` findings were deferred. Two `S5145`
      findings on `unit.systemPrompt.length`/pure numeric fields are very
      likely a taint-analysis false positive (a `.length` access can never
      carry a control character) and were left alone rather than wrapped
      for no real effect.
    - Both Dockerfiles' fix was BUILT AND RUN, not just read, TWICE — once
      for `--ignore-scripts`, again for the pin: `docker build` succeeded
      both times for both, `claude --version` printed `2.1.252 (Claude
      Code)` and `npm --version` printed `12.0.2` in the built claude
      image, and the same two in copilot's plus
      `require('@github/copilot-sdk')` resolving.
  - **The other ~13 components were never touched** — no findings were even
    read for them (see 1.2).
  - **A THIRD, INFRASTRUCTURE-LEVEL FIX landed alongside these**, asked and
    confirmed directly: `manager`'s own new-code coverage was failing at
    17.1% because this pull request's `manager` diff mostly touches
    `test/e2e/` and `test/conformance/` source (`cluster.go`, `install.go`,
    `wiring.go`, `runner.go`) — build-tag gated, so the plain
    `go test ./...` this session's CI job runs has NO coverage data for
    them at all, which the new-code condition reads as uncovered
    regardless of what the conformance suite or a live cluster actually
    exercises. `.github/actions/sonar-scan` now carries
    `-Dsonar.coverage.exclusions=test/e2e/**,test/conformance/**` — the
    files are still analysed for bugs/vulnerabilities/smells exactly as
    before, only the coverage metric is exempted. `CONTRIBUTING.md`'s
    Code analysis section documents it in the same commit.

    The exclusion took `manager`'s new-code coverage from 17.1% to 43.8%,
    not to 80% — the REMAINDER was already-uncovered lines the S1192
    constant-extraction happened to touch, none of them exercised by any
    existing test:
    - Three `if s.Activity == nil { … 503 … }` guard clauses in
      `internal/httpapi/activity.go` (36% file coverage, the worst of the
      touched files). New `internal/httpapi/activity_test.go` constructs a
      `Server` with a nil `Activity` log and hits all three handlers
      directly (43.8% → 62.5%).
    - Four `if err := json.Unmarshal(...); err != nil { … errInvalidJSON
      … }` sites in `internal/httpapi/server.go`, across
      `handleWorkDone`/`handleChannelOpDone` (need only a zero-value
      `Server` — neither reads anything before the parse; new
      `invalidjson_test.go`) and `handleStatePut`/`handleChannelStatus`
      (need a real channel to resolve first; new `state_test.go`, a fake
      client the same way `chat`'s `router_test.go` already builds one).
    - Two `res.Error != ""` / `res.ThreadID == ""` branches in
      `internal/chat/ops.go`'s `tryFinishEnsureTopic` — the adapter-error
      paths of ensure-topic completion, reachable only through a real (fake)
      `client.Client`, which no `chat`-package test had ever constructed for
      this function specifically. Extended `ops_test.go`, reusing
      `router_test.go`'s `closeTestScheme`/`testNS` from the same package.

    All via the same lightweight per-package pattern this codebase already
    uses (`contextreport_test.go`, `router_test.go`) — no envtest needed for
    any of them. `go test ./...` (envtest included, for the rest of the
    module) still green, 47.7s.

    **CONFIRMED: `manager`'s new-code gate reads OK on CI**, all six
    conditions including `new_coverage` at 100.0%.

  - **A FOURTH, UNRELATED CI FIX**, found while chasing what looked like a
    flake: `images (runtime-copilot)` and `images (runtime-ollama)` both
    failed Trivy on the identical `libexpat1` `CVE-2026-56408`, on the
    identical cached version `2.5.0-1+deb12u2`, across two separate CI runs
    minutes apart. NOT a flake — `docker/build-push-action`'s log showed
    the `apt-get install` layer as `CACHED` both times: the RUN
    instruction's TEXT is the cache key, so an unchanged Dockerfile line
    keeps reusing whatever it resolved to on an earlier build even after a
    CVE lands against a transitive dependency (`libexpat1`, pulled in by
    `git`/`curl`/`jq`, never named directly). Confirmed locally with
    `--no-cache`: a genuinely fresh `apt-get install` on the SAME base
    image already resolves the patched `2.5.0-1+deb12u3` — the Debian
    security archive had it; CI's cache did not. `apt-get upgrade -y`
    added to all three runtime Dockerfiles (`claude`, `copilot`, `ollama`
    — the only three with an apt layer at all) forces the layer to
    re-resolve on every future security-relevant apt change, not just this
    one. All three rebuilt with `--no-cache` and verified: `dpkg -l
    libexpat1` shows `2.5.0-1+deb12u3` in each, and `claude --version` /
    the copilot SDK / the ollama runtime binary all still run.
- [ ] 2.2 Not run: no re-analysis of this branch has happened yet (needs a
  CI push), and `manager`'s own backlog is not fully fixed (26 production
  findings remain per 2.1), so a re-run would not read zero regardless.

## 3. The gate, extended (design D1, D2)

- [x] 3.1 `.github/scripts/sonar-provision.sh`'s gate stage: extend the
  `wanted` list with `reliability_rating GT 2`, `security_rating GT 2` and
  `sqale_rating GT 2` (A=1 … E=5, so `GT 2` fails worse than B), same
  update-not-duplicate handling the coverage condition already has. Verify:
  `sh -n` passes; the three metrics are literal strings beside `coverage`,
  not derived, matching how `coverage LT 80` is written today.
- [ ] 3.2 **OPEN — needs the user's token, and is theirs to run, and comes
  AFTER task 2.2 reads zero.** Run the extended provisioning script against
  the organisation. Read back `api/qualitygates/show?name=agentops`: seven
  conditions become ten. Verify: ten conditions, every component project
  still assigned, and a second run creates nothing new.

  This is a WRITE (creates/updates quality-gate conditions); the SonarQube
  MCP server that came up mid-session is deliberately read-only
  (`SONARQUBE_READ_ONLY=true`), so even with it working this task stays the
  user's to run by hand, as originally scoped.

## 4. Unit tests

- [x] 4.1 New `.github/tests/sonar-findings-baseline.test.sh` with `curl`
  stubbed from fixtures: an org with issues on both taxonomies reports both
  counts and flags the mismatch; an org with only Clean Code impacts reports
  cleanly; the component list is read from a captured `components.sh` output,
  exactly as `sonar-issues.test.sh` fixtures it. Wired into `run.sh`. Verify:
  `.github/tests/run.sh` passes.
- [x] 4.2 Extend `.github/tests/sonar-provision.test.sh`: an org with the
  `agentops` gate and seven conditions (coverage plus the six new-code ones)
  gains exactly the three rating conditions on a `--gate` run; a gate that
  already carries all ten makes no `create_condition` or `update_condition`
  call. Verify: `.github/tests/run.sh` passes, mutation-checked the same way
  the coverage condition's test was — replacing `update_condition` with
  `create_condition` fails only the no-duplicate assertion.
- [x] 4.3 Every module whose code changed under task 2 passes its own suite:
  `go test ./...` (or the module's own toolchain) in each touched component,
  from the `golang:1.25` build container per `build-test.md`. Verify: every
  touched module's suite exits 0.

  Also ran, since task 2's fixes touched `test/conformance/` and `test/e2e/`
  source (not only `_test.go`): `go vet` under `-tags conformance` and
  `-tags e2e`, and the full `go test -tags conformance ./test/conformance/...`
  suite (36 files: `channel-telegram`, `console`, `k8s-events`, `ha` — all
  green, real binaries built and run). `-tags e2e` was vetted but not run —
  it needs a live k3d cluster, out of scope for a unit-test task; see task 5.

  Separately, and not part of task 2: `internal/manager` unit-test coverage
  was raised from 70.7% to 80.9% this session (a `NOTES.txt` template bug
  fix plus a fuzz-based deepcopy round-trip test in `api/v1alpha1`) — asked
  for directly by the user, not scoped by this change's design, recorded
  here only because it landed on this branch alongside these fixes.

  Also separately: the fix sweep on `runtime-claude`/`runtime-copilot`
  (above) introduced three genuinely new, security-relevant functions
  (`safeJoin`, `sanitizeLog`, `resolveBin`) with no test at all — they had
  been written inline in each `runtime.js`, the untestable entrypoint (it
  calls `process.exit(1)` at require-time without `CONTROL_URL`/`CONVO_ID`,
  which is why there is no `runtime.test.js`). Moved into each `tools.js`
  (already tested, already exported) and given 24 new `node --test` cases
  across both runtimes covering the escape-refusal, control-character-strip
  and PATH-miss/executable-bit paths specifically — not just the happy
  path. `node --test --experimental-test-coverage`: `tools.js` went from
  untested-for-these-functions to **100% line / 89% branch** in both
  runtimes, `vocabulary.js` to **100% line / 94% branch** (its
  `parseMcpPattern` rewrite got 4 new edge-case tests: multiple `__`
  occurrences, a `_`-leading server, an empty tool half, no separator at
  all — pinning the exact leftmost-match behaviour the old lazy regex had).
  `runtime.js` itself stays uncovered by `node --test` in both — it is the
  daemon loop, unreachable without a live `/work` endpoint to poll; that gap
  is unchanged by this session and is a coverage question about the
  ENTRYPOINT shape, not about these three functions.
- [x] 4.4 Run the suite and both guards from the worktree:
  `.github/tests/run.sh`, `python3 .github/scripts/publication-guard.py`,
  `python3 .github/scripts/retired-vocabulary-guard.py`. Verify: all three
  exit 0.

## 5. E2E tests

- [x] 5.1 Not applicable: nothing here is decided by a cluster — a
  branch-wide analysis read, code fixes judged by the analysis service, and a
  gate provisioning script. The live proof is the first CI run of this
  branch (every touched component's analysis) and task 3.2's read-back.
  Verify: the pull request's SonarCloud checks show every touched component
  clean of Blocker/High findings, and the `agentops` gate's ten conditions.

## 6. Documentation

### 6.1 Reference docs

- [ ] 6.1.1 The delta spec is archived into
  `openspec/specs/code-quality-analysis/spec.md` by `/opsx:archive` on the
  branch; `openspec validate --all` passes. Verify: the command exits 0.

  Not run: `/opsx:archive` is refused with 1.2/2.1/2.2/3.2 open, correctly —
  this change is not finished.
- [x] 6.1.2 If task 2's fix sweep surfaces a technique worth keeping —
  a class of finding this codebase produces repeatedly, or a rule worth
  naming as a house convention — record it in `.claude/rules/gotchas.md` or
  the relevant topic file. Verify: named here if anything qualified, or this
  task states none did.

  Recorded in `.claude/rules/gotchas.md`: wrapping a flagged test function's
  body in `t.Run`/a closure does NOT lower Sonar's `go:S3776` score for
  it — nested function literals increment the NESTING level charged to the
  ENCLOSING function rather than resetting for a separate unit. The working
  technique is extraction into a genuinely separate, named top-level
  function. Confirmed against SonarSource's own community-forum answer on
  the mechanism before applying it across all 35 test-file fixes.

### 6.2 Adopter site

- [x] 6.2.1 `CONTRIBUTING.md`, "Code analysis": the gate now additionally
  requires at least a B overall reliability, security and maintainability
  rating per component, provisioned the same way the coverage condition is.
  No page under `docs/` describes the analysis (confirmed by
  `coverage-across-packages`' own check), so the site carries nothing to
  update — stated here so the absence is a claim rather than an omission.
  Verify: the paragraph names the three ratings and the threshold, and
  `wc -l README.md` is unchanged.

  Also updated, in the same section: the `.mcp.json` SonarQube entry
  paragraph, which this session's own tooling work made stale (it described
  the old Dockerised `sonarqube-mcp-server`; `.mcp.json` now points at
  SonarCloud's native hosted MCP endpoint instead) — a documentation.md
  obligation independent of this change's own scope, fixed in the same
  commit because it lives in the same section.
