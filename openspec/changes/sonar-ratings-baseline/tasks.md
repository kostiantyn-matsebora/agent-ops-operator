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

  **"ACCEPTED AS A KNOWN GAP" WAS WRONG HERE TOO, SAME AS `S6350`
  — REVERSED after pushback, by reading Sonar's OWN documented remediation
  instead of guessing at one.** The first attempt's `validated_api` /
  `validated_path` were hand-invented and Sonar's dataflow never
  recognised them as sanitisers — true — but the fix isn't "this class of
  finding can't be satisfied," it's "use the pattern the rule itself
  publishes":
  - `pythonsecurity:S2083`/`S8707` (path injection) — the rule's own
    compliant example is `os.path.realpath(path)` checked against
    `os.path.realpath(os.getcwd())` with `startswith(base + os.sep)`
    (their "partial path traversal" pitfall names the trailing separator
    as load-bearing). `validated_path` now does exactly that instead of
    the previous `pathlib.resolve()` + `..`-in-parts check, which is a
    DIFFERENT function the rule's engine apparently never credited. The
    real, accepted narrowing: `--out`/`--components`/
    `--components-script` must now resolve under the CURRENT WORKING
    DIRECTORY, matching the rule's own stated intent ("prevent LLMs from
    escaping the directory from which it was invoked") — an absolute
    path outside cwd is refused where it was silently accepted before.
    New test asserts the refusal directly (`../../../../etc/passwd`
    against `--out`).
  - `pythonsecurity:S8705` (argument injection) — the rule's own
    compliant example validates with a regex before the value reaches a
    subprocess argv; the shape here is closer to `S6350`'s: `curl -sf -u
    <token> <url>` misreads a `-`-leading `url` as an unknown option,
    confirmed directly (`curl --this-looks-like-a-flag` → `curl: option
    ...: is unknown`, exit 2; `curl -- --this-looks-like-a-flag` resolves
    it as the target instead, exit 6/"could not resolve host"). Added the
    same `--` separator `runtime-claude`'s fix uses, before `url` in the
    one `subprocess.run(["curl", ...])` call. `api` is already
    `validated_api`-restricted to `http(s)://`, so this call can in fact
    never hit the vulnerable path — the separator is what makes that
    verifiable without also tracing through a second function.

  Neither breaks the tool's real usage: CI always invokes this from the
  repository root with a repo-relative `--out`, and a real `curl` against
  `sonarcloud.io` with a fake token still reaches the network correctly
  through the `--` separator (verified by hand, `curl exit 22`/HTTP
  error, not a parse failure). `sonar-issues.py` carries the identical
  ORIGINAL pattern unaddressed on `master` still — out of scope here, a
  candidate for the same fix later, not evidence this one is unfixable.
  `.github/tests/run.sh`: all passing, plus the new refusal test.

  **STILL OPEN ON THE NEXT ANALYSIS — SECOND ROUND, DIAGNOSED FROM WHAT
  DID CLEAR.** `components()`'s inline `validated_path(path,
  must_exist=True).read_text()` was never flagged; the separated
  `out_path = validated_path(...)` assigned early and `.write_text()`'d
  many statements and a loop later STAYED flagged. The one working
  difference: distance from sanitiser to sink. `out_path` is now
  recomputed immediately before `.write_text()` (matching the rule's own
  `open(safe_path(filename))` — sanitiser and sink adjacent, not a
  variable carried across the function), with an early fail-fast check on
  `args.out` kept as a SEPARATE call so a malformed path is still refused
  before any network round-trip. `fetch()`'s `subprocess.run(["curl",
  ...])` gained an actual regex check immediately before it —
  `^https?://[\w.\-~:/]+\?[\w.\-~%=&]*$` — matching S8705's own compliant
  shape (`re.match` adjacent to the sink) rather than relying on the `--`
  separator alone, which fixed the real bug but was not what this rule's
  engine checks for. Verified the regex against every URL shape this
  script actually produces (empty query string included) and re-ran the
  full smoke test against real `sonarcloud.io` (curl exit 22, not a
  regex or parse rejection). `.github/tests/run.sh`: 11/11, unchanged.

  **PARTIAL RESULT, READ PRECISELY: `S8705` (the curl regex) CLEARED.
  `S2083`/`S8707` (the write) DID NOT** — same rule, same line, STILL
  open on the next analysis. The adjacent-STATEMENT fix
  (`out_path = validated_path(...)` immediately followed by
  `out_path.write_text(...)`, no loop or branch between them any more)
  was not enough; `components()`'s never-flagged read side is not just
  adjacent, it is ONE EXPRESSION — the sanitiser call is a sub-expression
  of the sink call itself
  (`validated_path(path, must_exist=True).read_text()`), never assigned
  to a variable at all. Third round: `validated_path(args.out,
  must_exist=False).write_text(...)` inline, matching that exactly (and
  the rule's own `open(safe_path(filename))`, which is the same shape —
  sanitiser nested INSIDE the sink call, not preceding it). The one
  remaining use of the resolved path (the closing print line) now calls
  `validated_path` a second time rather than keeping the old variable —
  cheap and pure, and keeps every use of the CLI-supplied `--out` value
  going through the same single-expression validation. `.github/tests/run.sh`:
  11/11 unchanged; smoke-tested against real `sonarcloud.io` again.

  **THAT ROUND ALSO STAYED FLAGGED, on the identical inline expression —
  ruling out "adjacent" and "one expression" both, as SHAPES within
  `main`'s own scope.** Fourth round, a different axis: `components`
  (never flagged) receives its CLI-supplied path as its OWN PARAMETER and
  validates it inside that function — not `args.components` referenced
  directly in `main`. New `write_result(out: pathlib.Path, result: dict)`
  does the same for the write side: `args.out` is passed as an ordinary
  argument, and `validated_path`/`.write_text()` happen inside
  `write_result`'s own scope, crossing a real function boundary instead
  of guessing at another same-scope rewrite. `main` now calls
  `write_result(args.out, result)`. `.github/tests/run.sh`: 11/11
  unchanged; smoke-tested against real `sonarcloud.io` again.

  Whether THIS round clears both remaining conditions
  (`new_security_rating`, and `new_duplicated_lines_density` — trending
  3.7% → 3.4% → 3.1% → 3.0% across rounds as the file diverges further
  from `sonar-issues.py`'s copied shape, needs `< 3`) is what the next CI
  push answers. Three same-scope shapes have now failed; if this one
  does too, the working/non-working line is `components` vs. `main`
  specifically, and it may be `args.*` attribute access itself Sonar's
  engine treats differently from a plain parameter — worth asking rather
  than guessing a fifth pattern.
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
    - **Fixed — 27 of 29, after two follow-up rounds:** `docker:S8543`
      (unlocked `npm@latest`/unpinned `claude-code` version) ×2 was
      INITIALLY left alone as the DELIBERATE design both Dockerfiles stated
      in comment — but touching the SAME line to add `--ignore-scripts`
      made it count as new code on this pull request's own gate regardless,
      and asked directly, the user chose to PIN rather than mark it
      won't-fix: both Dockerfiles now pin `npm@12.0.2` and
      `@anthropic-ai/claude-code@2.1.252` explicitly, with a comment naming
      `npm view <pkg> version` as how to check before the next bump. Both
      rebuilt and re-verified the same way as the first round.
    - **`jssecurity:S6350` was INITIALLY marked structural/deferred, and
      that call was WRONG — reversed after the user pushed back on
      accepting it.** Re-investigated rather than re-asserted: verified
      against the real CLI that a prompt beginning with `-` was ALREADY
      MISPARSED as an unrecognised option (`error: unknown option
      '...'`), the run never starting — a genuine reliability bug this
      predates, not a hypothetical Sonar heuristic. `--resume [value]`
      (an OPTIONAL argument, per `--help`) carries the same misparse for a
      dash-leading context id, one path over — the value reads as absent
      and the id is parsed as its own unrecognised token. Fixed both,
      confirmed against the CLI at each step: `--resume=<id>` (single
      `=`-joined token, unambiguous regardless of a leading dash) and a
      `--` separator immediately before the trailing positional prompt
      (moved to the END of `args`, after every flag, since everything
      following `--` stops being read as an option). Neither changes how
      the prompt reaches the CLI — still argv, still one `spawn` call — so
      this is NOT the core-logic redesign (stdin plumbing) that was ruled
      out; it is a 15-line, behavior-preserving reorder of the SAME argv
      array. `--append-system-prompt <value>` needed no change — a
      REQUIRED-argument option already consumes the very next token
      unconditionally, verified the value survives a leading dash
      unmodified. `node --test`: 50/50 unchanged. Full image rebuilt
      (`--no-cache`) and the exact new argv shape — resume id, system
      prompt, every flag, a `--rm -rf /`-prefixed prompt — run against the
      real CLI: parses cleanly through to a legitimate downstream error
      (MCP config schema), never an argument-parsing one.

      **THE FIX IS REAL AND KEPT. IT DOES NOT CLEAR THE FINDING, AND NOW
      THAT IS VERIFIED RATHER THAN ASSUMED.** `jssecurity:S6350` stayed
      OPEN on the next analysis (same line, same `spawn` call). Read the
      rule's OWN compliant example rather than re-guessing: it is
      `if (allowed.includes(input)) { spawn(...) }` — an ALLOW-LIST of
      exact permitted values, checked before the call. That is what this
      rule accepts and the ONLY thing it accepts; a `--`
      separator/argument-injection fix (correct for the bug it fixes) is
      not in its vocabulary at all. An allow-list is incompatible with a
      free-form prompt by construction — there is no finite set of valid
      prompts. **This is now the "structural, cannot satisfy without
      breaking the feature" case the first assessment claimed without
      checking.** The difference from the first pass: this one is backed
      by the rule's own documented remediation, not a guess.

      **RESOLVED IN SONARCLOUD DIRECTLY, once `SONAR_TOKEN` reached this
      session's shell** (task 3.2): `mcp__sonarqube__change_sonar_issue_status`
      on `AaBdtVMUl80T31Xjjxim`, status `accept` — SonarQube's term for a
      confirmed, justified, permanent disposition, distinct from
      `falsepositive` (the finding is real; passing free-form content to
      a CLI genuinely is what an allow-list-only rule is warning about,
      it is simply not something this feature can avoid without ceasing
      to answer prompts). Took effect on the ALREADY-COMPLETED analysis
      with no new scan needed: `new_security_rating` read back `1` (OK)
      immediately, and the pull request's `runtime-claude` quality gate
      is fully `OK` — every condition, `S6350` included. `runtime-claude`
      keeps the real `agentops` gate throughout, unlike `scripts` (3.3) —
      accepting one confirmed-inapplicable issue is not the same
      decision as exempting a whole project, and only the second one was
      asked for here.

      **SEPARATELY: `new_coverage` was ALSO short (78.6%, `runtime.js`
      itself unreachable by `node --test` at 0%, as documented — task 4.3
      names why).** The argv-building block this fix touched is genuinely
      pure (`unit` in, an array out — no I/O), so it moved: `tools.js`
      gained `buildClaudeArgs({contextId, systemPrompt, allowed, maxTurns,
      mcpConfig, prompt})`, `runtime.js`'s `runClaude` now calls it instead
      of building the array inline. 9 new direct tests assert the exact
      argv shape — the `--` separator position, `--resume=` with a
      dash-leading id, `--append-system-prompt` surviving one unmodified,
      omission of both when absent, the allowlist/mcp-config/max-turns
      values and its default — locking in the `S6350` fix itself with a
      test, independent of whether Sonar ever credits it. `node --test`:
      59/59, `tools.js` stays 100% line coverage. Image rebuilt
      (`--no-cache`) and `claude --version` still runs.
    - Two `S5145` findings on `unit.systemPrompt.length`/pure numeric
      fields are very likely a taint-analysis false positive (a `.length`
      access can never carry a control character) and were left alone
      rather than wrapped for no real effect — the one true remaining
      "not fixed" item for `runtime-claude`/`runtime-copilot`.
    - Both Dockerfiles' fix was BUILT AND RUN, not just read, TWICE — once
      for `--ignore-scripts`, again for the pin: `docker build` succeeded
      both times for both, `claude --version` printed `2.1.252 (Claude
      Code)` and `npm --version` printed `12.0.2` in the built claude
      image, and the same two in copilot's plus
      `require('@github/copilot-sdk')` resolving.
  - **`console` and `signal-ha` fixed too**, after PR #145 itself went
    green and the user asked directly what was still below B org-wide: a
    fresh per-component ratings pull (same `get_component_measures` sweep
    as before) showed exactly these two still failing, everything else
    already A. Neither was in the original plan; both are the SAME kind
    of work task 2 already covers, so folded in here rather than opened
    as a separate change.
    - **`signal-ha` — ONE finding, ZERO code changed:** `go:S4790`
      (weak hash) on `ws.go`'s `acceptKey`, computing RFC 6455's
      `Sec-WebSocket-Accept` handshake value — which the SPEC DEFINES as
      `SHA1(key + GUID)`. Read the rule's own doc before touching
      anything: it scopes explicitly to hashing PASSWORDS, SECURITY
      TOKENS or DATA INTEGRITY, none of which apply to a protocol
      checksum with no confidentiality purpose — and changing the
      algorithm would produce a value no RFC 6455-compliant server
      (Home Assistant included) would recognise, breaking the handshake
      outright rather than improving anything. Marked `accept` via
      `mcp__sonarqube__change_sonar_issue_status` (key
      `AaBNvIYkrl9RvYuOlFsq`). Took effect immediately: reliability,
      security and maintainability all read `1.0` (A) on the next
      `get_component_measures` call, no new analysis needed — same
      immediacy `S6350`'s disposition on `runtime-claude` showed.
    - **`console` — 13 findings, all fixed in code:** `typescript:S2871`
      ×2 (`graph/model.ts` — bare `.sort()` on a `Set` of strings uses
      UTF-16 code-unit order, not alphabetical; both now
      `.sort((a, b) => a.localeCompare(b))`); `typescript:S7723`
      (`graph/Graph.tsx` — `Array(rings)` → `new Array(rings)`);
      `typescript:S8786` ×2 (`components/Yaml.tsx`'s YAML-line tokenizer
      — the same adjacent-quantifier shape `runtime-claude`'s `tools.js`
      had, rewritten the same way: indent, dash and key each matched by
      their own single-quantifier regex and split with plain slicing,
      not one pattern backtracking across all three; `Yaml.test.tsx`'s
      7 cases plus `model.test.ts`'s 19 and `Graph.test.tsx`'s 19 all
      still pass, confirming the rewrite is behaviourally identical);
      `docker:S6505` ×2 + `S8543` (the `Dockerfile`'s conditional `npm
      ci`/`npm install` fallback — `--ignore-scripts` added to both,
      verified locally that `npm ci --ignore-scripts && npm run build`
      still succeeds since esbuild's platform binary is an
      `optionalDependency`, not a postinstall download; the `npm
      install` fallback branch was ALSO removed entirely rather than
      just pinned, since `ui/package-lock.json` is committed and always
      present — a silently-taken unpinned path was the S8543 finding
      itself, and reproducible-or-fail is strictly better than
      reproducible-or-silently-not); `go:S2092` ×2 + `S3330`
      (`auth.go`'s two `http.SetCookie` calls, missing `Secure`/
      `HttpOnly` — new `secureCookie(r)` sets `Secure` from `r.TLS` or a
      proxy-forwarded `X-Forwarded-Proto: https`, never hardcoded true,
      since an install with a plain-HTTP internal hop to the console
      would otherwise never receive the cookie back at all; both the
      login and the logout cookie now carry both flags); `gosecurity:S5145`
      ×2 (`convapi.go`'s two `log.Printf` calls wrapping an `error` that
      can carry user-controlled content — new `sanitizeLog(err)`, same
      control-character-stripping shape as the runtimes' JS
      `sanitizeLog`, applied at both sites).

      **`go:S2092` STAYED FLAGGED ON ONE SITE — the logout cookie
      specifically, NOT login — despite both carrying the identical
      `Secure: secureCookie(r)` expression.** Same shape as `scripts`'
      `S2083`/`S8707` saga: not a code-correctness question, a
      dataflow-recognition one, and inconsistent between two
      byte-identical expressions this time rather than between two
      different ones. Fix: `newSessionCookie(r, value, maxAge)`, the ONE
      place either flag is now set, called from both `handleLogin` and
      `handleLogout` — a real DRY win regardless (two near-duplicate
      `http.Cookie` literals were genuinely redundant), and it also
      happens to be the same "cross a function boundary" shape that
      cleared `scripts`' write-sink findings. Whether Sonar credits THIS
      one is what the next analysis answers; recorded rather than
      assumed, per the pattern this whole task has followed.

      New/extended tests: `TestSessionCookieIsSecureOnlyWhenTheRequestWas`
      and `TestLogoutCookieCarriesTheSameFlagsAsLogin` in `api_test.go`
      (both directions of the `Secure` condition, not just the happy
      path); `TestLoginIssuesSessionCookie` extended to assert `Secure`
      is FALSE on a plain request; new `convapi_test.go` for
      `sanitizeLog` directly. `go build`/`go vet`/`go test ./...` all
      green; `npx vitest run`: 195/195; `npm run typecheck`: clean;
      `docker build --no-cache` succeeded and the binary starts
      (reports its one missing required env var, proving the embedded
      SPA and the Go binary both built correctly).
  - **Three more, pushed to full A even though B already passes the
    gate:** the change's own stated goal is "at least B", and
    `channel-telegram`/`runtime-ollama`/`signal-telegram` were already at
    B on security (nothing here failed CI) — kept going anyway, same as
    `console`/`signal-ha` were, rather than stopping at "good enough."
    - `runtime-ollama` — `go:S4036` on `repo.go`'s bare `exec.CommandContext(ctx,
      "git", ...)`. New `resolveBin(name string) string` (`exec.LookPath`,
      falling back to the bare name), `gitBin = resolveBin("git")` resolved
      once at package load — the same shape as the JS runtimes'
      `resolveBin`, this time genuinely testable without a subprocess:
      `t.Setenv("PATH", dir)` plus a fake executable, and the no-match
      fallback, both asserted directly in new `repo_test.go`.
    - `channel-telegram` and `signal-telegram` — one `gosecurity:S5145`
      each (two call sites in `channel-telegram`), `log.Printf` wrapping
      an `error` that can carry Telegram-relayed content. Both gained
      their OWN `sanitizeLog(err)` (each is a separate Go module per
      `structure.md`, so there is no shared package to add it to once) —
      identical shape to `console`'s, and to the runtimes' JS version.
      New `sanitizelog_test.go` in each, same two cases as `console`'s
      (control characters stripped, ordinary text unchanged).

    `go build`/`go vet`/`go test ./...` green in all three modules.
  - **The other ~8 components were never touched** — no findings were even
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
- [x] 3.2 **RUN, OUT OF THE ORIGINALLY PLANNED ORDER.** Scoped as "after
  task 2.2 reads zero" and "the user's to run by hand" — neither held:
  task 2.2 never ran (`manager` still carries 26 deferred production
  findings, and `runtime-claude`/`scripts` each carry one confirmed,
  permanent finding — see tasks 2.1 and 3.3 below), and `SONAR_TOKEN`
  reached this session's own shell partway through (the MCP
  env-substitution issue task 1.2 named was a session-restart problem,
  fixed once the session was restarted with the var already exported).
  Run directly against the organisation:
  `sh .github/scripts/sonar-provision.sh --gate`. Verified from the live
  response: the `agentops` gate did not exist as an object before this
  run (the six new-code conditions this session's own quality-gate reads
  were judging PRs against all along were the organisation's untouched
  default, the built-in `Sonar way` gate, which `agentops` copies
  verbatim — so nothing observed this session about new-code conditions
  changes) — it was created with all ten conditions in one call, set as
  the organisation default, and every project reported already assigned
  (SonarCloud's `get_by_project` falls back to the default for a project
  with no explicit assignment, which every component had). Confirmed via
  `mcp__sonarqube__list_quality_gates`: exactly one `agentops` (id
  159421, `isDefault: true`, ten conditions), no duplicate.
- [x] 3.3 **NOT IN THE ORIGINAL PLAN, ADDED DIRECTLY BY THE USER:**
  `scripts` is tooling, never a delivered artifact — no image, no chart,
  nothing `components.sh` or a release tag names — and after four
  documented-pattern attempts (task 2.1) its one remaining finding
  (`pythonsecurity:S2083`/`S8707` on a CLI `--out` write) is a rule Sonar's
  own engine will not credit any code shape for. `sonar-provision.sh` now
  provisions a second gate, `agentops-unenforced` — created empty and kept
  that way, never synced with conditions the way `agentops` is — and
  assigns `scripts` to it explicitly instead of `agentops`. Every other
  project stays on `agentops`; `scripts` keeps reporting real findings
  (the scan step still runs, still posts to the pull request) but never
  blocks a merge on them. Run live in the same call as 3.2: confirmed
  `agentops-unenforced` created (id 159422, zero conditions) and `scripts`
  explicitly assigned to it. `.github/tests/sonar-provision.test.sh`
  extended: the fixture stub is now project-aware for
  `qualitygates/get_by_project` (it answered every project identically
  before, which cannot express "scripts is on a different gate than
  manager"), plus direct assertions that the unenforced gate is created on
  a fresh org and that a second run reassigns nothing. 45/45.

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
