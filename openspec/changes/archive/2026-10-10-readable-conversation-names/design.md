## Context

See `proposal.md` - Why. Three call sites set
`Conversation.ObjectMeta.GenerateName` today. Each uses a bare kind-word
prefix and no slug:

- `httpapi.createConversationForGroup` (`platform/manager/internal/httpapi/signals.go`), for `alert` / `job` / `chat` / `task` root conversations. It already computes a title via `titleForGroup` before setting `spec.Title`. That same text is the natural word source.
- `chat.CreateTaskConversation` (`platform/manager/internal/chat/router.go`), for a `/<pipeline> <task>` chat command.
- `chat.createMember` (`platform/manager/internal/chat/coordinate.go`), for a Coordinator `invoke`. Its word source is the invoked `agents[]` entry name itself, not the text `memberTitle` derives from it.

`spec.Title` is a separate, pre-existing field and is out of scope. See
`conversation-naming/spec.md`.

## Goals / Non-Goals

**Goals:**
- Every conversation's object `Name` reads as words, never a random or hash-derived token, at any scale of recurrence.
- One shared, unit-tested word-chain function used by all three call sites.
- Collision handling that costs a fixed, small number of API calls regardless of how many prior conversations share a base name.
- No change to Conversation's spec fields, the CRD, or any HTTP contract.

**Non-Goals:**
- Renaming existing conversations, or backfilling a label onto names already generated.
- A `kubectl` printer column for `spec.title` (orthogonal, separate change if wanted).
- Changing how `spec.title` itself is computed, beyond widening one existing fallback tier (below). `titleForGroup` is otherwise read for word input, not altered. `memberTitle` is not a word source.

## Decisions

**Word-chain function location: a new `internal/namewords` package, not a
method on an existing type.**

All three call sites that need it sit in different packages (`httpapi`,
`chat`) with no existing shared import between them for this purpose.

A tiny, dependency-free package (`platform/manager/internal/namewords/words.go`)
avoids an import-cycle risk. It reads as what it is: a pure string
transform, not domain logic.

**Word extraction: split on punctuation and `CamelCase`, drop stopwords,
keep first three.**

Splitting purely on punctuation would leave `NodeDown` as one blob.
Prometheus-style alert names are almost always PascalCase, so splitting
`CamelCase` boundaries too is what makes `node-down` instead of `nodedown`
— directly serving the "essence of the conversation" goal rather than a
character-truncated fragment.

A small fixed stopword list (`a`, `an`, `the`, `of`, `on`, `in`, `to`,
`and`, `is`, `do`, `please`, …) is dropped before picking the first three
remaining words. This is a constant slice, not configuration — nothing
tunes it per install.

Alternative considered: character-length truncation of the raw text
(the original draft of this design). Rejected after the essence of the
conversation got lost mid-word (`migrat` instead of `migration`), and after
the result stopped reading as a sentence.

**Length cap: 24 characters for the joined word-chain, dropping whole
trailing words to fit.**

24 is chosen against the longest known fixed prefix derived from a
conversation name, `agentops-mcp-conv-` (18 chars), plus the longest kind
prefix used here (`member-`, 7 chars with its dash), plus room for a
`-NN` collision suffix.

That totals under the 63-char DNS label ceiling that bounds the tightest
known consumer of a name-derived string — a label value, should the
conversation name ever be copied into one.

No consumer does that today, but 24 keeps headroom rather than assuming
that stays true.

This is a plain Go constant, matching how `MaxRecordedInputText` and other
internal caps are already constants.

**The empty-word-source gap is closed at the source, not routed around.**

`titleForGroup`'s existing third tier — fall back to `"🔍 " + source.Name`
— today only fires for `alert`/`job` (non-one-shot kinds). This change
widens the same tier to `chat`/`task` too, so a caption-less sticker, voice
note, or photo still has a `SignalSource` name to build a word-chain from.

Alternative considered: give the naming function its own separate "no
words at all" fallback — keep `kind-` alone, as an earlier draft of this
design did.

Rejected once asked directly: it reintroduces a no-information name for a
case that `titleForGroup` can already avoid entirely, one tier up, for
every other kind.

**Collision resolution: a `name-base` label plus a list-then-create, never
sequential probing and never a random suffix.**

Every conversation gets an immutable label, `agentops.dev/name-base:
<kind>-<words>`. Creating a new conversation for that base name:

1. Lists conversations carrying that exact label value.
2. Reads the highest numeric suffix already in use (none found means the
   bare base name itself is free).
3. Creates with the base name, or the base name plus the next integer
   suffix.

This costs one List and one Create call no matter whether it is the 2nd or
the 2000th conversation sharing that base.

The alternative — probing `base`, `base-2`, `base-3`, … one Create attempt
at a time — would cost one failed API call per prior conversation for a
long-recurring alert.

Alternative considered, and explicitly rejected on request: falling back
to Kubernetes' own `GenerateName` random suffix once a small
sequential-retry budget is exhausted. This was the original design here.

It was rejected because it still produces an opaque name, just in a less
common case. The label-based lookup removes the reason to reach for it at
all: cost no longer scales with history, so there is nothing left for a
random fallback to bound.

**A create conflict (a genuine concurrent-creator race) retries the same
list-then-create step, bounded to a small fixed number of attempts.**

This is the one place a retry loop remains, and it exists only for an
actual simultaneous write race, not for volume.

The bound (a small constant: 5) exists so a stuck informer cache or a
true hot loop fails loudly rather than spinning. It is not a tool for
absorbing scale, which is the lookup's job.

## Risks / Trade-offs

**[Risk] Two different conversations whose word-chains are identical but
semantically distinct collide on the same base name and get `-2`, `-3`, …
even though they are unrelated.**

→ Mitigation: accepted. This is inherent to any content-derived name and
no worse than today's: an operator reading `-2` already knows two
conversations share that word-chain. `spec.title` (capped at 60 chars,
less aggressively trimmed) stays the authoritative label wherever a UI
already prefers it.

**[Risk] The list-then-create step adds one List call to every
conversation creation that did not exist before.**

→ Mitigation: the List is scoped by an exact label-value match, not a
broad scan. Conversation creation is not a hot path either — it is bounded
by `MAX_ACTIVE_CONVERSATIONS` admission already.

The cost is one indexed lookup per creation, independent of total
conversation count in the cluster.

**[Risk] A future consumer starts copying `conversation.Name` into another
label value without revisiting this design's length budget.**

→ Mitigation: the 24-char constant is defined once, in the new
`namewords` package, with a doc comment stating the 63-char DNS label
ceiling it was chosen against.

A future reviewer changing a downstream prefix length has one number to
check against.

## Migration Plan

Pure forward behavior change in object creation. No migration: existing
conversations keep their existing names unchanged, and carry no
`agentops.dev/name-base` label retroactively.

Nothing else in the codebase reads a conversation's name as a format
contract. `causedBy`, `pipelineRef` and the rest are independent fields,
not derived from the name string (verified during proposal research).

Rollback is a plain revert of the creation-site edits. A conversation
created under the old scheme and one created under the new scheme coexist
without conflict, since the new scheme's names never collide with the old
random-suffixed ones.
