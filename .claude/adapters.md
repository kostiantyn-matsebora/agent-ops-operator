## Adapters (binding)

### Channel adapter

**Out-of-process channel-type implementation consuming `/channel/*`** — ops
long-poll plus inbound push.

`Channel.spec` is two halves:

| Half | Holds |
|---|---|
| type-agnostic metadata | `adapter`, `credentialsSecretRef` — NO wiring, NO delivery mode |
| opaque `config` | only the serving adapter interprets it |

`status.threadId` is an opaque STRING.

#### READ IS PER THREAD, THEREFORE PER CHANNEL

`status.threads[].readAt` + `.readTracked`, written ONLY by the manager on an
adapter's report to the OPTIONAL `POST /channel/read`.

- **One shared mark would let a Telegram reader clear the console's**, which is
  the whole reason it sits on the binding.
- **The watermark is MONOTONIC and CLAMPED to the manager's clock.** A stale
  browser must not un-read a thread, and a skewed one must not mark the future
  read.
- **A report that would not advance is `skipped` with NO write.**
- **The batch is bounded at 50.**
- **`readTracked` is stamped on EVERY binding the manager creates**, for every
  channel, so the backfill rule stays ONE rule: a binding without it predates
  the mechanism and is READ. Same shape, same fix, same reason as
  `status.runs[].deliveryTracked` — and without it the first upgrade shows the
  whole namespace as new.
- **An adapter that never reports stays fully conformant.**

### `ChannelAdapter` CR

**Pure implementation** — `image` plus workload knobs. NEVER configuration or
credentials: no `type`, no `env`.

**Interface METADATA is allowed and encouraged:**

- `configSchema` — JSON Schema for the served CRs' `config`.
- `credentialKeys` — docs only, the manager reads no Secrets.

No config VALUES, connectivity, or credentials.

**Its reconciler owns the adapter Deployment**, and — when `spec.port` is set —
the Service, which is named after the WORKLOAD: `agentops-adapter-<name>`.
**There is no `agentops-channel-<name>`.** Two changes have now written that
name by mistake.

**`spec.kubernetesAccess` mirrors SignalAdapter's:** mounts the SA token and
injects `POD_NAMESPACE`, IDENTITY ONLY. Permissions stay an external grant
against SA `agentops-adapter-<name>`, and no reconciler ever creates RBAC.

**Credentials are per-surface** on `Channel.credentialsSecretRef`.

- **Projected into the adapter pod** as `envFrom`, prefix
  `AGENTOPS_CRED_<CHANNEL>_`, resolved by the KUBELET.
- **The contract's channel listing advertises `credentialEnvPrefix`.**

### `SignalAdapter` CR / signal adapter

**The same pattern for ingest, but inbound-only** — no ops queue.

Adapters push normalized signals (`fingerprint`, `labels`, `title?`, `payload`,
`kind: alert|job`) to `/signal/inbound`.

- **Grouping, cooldown and recurrence stay MANAGER-side** from
  `SignalSource.spec.grouping`. Adapters normalize, the manager groups.
- **Workload names `agentops-signal-<name>`.** Token derivation context is
  `signal-adapter:<name>`, never interchangeable with channel tokens.
- **There are NO built-in signal types.** The manager hosts no signal
  transports, so every type needs a serving adapter.
- **`SignalAdapter.spec.port` is an implementation property.** When set, the
  reconciler owns the Service `agentops-signal-<name>` and injects
  `LISTEN_ADDR`. Charts ship NO adapter connectivity.
- **`spec.kubernetesAccess`** mounts the SA token and `POD_NAMESPACE` for
  implementations that self-register with their SENDER. Push-model senders hold
  the "where to push" binding, so the adapter writes it from
  `SignalSource.spec.config.register`, degrading to instructions in the Ready
  condition when it can't.

### On both adapter kinds, the CR NAME is the ROUTING KEY

`Channel` / `SignalSource.spec.adapter` names the serving adapter.

- **A REFERENCE, not an attribute.** That adapter's implementation defines the
  schema of the sibling `config`.
- **It drives** the contract listing `?adapter=`, the injected `ADAPTER_NAME`,
  credential projection, token scope and `Served`.
- **One adapter per implementation by construction** — duplicates for one
  implementation are impossible.
- **Adapter CRs carry NO configuration.** Connectivity, credentials and config
  live only on Channel and SignalSource.
