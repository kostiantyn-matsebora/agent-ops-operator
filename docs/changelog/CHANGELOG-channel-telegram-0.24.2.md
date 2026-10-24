# Changelog archive — channel-telegram 0.24.2

Migration guide for **channel-telegram 0.24.2**, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](../CHANGELOG.md), which holds the ten most recent
versions.

## [channel-telegram 0.24.2] — 2026-08-24

### Fixed

**A signal card larger than about 4KB never arrived**, and the delivery retried
in a loop. The agents' own answers were unaffected — only event cards failed.

Telegram answered:

```
can't parse entities: Can't find end tag corresponding to start tag "blockquote"
```

**Cause.** A signal payload over six lines is folded into
`<blockquote expandable>`, which spans many lines. The splitter chooses a chunk
boundary at a newline, on an assumption stated in its own comment — that every
tag it emits opens and closes on the same line.

That was false for exactly this tag. The first chunk carried an opening tag with
no end and the second a stray end tag, and the Bot API rejects the **whole
message** rather than the tag.

The adapter already solved this one path over: a split fold in an agent's answer
has each piece re-wrapped in its own quote. A signal is not agent output, so it
never reached that code.

**Fix.** The splitter closes a quote it cuts and reopens the same tag on the
remainder, and reserves room for the end tag up front — adding it after choosing
a cut would push the chunk past the 4096 limit, trading one rejected message for
another.

**Still true after this:** a message the Bot API will never accept retries
forever. The latch that disables expandable quotes recognises a refusal about an
*unsupported* tag, and this one was *unbalanced*. This removes the trigger, not
the poison-pill behaviour.

### Upgrade

Nothing to do. The chart pins the new tag.
