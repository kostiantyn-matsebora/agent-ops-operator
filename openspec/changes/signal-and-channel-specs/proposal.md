# Proposal: signal-and-channel-specs

## Why

`Channel.spec.config` and `SignalSource.spec.config` are opaque blobs that only the serving adapter understands — an adopter defining a Channel or SignalSource has no way to discover what fields a given type requires short of reading the adapter's source, and mistakes surface only after deployment as an `InvalidConfig` Ready condition from the adapter. Adapter CRs should be able to (optionally) declare a machine-readable spec of their type's configuration so adopters can discover it and the manager can validate CRs against it early — without breaking the invariant that the manager itself knows nothing about any particular channel/signal type.

## What Changes

- `ChannelAdapter` and `SignalAdapter` CRDs gain an optional **config schema declaration on their spec**: `spec.configSchema` (JSON Schema for the served type's `spec.config`) plus `spec.credentialKeys` (documentation of expected credential Secret keys). The declaration is part of the adapter CR the adopter applies — a static, cluster-visible contract. No registration protocol, no new HTTP endpoints: the manager reads it from the CRD like any other client, and any future system (docs generators, UIs, admission tooling) can read the same source.
- A declaration on the CR spec is implementation *contract* metadata, not configuration or credentials — adapter CRs still carry no connectivity, config values, or secrets.
- The adapter CR reconcilers compile the declared schema and surface an uncompilable one as a condition on the adapter CR (`SchemaValid=False`); a broken schema never blocks the adapter workload.
- The Channel and SignalSource reconcilers **validate `spec.config` against the declared schema** when one exists, reporting the result as a new `ConfigValid` condition. Validation is generic (apply an adapter-declared JSON Schema; no type knowledge manager-side) and **advisory** — a violation does not stop the channel/source from being served; the adapter remains the authority via the Ready condition. When no schema is declared, `ConfigValid` is not set and behavior is unchanged.
- Reference declarations ship where the reference adapter CRs live: the chart's gated `ChannelAdapter` for telegram (chatId required; feedThreadId/approvers/pollingEnabled optional; `botToken` credential key) and the cron `SignalAdapter` sample (schedule/input required, title optional). The vmalertmanager adapter declares nothing, demonstrating optionality. **Adapter binaries are untouched.**
- CRD `config` fields stay `PreserveUnknownFields` — admission still accepts arbitrary config for any type; schema violations surface as a condition, not a rejection.

## Capabilities

### New Capabilities

- `adapter-config-schema`: the config schema declaration — its place on the adapter CR spec (JSON Schema for config + credential key documentation, optional), schema compile checking on the adapter CR, generic manager-side validation semantics (`ConfigValid` condition, advisory, adapter stays authoritative), and adopter discoverability straight from the CRD.

### Modified Capabilities

- `channel-type-model`: "Arbitrary config accepted for any type" is amended — config remains schema-less at admission, but when the serving adapter CR declares a schema the manager validates against it and reports `ConfigValid`.
- `signal-source-model`: same amendment for SignalSource config.
- `telegram-channel-adapter`: the chart-shipped ChannelAdapter CR declares the telegram config schema and credential key documentation.
- `cron-signal-adapter`: the reference SignalAdapter CR declares the cron config schema.

## Impact

- `api/v1alpha1/`: new **spec** fields on `ChannelAdapter` and `SignalAdapter` (+ deepcopy/CRD regen, `chart/files/crds/`), new condition constants (`ConfigValid`, `SchemaValid`).
- `internal/controller/`: adapter reconcilers gain the schema compile check; `channel_controller.go` and `signalsource_controller.go` gain schema validation + `ConfigValid` (they already Get and watch the adapter CRs — no new plumbing).
- New JSON Schema validation dependency in the **root module only**; adapter modules unchanged (no code changes at all in `channel-telegram/`, `signal-cron/`, `signal-vmalertmanager/`).
- `chart/`: telegram ChannelAdapter template gains the declaration; `config/samples/`: cron SignalAdapter sample gains it.
- No httpapi changes, no secret reads, no RBAC changes, token scoping unchanged. README + CLAUDE.md terminology updates.
