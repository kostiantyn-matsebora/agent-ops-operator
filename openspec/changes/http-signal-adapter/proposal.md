## Why

Nothing accepts an arbitrary HTTP request as a signal. `signals/alertmanager` and `signals/pulse` each parse one vendor's payload. Anything else (CI, a script, a monitoring tool with a generic "call this URL" action) has no adopter-friendly way in.

## What Changes

- **New adapter `signals/http`** (image `agentops-signal-http`). It serves `POST /request/{source}` and pushes one signal per request. The name says what it is: a sender calls it, nobody subscribes.
- **Per-source config of every normalized field.** `initiationId`, `signalId`, `kind`, `title`, `labels`, `payload` and `severity` are each a template or a constant. `reader` is not exposed.
- **No grouping by default.** An unset or unresolvable `initiationId` and `signalId` become a fresh random id per request.
- **`kind` is unset by default.** The adapter sends none and the manager reads `alert`. A templated or constant `kind` may be `alert`, `job` or `task`, never `chat`.
- **A bearer token is mandatory per source**, from that source's own `credentialsSecretRef`. There is no anonymous mode and no adapter-wide token.
- **Bad config surfaces on the source.** Templates are parsed at source load. A fault sets `Ready=False` through the status endpoint.
- **New adapter `channels/http`** (image `agentops-channel-http`). It delivers each agent message by POSTing typed JSON to a URL set on the `Channel`, and accepts replies on `POST /reply/{channel}`. It is reply-only, so it never starts a conversation.
- **A chart bundle** `chart/charts/http/` rendering the `SignalAdapter` and the `ChannelAdapter`, default off, one `http.enabled`.
- Not a change to the manager or to the signal adapter contract's obligations.

## Capabilities

### New Capabilities
- `http-signal-adapter`: the adapter, its endpoint, auth, field mapping, validation and failure behaviour.
- `http-channel-adapter`: the channel adapter, its outbound delivery, reply endpoint, auth and failure behaviour.

### Modified Capabilities

None. Listing the adapters on the contract pages' shipped-adapter enumerations is an editorial change that needs the user's approval (`signal-rules.md`, `channel-spec.md`), tracked as a task.

## Impact

- New modules `signals/http/` and `channels/http/`, two new published images.
- New chart bundle under `chart/charts/http/`.
- `docs/integrations/http.md`, `.claude/rules/structure.md`, `docs/contracts/signal-adapter.md` and `docs/contracts/channel-adapter.md` (approval first for each) and the changelog.
- No manager, CRD or contract-version change.
