## 1. Spikes against pi's SDK — backend-developer

Each ends in a written finding under `design.md`'s Spikes section. No code in
sections 2 to 4 depends on an answer until its spike is recorded.

- [x] 1.1 Pick the SDK version and package scope, install it in a scratch
  directory, and record the exact version, the scope and the SDK entry points
  the runtime will use. Verify: the finding names the pinned version and the
  runtime's `npm ci` resolves it from a lockfile.
- [x] 1.2 S1: enable a chosen set of built-in tools for a session, and find
  out whether a per-call hook exists. Verify: a script starts two sessions,
  one with `read` only and one with none, and the finding records the
  registered built-in names against design D2's table.
- [x] 1.3 S2: find the system-prompt option, whether it appends or replaces,
  and whether it applies on a resumed session. Verify: the finding quotes the
  resulting prompt from a scripted session, before and after a resume.
- [x] 1.4 S3 and S4: create and resume a session by a caller-chosen id, and
  place the state root under a given `$HOME`. Verify: the finding lists the
  paths a session writes, which become `contextSync.paths`, and says whether
  the runtime needs its own id-to-session mapping file (design D5).
- [x] 1.5 S5: read per-turn usage, cache counts and stop reason from the
  events. Verify: the finding maps each to a `turns[]` field and names the
  ones pi does not report.
- [x] 1.6 S6: determine whether pi loads `AGENTS.md` or `CLAUDE.md` from the
  working directory by itself, and whether that can be turned off. Verify:
  a scratch checkout with such a file, and a transcript showing the outcome.
- [x] 1.7 S7: point pi's provider base URL at a fake endpoint. Verify: a
  scripted run completes against a local fake, or the finding states it
  cannot and 8.4 is struck with that reason.

## 2. The runtime — backend-developer

- [x] 2.1 Create `runtimes/pi/` with `package.json` (exact pin, `node
  >=22.19.0` — the SDK's own declared floor per spike 1.1, above this task's
  original `>=22.12` guess) and its lockfile, and the entry file that
  long-polls `GET /work`, runs a unit, posts `/work/done` and exits `0`
  after `RUNTIME_IDLE_TTL_M`, ported from `runtimes/copilot/runtime.js`.
  Verify: `node --test` in `runtimes/pi` runs the ported main-loop test.
- [x] 2.2 Fail at start on missing `CONTROL_URL`, `CONVO_ID` or credential
  with a nonzero exit naming the gap (`LC-4`). Verify: a test per variable
  asserts the exit code and the message.
- [x] 2.3 Clone `REPO_URL`/`REPO_REF` with the environment's git credentials
  and skip cleanly on an empty `REPO_URL` (`RT-1`, `MGR-5`), ported from
  copilot's clone code and its tests. Verify: the ported clone, SSH and HTTPS
  tests pass.
- [x] 2.4 `tools.js`: read `.claude/agents/<agent>.md`, return its `tools:` and
  its body, and compose the allowlist per `toolsMode`. Absent, unreadable or
  unparseable contributes nothing and never blocks (`RT-3.3`). Verify: tests
  for `merge`, `overwrite`, an empty composition, a missing file and bad
  frontmatter.
- [x] 2.5 `vocabulary.js`: map the composed allowlist to pi's built-in names
  per design D2. A scoped shell pattern enables nothing and is logged, an
  unrecognised pattern contributes nothing and is logged, and every mapped
  name is checked against the session's registered inventory at start.
  Verify: table tests for each built-in, a scoped `Bash(...)`, an unknown
  pattern and an inventory gap.
- [x] 2.6 Build the system prompt: the definition's body, then the unit's
  `systemPrompt`, on every run including resumes, using the option S2
  found. Verify: a test asserts the composed prompt on a first run and on a
  resume.
- [x] 2.7 Answer every permission decision from the composed allowlist, deny
  by default, with no path to an interactive prompt. Verify: a test drives a
  denied call and asserts the run continues and the denial is logged.
- [x] 2.8 `continuity.js`: mint `runtimeContextId`, always the runtime's own
  id. When S3 found pi cannot take a caller-chosen id, map it to pi's session
  by a file in the state directory. Create or resume the session from it.
  Verify: tests for a first run, a resume and the mapping file.
- [x] 2.8a On a failed resume, re-check the state with bounded delays. Treat
  an unreadable path as present and retry once. A confirmed-absent context
  fails with `continuity: unavailable` and a reason naming the context
  volume. Verify: tests for a slow store, an unreadable path and a lost
  context, ported from `runtimes/copilot/continuity.test.js`.
- [x] 2.9 Apply the S6 finding: if pi loads workspace context files by
  itself, turn that off, or record the behaviour for the runtime's page.
  Verify: a test with an `AGENTS.md` in the checkout asserts the prompt the
  session received.

## 3. The MCP extension — backend-developer

- [x] 3.1 `mcp.js`: read `MCP_CONFIG`, treating an absent file as no servers
  and an unreadable or unparseable one as one logged failure, and translate
  each entry, stdio and HTTP, as `runtimes/copilot/mcp.js` does. Verify: the
  ported translation tests pass.
- [x] 3.2 Expand `${ENV}` placeholders in-process. A missing one fails that
  server alone with a logged reason, and no value reaches a log line, the
  transcript or the result. Verify: tests for a resolved value, a missing
  one, and a log capture asserting the secret never appears.
- [x] 3.3 Connect to the listed servers only, list their tools, and register
  each as `mcp__<server>__<tool>` as a pi custom tool, only when the composed
  allowlist permits it. A `mcp__<server>__*` pattern permits that server's
  tools and no other's. Verify: tests for a named tool, a server wildcard, a
  server nobody bound, and no configuration at all.
- [x] 3.4 Ship the extension inside the image and load only it. Verify: a
  test asserts the session loads no other extension, and the image's file
  list in 5.1 names it.

## 4. Telemetry — backend-developer

- [x] 4.1 `report.js`: build `turns[]` and `toolCalls[]` from the SDK's
  events, trimmed to the published bounds, with tool names in the agent-ops
  vocabulary, no tool input or output, an omitted figure where pi reports
  none, and neither array when no call was made (`API-4` to `API-9`).
  Verify: ported bounds and omission tests from
  `runtimes/copilot/report.test.js`, plus an event fixture from S5.
- [x] 4.2 Deliver the report with bounded retry on transient failure, no
  retry on a definitive `4xx`, and a log line carrying status and body for
  every undelivered report (`API-10` to `API-12`). Verify: the ported
  delivery tests pass.

## 5. The image and its release — deployment-engineer

- [x] 5.1 `runtimes/pi/Dockerfile` from copilot's: the same base, the
  `APT_REFRESH` layer, `npm ci` from the lockfile, the npm CVE-pin block, no
  domain tooling, and `HOME=/data/context`. Verify:
  `.github/components.sh images` lists `agentops-runtime-pi`, and a local
  `docker buildx build --platform linux/amd64,linux/arm64` of the directory
  succeeds. Component discovery and a `linux/amd64` build were verified from
  this remote session. The sandbox has no privileged mode/binfmt_misc, so the
  `linux/arm64` leg of the local build was not performed here. The
  Dockerfile is architecture-neutral (no native binaries, confirmed), and
  CI's own multi-arch build is the gate that actually proves it, per
  `remote-session.md`.
- [x] 5.2 Run the image scan locally the way CI does and clear fixable
  CRITICAL and HIGH findings, pinning as copilot does. Verify: the scan
  reports none. Run against the `linux/amd64` build above, via the same
  Trivy invocation `.github/actions/trivy-scan` uses — zero CRITICAL/HIGH.
- [ ] 5.3 Publish by tag from the merged tree,
  `runtime-pi-v0.1.0`. Verify: check the registry for both architectures,
  not the tag, and flip the package's Actions access and visibility once.

## 6. Chart: the `pi` bundle — deployment-engineer

- [x] 6.1 Create `chart/charts/pi/` with `Chart.yaml` 0.1.0, `values.yaml`
  (`enabled: false`, `name: pi`, `default: false`, `image`, the credential
  Secret reference, provider and model env, `contextSync.paths` from S4), and
  `templates/runtime.yaml` calling `agentops.renderRuntime`. Verify:
  `helm template` of the bundle on renders one `AgentRuntime` and nothing
  else.
- [x] 6.2 Add `agentops.piRuntimeEntry` to `chart/templates/_helpers.tpl` and
  `pi` to `agentops.declaredRuntimes`, so the default-runtime guard and the
  bootstrap env see it. Verify: the existing guard test still passes.
- [x] 6.3 `chart/Chart.yaml` gains the dependency with `condition:
  pi.enabled`, and `chart/values.yaml` the documented `pi:` section beside
  `copilot:`. Verify: `helm lint` passes and a default render is
  byte-identical to the render before this change.
- [x] 6.4 Bump the chart minor. Verify: `docs-generate.py --check` accepts the
  new version once section 10 is done.

## 7. The specification — api-architect

The user approved this edit to `docs/contracts/runtime.md` for this change.

- [x] 7.1 `RT-3.4`: name `runtimes/pi` among the runtimes reading
  `.claude/agents/<agent>.md`. Move the version to 2.0.2 and add the
  changelog row, stating it clarifies and changes no obligation. Verify: the
  page's version, its changelog and every shipped runtime page's claimed
  version agree.
- [x] 7.2 Walk the 35-item conformance checklist for `runtimes/pi`, one
  verdict per item with the test or finding that backs it, kept in this
  change's `design.md`. Verify: all 35 have a verdict and none is "assumed".
  Found and fixed in cross-role review: item 7 (`API-7`) claimed an
  unqualified "Pass, tested", but task 8.4's own live conformance finding
  (`report.js`'s zero-vs-omitted guarantee holds for `reasoningTokens`, not
  for the four usage counters once `ModelRuntime` default-fills a missing
  wire value to 0) was never carried into this table's verdict. Reworded to
  "Pass for `reasoningTokens`, vendor-forced partial for the other four",
  with the live finding as evidence — the checklist now states the same
  narrower claim the test and the code comment already do.
- [x] 7.3 Move the claimed version on the four existing runtime pages to
  2.0.2. Verify: a grep for the old version in `docs/runtimes/` returns
  nothing.

## 8. Unit tests — testing-specialist

- [x] 8.1 `cd runtimes/pi && node --test` passes, covering sections 2 to 4.
  Verify: the run is green and coverage is collected with the flags
  `build-test.md` gives. The bare default-glob invocation hangs in this
  sandbox (the same orphaned-child-under-concurrent-spawn artifact section
  4 already hit), reproduced and confirmed by `ps aux` leaving nothing
  behind. Naming the suite's 11 files explicitly to the same `node --test`
  invocation does not hang: 129 tests, 128 pass, 1 expected skip, 98.45%
  line coverage. CI's own invocation is unaffected, since it runs outside
  this sandbox.
- [x] 8.2 Chart render tests: defaults byte-identical, the bundle on renders
  the runtime under its own name and its Secret when a token is supplied,
  `default` is still a copy of the reference runtime, and the bundle on with
  the reference off makes pi the default. Verify: the extended
  `internal/integration/charttemplate_test.go` passes with `helm` present,
  and `serviceaccount-guard.py` still passes.
- [x] 8.3 `.github/components.sh images` and `modules` list the new
  component and every other module's build, vet and test pass. Verify: the
  discovery loop from `build-test.md` is green. `runtime-pi` is discovered
  by both, correctly on `modules`'s exclusion side (a Node runtime, not a Go
  module). Two pre-existing failures outside this change's touched
  directories — `platform/console` (`ui/dist` never built in this
  environment) and `platform/context-sync` (two checkpoint tests) — are
  unrelated to `runtime-pi` (confirmed via `git diff`, neither directory has
  a change on this branch) and are not this task's to fix.
- [x] 8.4 Conformance lane, if S7 found a fake provider is possible: build
  the real runtime and run it as a subprocess against `FakeManager` in
  `platform/manager/test/conformance/`, asserting the same runtime-contract
  IDs the ollama lane asserts. Verify:
  `go test -tags conformance -count=1 -v ./test/conformance/` passes with the
  pi lane present. If S7 said no, this task is ticked with the reason. S7
  said yes — `runtimepi_test.go`/`fakepiprovider.go` drive the real
  `runtime.js` as a subprocess against a ported Go SSE double. Found and
  fixed in review: a doc comment in `report.js` overstated the omission
  guarantee for token counts (true for `reasoningTokens`, not for the four
  usage counters once `ModelRuntime` default-fills a missing wire value to
  0) — corrected, not left as a documented gap.
- [x] 8.5 The documentation gates that read code: `docs-generate.py --check`,
  `retired-vocabulary-guard.py` and `publication-guard.py` pass. Verify: each
  exits zero. `retired-vocabulary-guard.py` and `publication-guard.py` do.
  `docs-generate.py --check` exits 1 on the chart version bump
  (`docs/installation.md` still prints 19.0.1), exactly the gap task 6.4
  already named as owed to section 10, not a new regression.

## 9. System tests — testing-specialist

- [x] 9.1 Add a pi lane to `platform/manager/test/system/`: a route with
  `runtimeRef: pi` answers a signal and every bound thread receives it.
  Verify: the lane passes against the cluster, run from this working copy's
  tree and not the default branch's. WRITTEN
  (`TestPiRuntimeRouteAnswersAndDeliversToEveryBoundThread`), on the stub
  image per `TestImagePullThroughAuthenticatedRegistry`'s own precedent, not
  run — no cluster in this remote session (`remote-session.md`). Compiles
  (`go test -tags system -c ./test/system/`, exit 0) and `go vet -tags
  system ./...` is clean, independently re-verified.
- [x] 9.2 In that lane, assert the pod's shape: the `context-sync` sidecar
  and the egress init container are present, and the session state is under
  the declared path. Verify: the lane reads the pod spec and the volume.
  WRITTEN (`TestPiRuntimePodShapeHasContextSyncAndEgress`), not run, same
  gap as 9.1. Reads `pod.Spec.InitContainers` and `kubectl exec ... test -e`
  against the live state file.
- [x] 9.3 Context continuity: a second run resumes the same context, and
  deleting the session state fails the next run with the cannot-be-continued
  message rather than answering fresh. Verify: both outcomes asserted.
  WRITTEN (`TestPiRuntimeContextContinuityAndLoss`), not run, same gap.
  Widens this lane's own `IdleTTLMinutes` to 5 so all three runs land on one
  pod, with the reasoning recorded in the test's own comment.
- [x] 9.4 Tools: with observation-only toolsets the shell is absent on one
  route while the same profile keeps it on another, and a bound
  `Bash(kubectl:*)` grants no shell and is logged. Verify: both asserted.
  WRITTEN (`TestPiRuntimeToolsetsNarrowAndScopeTheShell`), not run, same
  gap. No `runtimes/pi` vocabulary mapping exists at this tier (stub image),
  so `test/stubruntime`'s own `wiring` directive was extended, test-only, to
  echo the manager's composed `allowedTools`/`MCP_CONFIG` back and log a
  scoped-pattern narrowing as a stand-in — the real per-tool mapping stays
  pinned at the unit tier (`runtimes/pi/vocabulary.test.js`). Re-verified:
  `cd test/stubruntime && go build ./... && go vet ./... && go test ./...`
  all pass, including the two new stub tests.
- [x] 9.5 MCP: a bound `MCPConfig` makes its tools reachable and an unbound
  server's are not. Verify: both asserted through the lane's stub server.
  WRITTEN (`TestPiRuntimeMCPBindingReachesTheMountedConfigAndExcludesTheUnbound`),
  not run, same gap. Proves the manager's `internal/mcpcompile` delivers a
  bound server's key into the exact `MCP_CONFIG` file a real pod mounts,
  and withholds an unbound one, while real MCP tool-call reachability stays
  pinned at the unit and conformance tiers (`runtimes/pi/mcp.test.js`,
  `test/conformance/runtimepi_test.go`'s section-8.4 lane) — no new MCP
  server double built here, since that would duplicate rather than add
  coverage.
- [ ] 9.6 Run the pack. A remote session records this as dispatched, not
  done, per `remote-session.md`. Verify: the pack's result for the pi lane.
  Left unticked until dispatched — the integrating session dispatches the
  cluster smoke-test tier once every section is complete (step 6 of
  `implement-issue.md`), named in the pull request rather than run here.

## 10. Documentation

Both halves, ticked separately. This section is last on purpose and the
archive hook checks it.

### Reference docs

- [x] 10.1 `docs/contracts/runtime.md` is done in 7.1. Here, update
  `docs/contracts.md` and `docs/concepts.md`: the fifth runtime, the
  per-runtime definition path, the tool-enablement posture each runtime
  takes, and the pinned versions. `docs/concepts.md` edited (agent-definition
  path, `toolsets.mode` note, context-sync-paths enumeration, all three
  places now naming pi). `docs/contracts.md` checked and needs NO edit — it
  is entirely vendor-neutral (grepped `runtime`/`claude`/`ollama`/`version`,
  no vendor named, no version pinned), recorded here rather than silently
  skipped.
- [x] 10.2 `docs/configuration.md`: the `pi` bundle's values. Added the `pi:`
  link to the bundle list, a `pi:` values block (`enabled`, `provider`,
  `model`, `credentialsSecret.token`) and a `pipelines[]` example naming
  `runtimeRef: pi`.
- [x] 10.3 `docs/CHANGELOG.md`: an Added entry for the bundle and image,
  newest first, with the image tag it ships and no upgrade steps. Added as a
  bullet under the existing `## [Unreleased]` / `### Added` — chart 19.1.0 is
  not yet tagged, and `docs/CLAUDE.md`'s own format is one heading per
  VERSION, not per change.
- [x] 10.4 `.claude/rules/runtime-spec.md`, `.claude/rules/structure.md` and
  `.claude/rules/wiring.md`: the shipped-runtime lists and the
  "three shipped runtimes" wording, and the definition path being per-runtime.
  Verify: `python3 .claude/scripts/rules_compliance.py` over the three is
  silent. `runtime-spec.md`'s conformance-carries-every-shipped-runtime list
  and `structure.md`'s `runtimes/` table/count both now name `runtimes/pi`.
  `wiring.md` checked (grepped every runtime name and "definition path"/
  "agents/<") and needs NO edit — it names no specific runtime anywhere.
  Verified silent: `python3 .claude/scripts/rules_compliance.py
  .claude/rules/runtime-spec.md .claude/rules/structure.md
  .claude/rules/wiring.md` — no output.
- [x] 10.5 Re-run `python3 .github/scripts/docs-generate.py` and commit
  every regenerated block, since a chart value and a runtime page changed.
  Run for real (not `--check`) after adding the `pi` entry to
  `docs-generate.py`'s own `BUNDLES` dict (required `provider`/`model`, both
  added to `DOCUMENTED_PLACEHOLDERS`) — wrote `docs/assets/img/runtimes/
  pi-{light,dark}.svg` and filled `pi.md`'s `renders bundle=pi` block. Also
  had to fix a pre-existing, unrelated gap this run surfaced first:
  `docs/installation.md` still named chart `19.0.1` while `chart/Chart.yaml`
  is `19.1.0` (the version check runs BEFORE the write step, so it silently
  blocked every other regeneration) — bumped both install-command lines.
  `docs-generate.py --check` now exits 0 (73 generated files up to date).

### Adopter site

- [x] 10.6 New `docs/runtimes/pi.md` in the copilot page's shape: what it
  executes, what it needs, where its context lives, what it costs, the
  tool-enablement posture and the `renders bundle=pi` marker, claiming
  specification 2.0.2. Add its diagram spec to `RUNTIME_DIAGRAMS` in
  `.github/scripts/docs_diagrams.py`. Written, modeled line-for-line on
  `copilot.md`'s sections, with pi's own facts (multi-provider,
  `.claude/agents/<agent>.md`, no scoped-shell narrowing, session resumed by
  minted id rather than held open). Diagram spec added (`"pi"` key, `dir:
  "runtimes"`) and rendered.
- [x] 10.7 `docs/_data/nav.yml`: one entry in the Runtimes group. Before the
  new page links the pi project, add its host to
  `.github/publication-allowlist.json` as a linked upstream project, never by
  loosening a rule. Verify: `publication-guard.py` passes. Nav entry added
  (no icon — no pi mark exists under `assets/img/logos/`, and one was not
  fabricated). The page links the npm package page
  (`npmjs.com/package/@earendil-works/pi-coding-agent`) — `npmjs.com` is
  ALREADY in `publication-allowlist.json`'s hostname allowlist (the
  "TOOLCHAIN, VENDOR AND STANDARDS HOSTS" section), so no entry was added
  there. `docs-generate.py`'s own SEPARATE, stricter `ALLOWED_HOSTS` set
  (the published-page audit) did need `www.npmjs.com` added, with a comment
  citing the same reasoning — found by actually running the generator, not
  assumed. `publication-guard.py` and `docs-generate.py --check` both exit 0.
- [x] 10.8 The pages that list the shipped runtimes: `docs/introduction.md`,
  `docs/index.md`, `docs/installation.md` and `docs/guides/agent-runtime.md`.
  First three updated (the "Two/Three are already shipped" count, the
  "Claude Code, Ollama and GitHub Copilot" sentence, the bundle-values
  table) — also where the chart-version gap from 10.5 was fixed.
  `agent-runtime.md` checked and left as-is: its two named examples
  (Ollama/Copilot) illustrate two DISTINCT backend patterns ("its own
  harness" vs "a vendor SDK translated"), and pi is a second instance of the
  SAME pattern Copilot already illustrates, so naming a third example there
  teaches nothing new — a deliberate omission, not an oversight.
- [x] 10.9 `README.md`: the runtime list only. Verify: `wc -l README.md` is
  at most 215. Three spots updated (`Works with` logo strip, `Any model you
  can run` bullet, the runtime link row). Final: 215 lines exactly.
- [x] 10.10 Run the prose check from `docs/CLAUDE.md` over the touched pages
  and build the site to look at the new page. Verify: the check is silent
  and the page renders with its diagram in both colour schemes. The prose
  check (`.claude/scripts/rules_compliance.py`, the same check) is silent
  over every touched file. The site was built with the documented
  Docker/Jekyll process (network+CA-bundle flags added for this sandbox's
  egress proxy) — build succeeds, `runtimes/pi/index.html` renders with no
  unresolved Liquid, the diagram `<img>` resolving to the real
  `pi-light.svg` asset, 6 tables, and the nav/installation/index pages all
  link `/runtimes/pi/` correctly. A LIVE BROWSER SCREENSHOT in both colour
  schemes was not taken — serving the build over HTTP in this sandbox was
  refused by the environment's own auto-mode classifier (starting a
  background server), so this is recorded as a workstation-only gap per
  `remote-session.md` rather than claimed done.
- [x] 10.11 `docs-generate.py --check`, `retired-vocabulary-guard.py` and
  `publication-guard.py` pass. Verify: each exits zero. All three
  independently re-run after every edit above, each exiting 0:

  | Check | Result |
  |---|---|
  | `docs-generate.py --check` | "73 generated file(s) up to date" |
  | `retired-vocabulary-guard.py` | "clean (147 files)" |
  | `publication-guard.py` | "clean" |

## Cross-role review (`/opsx:apply` step 6.6)

One round, three dispatches, each a role reviewing sections it did not
write. Every finding was independently re-verified against source before
acting, and every real finding was fixed on the branch, none dropped.

- **testing-specialist, sections 1-6**: found a genuine production bug —
  `runtimes/pi`'s `tool_call` permission hook (`runtime.js`'s
  `buildResourceLoader`) checked only `grant.builtinNames`, never the MCP
  tool names `mcp.js` actually registers, so every MCP tool call pi
  attempted was denied regardless of what a Pipeline's `MCPToolset`/
  `MCPConfig` granted. Fixed (`decide([...grant.builtinNames,
  ...mcpHandle.toolNames], ev.toolName)`), with a new regression test
  (`runtime.live.test.js`) verified to fail without the fix and pass with
  it.
- **api-architect, section 10**: found `chart/charts/pi/values.yaml`
  documented a dead env var (`PI_LOG_LEVEL`, nothing in `runtimes/pi/*.js`
  reads it) and omitted two real, functional ones (`PI_PROVIDER_API`,
  `PI_BASE_URL`). Fixed in `values.yaml`'s comment and in
  `docs/runtimes/pi.md`'s `pi.env` row and a new explanatory note. A third,
  explicitly non-blocking note (imprecise shared reasoning for why
  `auth.json`/`models-store.json` both stay empty) stands as written —
  the reviewer flagged it as not a defect.
- **backend-developer, sections 7-9**: re-ran every gate itself rather than
  trusting the stated verdicts (all green). Found one real finding — this
  change's own `design.md` conformance table overclaimed item 7 (`API-7`)
  as an unqualified "Pass, tested," when task 8.4's own live finding (the
  usage-counter zero-fill is vendor-forced, true only for
  `reasoningTokens`) was never carried into the table's verdict. Fixed in
  7.2 above and in `design.md`'s own row.
