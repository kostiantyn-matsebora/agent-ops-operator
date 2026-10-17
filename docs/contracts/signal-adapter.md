# The signal adapter contract

**Signals are one-directional**, so this is the channel contract minus the ops
queue. An adapter normalizes its transport into signals and the manager does the
rest: **adapters normalize, the manager groups.**

1. **Read your sources** and their opaque `spec.config` from
   `GET /signal/sources?adapter=`. Entries carry `credentialEnvPrefix` exactly
   like the channel listing.
2. **Push normalized signals** to `POST /signal/inbound`:

   ```json
   {"source":"…","signals":[{"fingerprint":"…","labels":{},"title":"…",
    "payload":"…","kind":"alert"}]}
   ```

   `kind` is one of `alert`, `job`, `task`, `chat`. `title` is optional.

   **A signal payload is never parsed as prose.** It reaches every adapter
   exactly as you sent it, and the adapter renders a CARD from your structured
   fields. The block grammar applies to AGENT output only — see [The body
   grammar](channel-adapter.md#the-body-grammar).

   The manager applies the source's `grouping` policy: fingerprint cooldown
   (at-least-once delivery is safe, re-sends collapse), signature from `labels`
   × `signatureLabels`, window reuse, and recurrence-on-session.
3. **Persist cursors** via `GET/PUT /signal/state/{source}/{key}`, and report
   config problems via `POST /signal/sources/{name}/status`.

**This endpoint is the only way work originates from outside a chat surface.**

There is no route that names a `Pipeline`. A caller posts to a `SignalSource`,
and the Ready Pipeline claiming that source decides which agent answers, on
which channels, with which capabilities.

**The `kind` selects the lane**, and the lane decides two things — which prompt
renders, and how a later signal is keyed when the source declares no
`signatureLabels`:

| `kind` | Lane | Subject | Keying with no `signatureLabels` |
|---|---|---|---|
| `alert` (default) | read-only investigation | a problem that recurs | default labels `alertgroup`/`alertname`/`namespace` — group and resume |
| `job` | task-lane prompt, `jobName` = source | a job that recurs | same default labels — successive ticks fold into one conversation |
| `task` | task-lane prompt, no `jobName` | a caller asking once | the signal's own fingerprint — each post is its own conversation |
| `chat` | task-lane prompt, no `jobName` | a person asking once | the signal's own fingerprint — each message is its own conversation |

- **`alert` and `job` are recurring-subject lanes.** The second signal is more
  news about the same thing, so it folds into the open conversation and resumes
  the session.
- **`task` and `chat` are one-shot lanes.** The second signal is a second
  request, and the default labels are alert vocabulary neither carries, so
  keying on them would hash every request to one empty signature.

**A `chat` signal MUST carry the label `agentops.dev/channel`** naming the
surface it arrived on, or it is refused — its reply has nowhere to go.

**A `task` signal must not.** Replies go to the claiming Pipeline's
`channelRefs`.

**A posted task inherits the target source's `grouping`.** The lane rule above
is a FALLBACK, not an override.

A source that declares `signatureLabels` groups by them in every lane, `task`
included, so two tasks sharing those label values land in one conversation. Two
tasks carrying none of them share the empty signature.

Post to a source whose grouping is what you want. Operators who need an isolated
ask lane create their own source against an adapter they run.

**Auth mirrors channels**: master token, or a per-`SignalAdapter` derived token
scoped to the adapter's name. The derivation context is distinct, so channel and
signal adapters sharing a name never share a token.

A `SignalSource` whose adapter nothing serves carries `Served=False`.

**Reference implementation:** [`signals/cron/`](../../signals/cron/), which replaced
the old roadmap `cron` sub-struct.

`config: {schedule, input, title?}` fires job-lane signals with
`<source>@<tick>` fingerprints, restart-safe via the state API. The grouping
window turns a recurring job into one conversation whose later runs resume the
agent session.

**How an adapter proves it:** the contract conformance suite,
`platform/manager/test/conformance/`, runs every adapter's BUILT BINARY against
a fake manager and asserts the obligations above.

For a channel adapter:

- the long-poll and the `contract=` declaration
- typed-message rendering
- ack-once under a redelivered op id
- inbound push with a `threadId`
- listing and status
- no relay loop

For a signal adapter:

- normalized emission, the bearer, and source scoping
- a rejected post that is retried or surfaced rather than dropped
- for a chat-originating adapter, the channel label on every signal

A new adapter joins by being listed there. Nothing in its own source changes.
See [Testing](../testing.md).

