# Design: signal-and-channel-specs

## Context

`Channel.spec.config` / `SignalSource.spec.config` are `*runtime.RawExtension` with `PreserveUnknownFields` — "schema-less by design, validated by the serving adapter, never by the operator". Adapters report validation results back only as the per-CR Ready condition (reason `InvalidConfig`). Adopters writing a Channel/SignalSource must read adapter source code to learn what `config` needs.

The adapter CRs (`ChannelAdapter`/`SignalAdapter`) are named after the type they serve — the CR name is the routing key — and the Channel/SignalSource reconcilers already `Get` and watch them by name for the `Served` condition.

Constraints that shape everything:
- The manager stays type-agnostic — it must never gain knowledge of any particular channel/signal type's config semantics.
- The manager reads no Secrets — a "credentials documentation" feature can only ever describe expected keys, never check them.
- Adapter binaries are dependency-free modules and should stay untouched by this change.
- Adapter CRs carry no configuration, connectivity, or credentials — a config *schema* is contract metadata describing the implementation's interface, which is exactly what an adapter CR is for.

## Goals / Non-Goals

**Goals:**
- An optional, machine-readable declaration of a type's `config` shape (+ expected credential Secret keys) as part of the adapter CR spec — a static, cluster-visible contract that the manager and any future system (docs tooling, UIs, admission tooling) read straight from the CRD.
- Discoverability: `kubectl get channeladapter <type> -o yaml` shows what to write, whether or not the adapter pod has ever run.
- Early, generic manager-side validation of `spec.config` against the declaration, surfaced as a condition — before/independent of the adapter's own runtime validation.
- Zero behavior change when no declaration is present; zero adapter code changes either way.

**Non-Goals:**
- Mandatory schemas or admission-time rejection — CRD `config` stays `PreserveUnknownFields`; a schema violation never blocks apply or serving.
- Runtime schema publication by adapter processes (no contract endpoints, no registration).
- Verifying credential Secrets (contents or existence) — `credentialKeys` is documentation only.
- Schema-driven UI/defaulting/migration tooling.
- Schemas for in-process built-in channel providers (registry types have no adapter CR to carry one).

## Decisions

### D1: The schema is declared on the adapter CR spec

`ChannelAdapter.spec.configSchema` / `SignalAdapter.spec.configSchema` (`*runtime.RawExtension`, `PreserveUnknownFields` — a JSON Schema draft 2020-12 document for the served type's `spec.config`) plus `spec.credentialKeys []CredentialKeyDoc{key, required?, description?}`. Both optional.

The adapter CR is the declaration of a type implementation; its config contract belongs with it. Spec placement makes the contract:
- **static** — present from the moment the CR is applied, before/without the adapter pod running;
- **registration-free** — no publication protocol, no new HTTP surface, no status writes from httpapi;
- **universally readable** — the manager consumes it via the informer cache it already has, and any other cluster client (kubectl, future doc/UI/admission tooling) reads the same source of truth;
- **reviewable** — it travels through git/helm review with the rest of the CR.

This does not violate "adapter CRs carry no configuration": `configSchema` holds no config *values*, connectivity, or credentials — it describes the implementation's interface, i.e. it is part of "pure implementation".

*Alternatives:* (a) runtime publication through the adapter contract with storage on CR status — rejected (user decision): adds endpoints and registration machinery, the contract only exists while an adapter has run, and status is lost on CR recreate; (b) a ConfigMap per type — more objects, unclear ownership; (c) chart-only documentation — not machine-readable in-cluster.

### D2: The adapter CR reconciler compile-checks the declared schema

On reconcile, if `spec.configSchema` is set, the reconciler compiles it and sets a `SchemaValid` condition on the adapter CR: True, or False (reason `InvalidSchema`, message with the compile error). An invalid schema SHALL NOT block the workload (`Deployed`/`Ready` unaffected) — it only means downstream validation is skipped. This gives the CR author immediate feedback where they authored the mistake.

*Alternatives:* admission-time rejection via webhook — rejected: this operator has no webhook infrastructure and a schema typo shouldn't brick an adapter apply; CEL — can't compile JSON Schema.

### D3: Validation is generic, reconciler-side, and advisory — condition `ConfigValid`

The Channel/SignalSource reconcilers already `Get` the adapter CR by `spec.type` (for `Served`) and already **watch** adapter CRs with a type→CRs event mapping, so declaring or editing a schema automatically re-reconciles every served CR — no new plumbing. On reconcile: if the adapter CR exists, its `SchemaValid` is not False, and `spec.configSchema` is present, validate `spec.config` (nil config validates as `{}`) and set:

- `ConfigValid=True` reason `SchemaValidated` — config conforms;
- `ConfigValid=False` reason `SchemaViolation`, message listing the violations (path + error, truncated to condition-message size);
- **no `ConfigValid` condition at all** when no schema is declared (or the adapter CR is absent, or its schema doesn't compile) — absence means "nothing to check", not "unknown".

Advisory means: `Served`, the adapter listings, credential projection, dispatch, and signal ingestion are all unaffected by `ConfigValid=False`. The adapter's own Ready reporting stays authoritative (the CR-declared schema may drift from the running image; blocking on it would turn a stale schema into an outage). This deliberately amends the "operator never validates config" posture to "the operator never *interprets* config — it may apply an adapter-declared schema mechanically", which keeps the real invariant (no type knowledge in the manager) intact.

*Alternatives:* folding the result into `Served` or Ready — rejected: conflates "is anything serving this type" / adapter-observed health with declarative lint; webhook/admission enforcement — rejected: makes a stale schema block valid configs.

### D4: JSON Schema draft 2020-12; validator dependency in the root module only

`github.com/santhosh-tekuri/jsonschema/v6` (pure Go, no transitive weight) in the operator module, wrapped in one internal package (`internal/configschema`) so it stays swappable. Adapter modules gain nothing — they have no role in this feature. JSON Schema over a bespoke DSL because adopters already know it, it round-trips through the CR as plain JSON/YAML, and expressiveness (required, enums, patterns) covers real adapter configs.

### D5: Reference declarations ship with the reference adapter CRs

- The chart's gated telegram `ChannelAdapter` template declares: `chatId` (string, required), `feedThreadId` (integer), `approvers` (array of integer), `pollingEnabled` (boolean); `credentialKeys: [{key: botToken, required: false}]` (false because of the `TELEGRAM_BOT_TOKEN` fallback).
- The cron `SignalAdapter` sample in `config/samples/` declares: `schedule` (string, required), `input` (string, required), `title` (string); no credential keys.
- The vmalertmanager `SignalAdapter` (vm-bundle subchart) declares nothing — the living proof that declaration is optional.

Each declaration lives beside the `image` field it describes, so an image bump and its schema update travel in the same diff.

## Risks / Trade-offs

- [Schema/image drift — the CR-declared schema is authored separately from the parser in the image] → the declaration sits next to `image:` in the same CR and ships in the same chart/sample diff; validation is advisory-only so drift can never block a valid config; the adapter's runtime Ready reporting remains the authority. Documented as an authoring rule: bump schema with image.
- [Malformed or adversarial schema (huge, pathological regex/refs)] → compile check at the adapter CR (`SchemaValid=False` skips validation); CR size is already bounded by etcd limits; validation errors truncated for condition messages.
- [`ConfigValid=False` misread as "broken"] → condition message names the violating fields and the declaring adapter CR; README documents advisory semantics.
- [New root-module dependency] → confined to `internal/configschema`; adapter modules untouched.
- ["Adapter CRs carry no configuration" invariant appears weakened] → sharpened instead: no config *values*, connectivity, or credentials; interface metadata (schema, credential-key docs) is implementation declaration. CLAUDE.md updated to state the distinction.

## Migration Plan

Purely additive: new optional spec fields (CRD regen → `chart/files/crds/`), new conditions. Existing adapter CRs without a declaration: nothing changes. Chart upgrade adds the telegram declaration; downgrade prunes the fields with the CRD — no rollback concerns. No adapter image changes, so no image tag bumps required by this change (manager image only).

## Open Questions

- None blocking. (Future: surface `credentialKeys`/schema presence in `kubectl get` printer columns; admission-time warnings via a webhook if the project ever grows one.)
