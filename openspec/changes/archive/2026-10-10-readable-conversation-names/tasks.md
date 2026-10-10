## 1. Word-chain and name-allocation helper — backend-developer

- [x] 1.1 Create `platform/manager/internal/namewords/words.go` with `Words(text string) string`: splits on punctuation and `CamelCase` boundaries, drops the fixed stopword list, keeps the first three remaining words, lowercases, joins with `-`, caps at 24 characters by dropping whole trailing words.
- [x] 1.2 Add `AllocateName(ctx, client, namespace, kind, words string, obj *Conversation) (name string, err error)` to `internal/namewords/allocate.go`: lists conversations labeled `agentops.dev/name-base: <kind>-<words>`, computes the next free name (bare base, or base plus next integer suffix), sets `obj.Name` and `obj.Labels["agentops.dev/name-base"]`, and creates it. A create conflict retries within the bounded attempt count (5).
- [x] 1.3 Define the 24-char length constant and the small fixed stopword list as documented Go constants/vars in the new package, with the doc comment on the length constant stating that it is chosen against the longest derived name (`agentops-mcp-conv-` + `member-` + 24 + a 4-character suffix), not the bare 63-char DNS label ceiling.

## 2. Wire the three creation sites — backend-developer

- [x] 2.1 In `platform/manager/internal/httpapi/signals.go`, the source-name fallback now applies to every kind, not only `alert`/`job` — via a new `wordsForGroup` helper rather than `titleForGroup` itself, since a one-shot payload can be non-empty text (an emoji-only caption, a non-ASCII question) with no word `namewords.Words` can extract. `titleForGroup`'s own return is untouched, so `spec.Title` still shows the real caption or non-ASCII text rather than silently swapping in the source name — caught by `TestChatConversationIsTitledByTheMessage`'s long-Cyrillic-title case regressing when the fallback was first wired into `titleForGroup` directly.
- [x] 2.2 In the same file, replaced `createConversationForGroup`'s `GenerateName` assignment with a call to `wordsForGroup` (which calls `namewords.Words`), followed by `AllocateName`, setting both `Name` and the `name-base` label.
- [x] 2.3 In `platform/manager/internal/chat/router.go`'s `CreateTaskConversation`, same shape, word source is the addressed pipeline name (`origin.GetName()`).
- [x] 2.4 In `platform/manager/internal/chat/coordinate.go`'s `createMember`, same shape, word source is the invoked `agents[]` entry name (`entry.Name`).

## 3. Unit tests

- [x] 3.1 Table-driven unit tests for `namewords.Words` covering punctuation, CamelCase, stopwords, and over-length truncation (`internal/namewords/words_test.go`).
- [x] 3.2 Envtest-backed unit tests for `AllocateName` covering first-allocation, a recurring base name, a genuine concurrent-creator race (several goroutines racing one base name, all succeeding with distinct suffixes), and exhaustion of the bounded retry on a persistent conflict returning the conflict error (`internal/integration/namealloc_test.go`).
- [x] 3.3 Unit tests asserting a `chat`-kind signal with an empty payload, and one with a payload carrying no significant word (emoji/punctuation only), both fall back to the source name for the object NAME while `spec.Title` keeps the real message (`internal/integration/naming_test.go`).
- [x] 3.4 Unit tests asserting `createConversationForGroup` names match `conversation-naming/spec.md`'s alert/job/chat scenarios, plus a chat message that does carry words (`internal/integration/naming_test.go`).
- [x] 3.5 Unit test asserting `CreateTaskConversation` yields the `task-<pipeline>` name (`internal/chat/naming_test.go`).
- [x] 3.6 Unit test asserting `InvokeMember`/`createMember` yields `member-<entry>`, and a second invoke of the same entry name from a DIFFERENT caller yields the `-2` suffix — the SAME caller would instead attach to the live member per the existing (parent, entry) reuse rule, so the suffix case needs two callers (`internal/chat/naming_test.go`).
- [x] 3.7 `go test ./...` in `platform/manager` passes, including the envtest-backed allocation and creation-site tests (`KUBEBUILDER_ASSETS` from the session's envtest install).

## 4. E2E tests

- [x] 4.1 Nothing here is decided by a cluster beyond what envtest already covers in section 3 — name allocation is ordinary List/Create against the Kubernetes API, with no kubelet, RBAC, or pod-lifecycle behavior involved. No e2e pack lane is added.

## 5. Documentation

### Reference docs
- [x] 5.1 Added a note to `docs/concepts.md`'s Conversation section describing the word-chain naming convention, a table of each creation path's word source, and the `agentops.dev/name-base` collision label.
- [x] 5.2 Added an entry to `docs/CHANGELOG.md` (Unreleased → Changed) describing the non-breaking change in conversation object-name shape.

### Adopter site
- [x] 5.3 Searched the adopter-site pages for `generateName`, `member-`, `alert-`: no page describes conversation naming today (the only hits are unrelated — `alert-investigator`/`alert-triage` profile/pipeline names in `integrations/prometheus.md`, `member-result` in `guides/coordinate-agents.md`). No adopter-site page needs an update.
