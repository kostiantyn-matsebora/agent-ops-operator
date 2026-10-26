## 1. API: the move mark and the manager origin — api-architect

- [x] 1.1 Add `Moved bool` to `InflightRun` in
      `platform/manager/api/v1alpha1/conversation_types.go`, documented as
      design D-A states (set by an accepted verb, consumed at completion).
      Verify: the field renders in `chart/crds/` after regeneration.
- [x] 1.2 Add `OriginManager OriginKind = "manager"` with the enum marker
      widened to `signal;channel;member;manager`, and update the "three
      doors" doc comment to four. Verify: the CRD enum lists four values.
- [x] 1.3 Regenerate deepcopy and CRDs with the two `controller-gen` lines in
      `.claude/rules/build-test.md`. Verify: `git diff chart/crds/` shows
      only the two additions, and `go build ./...` passes in
      `platform/manager`.

## 2. Manager: the verbs stamp, `/work/done` judges — backend-developer

- [x] 2.1 In `platform/manager/internal/httpapi/coordinate.go`, after
      `InvokeMember`, `CloseCoordinated` and `Escalate` return success, stamp
      the CALLER's `status.inflight.moved` with an optimistic-locked status
      patch retried on conflict (design D-A). A caller with no inflight run
      stamps nothing. A refused verb stamps nothing. Verify: a unit test per
      verb in `coordinate_test.go` asserts the stamp, plus one for attached
      invoke, one for a refused invoke, one for `list_agents`.
- [x] 2.2 In `handleWorkDone` (`server.go`), add the judgement for a
      conversation with `spec.coordinatorRef` set, in the order design D-B
      gives: succeeded, not moved, no consumed input of origin `channel`.
      First miss appends the corrective input (D-C). Second miss (the run's
      consumed inputs are exactly one `manager`-origin input) calls
      `Router.Escalate` with the fixed digest (D-D). A member, a failed run
      and a Closed conversation are never judged. Verify: table-driven tests
      in `workreport_coordinate_test.go` covering every scenario in the
      `coordination-loop` delta spec.
- [x] 2.3 Build the corrective payload from the Coordinator's `agents[]`
      names read at the miss, bounded, with the fixed rule text. Verify: a
      test asserts the roster names appear and the text names the rule.
- [x] 2.4 Confirm the corrective input is delivered to bound threads by the
      existing per-destination rule, attributed to the manager, with no
      new clause. Verify: an envtest in `internal/integration/` shows a
      `manager`-origin input on a channel-bound root produces one send op
      per bound channel.

## 3. Manager: the root dispatch template — backend-developer

- [x] 3.1 Add `platform/manager/internal/dispatch/templates/coordinate.md`
      carrying the input verbatim, who it came from per origin (design
      D-E), and `{{DELIVERY_INSTRUCTIONS}}`. No checkout, no role file, no
      "within your tools" fallback. Verify: a test greps the rendered text
      for the absence of `.claude/agents`, `checkout` and `working
      directory`.
- [x] 3.2 In `dispatch.Next`, select `coordinate.md` for every input type
      when `c.Spec.CoordinatorRef != nil`, keeping `profile.spec.prompt`'s
      precedence and the reply-batching path. Members keep the three
      templates. Verify: `dispatch_test.go` gains one test per origin
      (signal, channel, member, manager) and one asserting a member with
      `causedBy` alone still gets `task.md`.

## 4. Manager: children-first close cascade — backend-developer

- [x] 4.1 Reorder `closeWithCascade` in
      `platform/manager/internal/chat/coordinate.go` to walk children
      (recursively, children-first) before closing the conversation itself,
      returning on the first failure so the ancestor stays open (design
      D-H). Verify: a test in `coordinate_test.go` with a three-level tree
      asserts close order, and one with an injected failure asserts the
      root is still open and a retry resumes.

## 5. aops MCP server: the descriptions — backend-developer

- [x] 5.1 Extend the tool descriptions in `platform/mcp-aops/` so `invoke`,
      `escalate` and `close` state the turn rule and name themselves moves,
      the six reads name themselves reads, and the moves name the
      hand-back-then-escalate consequence (design D-G). Verify:
      `platform/mcp-aops/mcp_test.go` asserts each description contains the
      rule text, and `go test ./...` passes in `platform/mcp-aops`.

## 6. Runtime claude: offered built-ins follow the allowlist — backend-developer

- [x] 6.1 In `runtimes/claude/tools.js`, add a pure function deriving the
      built-in base names from a composed allowlist (every non-`mcp__*`
      entry, scope stripped, deduplicated). Verify: `tools.test.js` covers
      `[]`, `mcp__aops__*` alone, `Read,Grep`, `Bash(git *)`, and a mixed
      list.
- [x] 6.2 In `runtimes/claude/runtime.js` `openSession`, pass that set to
      the SDK as the built-in tools to offer (`tools` option, falling back
      to `disallowedTools` if the pinned SDK lacks it, design D-F), and add
      it to the session fingerprint. `canUseTool` and `mcpServers` are
      unchanged. Verify: `runtime.permissions.test.js` asserts the option
      is passed as derived, and `runtime.sessionreuse.test.js` asserts a
      changed allowlist re-opens the session.
- [x] 6.3 Bump `runtimes/claude`'s version and confirm the SDK option
      name against the pinned `@anthropic-ai/claude-agent-sdk` in
      `package.json`. Verify: `cd runtimes/claude && npm ci && node --test`
      passes.

## 7. Console: the fourth origin — frontend-developer

- [x] 7.1 Render origin `manager` wherever the console labels an input's
      origin (`ConversationRow.tsx` and the chat tree), with a neutral
      "manager" attribution. Verify: `npm run test` in
      `platform/console/ui` passes with one new case per touched renderer.

## 8. Unit tests

- [ ] 8.1 `platform/manager`: `go build ./... && go vet ./... && go test
      ./...` with `KUBEBUILDER_ASSETS` set, covering sections 1 to 4.
- [ ] 8.2 `platform/mcp-aops`: `go build ./... && go vet ./... && go test
      ./...`.
- [ ] 8.3 `runtimes/claude`: `node --test`, including the coverage flags
      from `.claude/rules/build-test.md`.
- [ ] 8.4 `platform/console/ui`: `npm run test:coverage`.
- [ ] 8.5 Chart render tests: `go test ./internal/integration/ -run
      TestCoordinator` from `platform/manager`, confirming the prompt tests
      are untouched by this change (no prompt wording moved).

## 9. System tests

- [ ] 9.1 Extend the coordinator lane in `platform/manager/test/system/`
      (`coordinator_test.go`) with a scripted stub-runtime root that calls
      no verb twice: assert one `manager`-origin input is appended after the
      first run, and `escalatedAt` is set after the second, with the digest
      delivered to the bound console thread. Build the test binary in the
      build container from THIS WORKTREE's tree and run it on the host, as
      `build-test.md` describes.
- [ ] 9.2 Extend the same lane with a root whose run calls `invoke`: assert
      no corrective input is appended and `inflight.moved` is consumed by
      completion.
- [ ] 9.3 Run the full system pack against THIS WORKTREE's chart
      (`--state-values-set chartPath=...`), not the released one. Verify:
      pack green, run id recorded here.

## 10. Documentation

### Reference docs

- [ ] 10.1 `docs/concepts.md`, Coordinated agents: the turn rule, the
      corrective input, the escalation on a second miss, the `manager`
      origin in the provenance table, and children-first close.
- [ ] 10.2 `docs/contracts.md`, the aops section: the move mark on the
      inflight run, which tools are moves and which are reads (settle the
      `read` row's Kind column here), the hand-back consequence.
- [ ] 10.3 `docs/runtimes/claude.md`: the offered built-ins follow the
      composed allowlist, as a form of `RT-3`. `docs/contracts/runtime.md`
      is NOT edited.
- [ ] 10.4 `docs/CHANGELOG.md`: manager, runtime-claude, mcp-aops and
      console entries, the chart version, and the `kubectl apply -f
      chart/crds/` line for the two CRD additions.
- [ ] 10.5 Re-run `python3 .github/scripts/docs-generate.py` for the CRD
      field and enum additions (`docs/crds/conversation.md`). Verify:
      `python3 .github/scripts/docs-generate.py --check` passes.

### Adopter site

- [ ] 10.6 `docs/guides/coordinate-agents.md`: what a person sees when the
      coordinating agent ends a turn without a move, in the thread and on
      escalation, in dense prose.
- [ ] 10.7 Check `docs/getting-started.md`, `docs/introduction.md` and
      `docs/index.md` for any claim that a root closes itself unaided, and
      correct it where found.
- [ ] 10.8 Run the writing lint over every touched page. Verify:
      `python3 .claude/scripts/rules_compliance.py <pages>` is silent.
