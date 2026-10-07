## 1. Word-chain and name-allocation helper — backend-developer

- [ ] 1.1 Create `platform/manager/internal/namewords/words.go` with `Words(text string) string`: splits on punctuation and `CamelCase` boundaries, drops the fixed stopword list, keeps the first three remaining words, lowercases, joins with `-`, caps at 24 characters by dropping whole trailing words. Verify with table-driven unit tests covering punctuation, CamelCase, stopwords, and over-length truncation (from `conversation-naming/spec.md` scenarios).
- [ ] 1.2 Add `AllocateName(ctx, client, kind, words string) (name string, err error)` to the same or a sibling package: lists conversations labeled `agentops.dev/name-base: <kind>-<words>`, computes the next free name (bare base, or base plus next integer suffix), and returns it for the caller to set on `ObjectMeta.Name` and `ObjectMeta.Labels["agentops.dev/name-base"]`. Verify with an envtest-backed unit test covering first-allocation, a recurring base name, and a simulated create conflict retried within the bounded attempt count (5), and exhaustion of those attempts returning the conflict error.
- [ ] 1.3 Define the 24-char length constant and the small fixed stopword list as documented Go constants/vars in the new package, with the doc comment on the length constant stating that it is chosen against the longest derived name (`agentops-mcp-conv-` + `member-` + 24 + a 4-character suffix), not the bare 63-char DNS label ceiling. Verify by reading the generated godoc.

## 2. Wire the three creation sites — backend-developer

- [ ] 2.1 In `platform/manager/internal/httpapi/signals.go`, widen `titleForGroup`'s fallback (`"🔍 " + source.Name`) to apply to `chat`/`task` kinds too, not only `alert`/`job`. Verify with a unit test asserting a `chat`-kind signal with empty title and payload falls back to the source name.
- [ ] 2.2 In the same file, replace `createConversationForGroup`'s `GenerateName` assignment with a call to `namewords.Words` over the already-computed title, followed by `AllocateName`, setting both `Name` and the `name-base` label. Verify with a unit test asserting the created conversation's name matches `conversation-naming/spec.md`'s alert/job/chat/task scenarios.
- [ ] 2.3 In `platform/manager/internal/chat/router.go`'s `CreateTaskConversation`, do the same using the addressed pipeline name as the word source. Verify with a unit test asserting the `task-<pipeline>` name scenario.
- [ ] 2.4 In `platform/manager/internal/chat/coordinate.go`'s `createMember`, do the same using the invoked `agents[]` entry name as the word source. Verify with a unit test asserting the `member-<entry>` name scenario, and a second invoke of the same entry producing the `-2` suffix.

## 3. Unit tests

- [ ] 3.1 Run `go test ./...` in `platform/manager` and confirm all new and existing tests pass, including the envtest-backed allocation and creation-site tests added in sections 1 and 2.

## 4. E2E tests

- [ ] 4.1 Nothing here is decided by a cluster beyond what envtest already covers in section 1 — name allocation is ordinary List/Create against the Kubernetes API, with no kubelet, RBAC, or pod-lifecycle behavior involved. No e2e pack lane is added.

## 5. Documentation

### Reference docs
- [ ] 5.1 Add a short note to `docs/concepts.md`'s Conversation section describing the word-chain naming convention (object names are now built from context, not a random suffix) and the `agentops.dev/name-base` label.
- [ ] 5.2 Add an entry to `docs/CHANGELOG.md` describing the non-breaking change in conversation object-name shape.

### Adopter site
- [ ] 5.3 Confirm no adopter-site page (landing page, Introduction, Getting started, Installation, guides) describes conversation naming today, so none needs an update — checked by searching those pages for `generateName`, `member-`, or `alert-` before closing this task.
