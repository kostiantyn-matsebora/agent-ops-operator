# adapter-config-schema

## ADDED Requirements

### Requirement: Adapter CRs may declare a config spec for their type
The `ChannelAdapter` and `SignalAdapter` CRDs SHALL carry an optional **config schema declaration** on their spec: `spec.configSchema` — a JSON Schema (draft 2020-12) object describing the shape of `spec.config` for Channels/SignalSources of the served type — and `spec.credentialKeys` — a list of expected credential Secret keys (`{key, required?, description?}`). Declaration is OPTIONAL — an adapter CR that declares nothing SHALL behave exactly as before this capability existed, and no part of serving, credential projection, or dispatch SHALL depend on a declaration being present. The declaration is static contract metadata readable by any cluster client (the manager, kubectl, future tooling) directly from the CR — there SHALL be no runtime registration or publication step, and adapter binaries SHALL play no role in it. The declaration SHALL carry no config values, connectivity, or credentials — adapter CRs remain pure implementation.

#### Scenario: Adopter discovers the config contract from the CRD alone
- **WHEN** an operator user runs `kubectl get channeladapter telegram -o yaml` before the adapter pod has ever started
- **THEN** the spec shows the JSON Schema for `spec.config` and the documented credential Secret keys

#### Scenario: Declaration is optional
- **WHEN** an adapter CR is applied without `configSchema` or `credentialKeys`
- **THEN** its type is served exactly as before, and no schema-related condition appears on its Channels/SignalSources

#### Scenario: Declaration without config schema
- **WHEN** an adapter CR declares only `credentialKeys` (no `configSchema`)
- **THEN** the credential keys are discoverable on the CR, and no config validation occurs

### Requirement: Declared schemas are compile-checked on the adapter CR
When `spec.configSchema` is set, the adapter CR reconciler SHALL compile it and report a `SchemaValid` condition on the adapter CR: True on success, False (reason `InvalidSchema`, message naming the compile error) otherwise. An invalid schema SHALL NOT affect the adapter workload (`Deployed`/`Ready` unchanged) — it only disables downstream config validation for the type.

#### Scenario: Broken schema surfaces where it was authored
- **WHEN** a ChannelAdapter is applied with a `configSchema` that is not a valid JSON Schema
- **THEN** the ChannelAdapter carries `SchemaValid=False` naming the error, its Deployment is created normally, and its Channels carry no `ConfigValid` condition

#### Scenario: Valid schema reports itself
- **WHEN** an adapter CR declares a compilable schema
- **THEN** the adapter CR carries `SchemaValid=True`

### Requirement: Manager validates config against a declared schema, generically and advisorily
When the serving adapter CR (the CR named by `spec.type`) declares a compilable `configSchema`, the Channel/SignalSource reconciler SHALL validate the CR's `spec.config` (absent config validates as `{}`) against it mechanically — applying the adapter-declared schema with a generic JSON Schema validator, with no type-specific knowledge in the manager — and report the result as a `ConfigValid` condition: True (reason `SchemaValidated`) on conformance, False (reason `SchemaViolation`, message naming the violating fields) otherwise. When no schema is declared for the type (or the adapter CR is absent, or its schema does not compile), the `ConfigValid` condition SHALL be absent (removed if previously set). Validation SHALL be advisory: a False `ConfigValid` SHALL NOT affect `Served`, adapter listings, credential projection, dispatch, or signal ingestion — the adapter's own Ready reporting remains authoritative. Declaring or changing a schema SHALL re-trigger validation of the served CRs without manual intervention.

#### Scenario: Violation surfaces early as a condition
- **WHEN** a Channel of a schema-declaring type is applied with `config` missing a schema-required field
- **THEN** its status gains `ConfigValid=False` naming the missing field, while `Served` and adapter delivery of the channel are unaffected

#### Scenario: Conforming config reports valid
- **WHEN** a SignalSource's `config` conforms to the schema its serving SignalAdapter declares
- **THEN** its status carries `ConfigValid=True`

#### Scenario: Declaring a schema re-validates existing CRs
- **WHEN** an adapter CR gains a `configSchema` after Channels of its type already exist
- **THEN** each such Channel is re-reconciled and gains a `ConfigValid` condition without being touched by the user

#### Scenario: Removing the declaration clears the condition
- **WHEN** an adapter CR's `configSchema` is removed (or the adapter CR is deleted)
- **THEN** served CRs of that type end up with no `ConfigValid` condition

#### Scenario: Manager stays type-blind
- **WHEN** validation runs for any type
- **THEN** the manager applies only the adapter-declared schema document — it contains no per-type validation code and never interprets config fields semantically
