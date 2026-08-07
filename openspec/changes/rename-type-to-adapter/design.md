# Design: rename-type-to-adapter

## D1 — Rename the selector, keep the layout

The defect is nominal, not structural. Every Kubernetes precedent for
"selector + implementation-specific config" keeps the two as siblings and
names the selector after the implementation that owns the config:

| API | selector | config slot |
|---|---|---|
| StorageClass | `provisioner` | `parameters` (inline map) |
| IngressClass | `controller` | `parameters` (object ref) |
| GatewayClass | `controllerName` | `parametersRef` (object ref) |

So `spec.adapter` + `spec.config` stays flat. `config` remains INLINE and
opaque rather than a reference: the adapter contract already delivers it, and
a referenced object could not be read by the adapter anyway — adapters hold
zero Kubernetes permissions by invariant, so the manager would have to read
and forward it, buying indirection and an extra object per surface for no
clarity the rename does not already provide.

## D2 — One env name for both surfaces

`CHANNEL_TYPE` and `SOURCE_TYPE` both carried the same thing: the adapter CR's
name. They collapse into `ADAPTER_NAME`, injected by both reconcilers. An
adapter no longer needs to know which surface vocabulary it belongs to in
order to read its own identity.

## D3 — Query parameter follows the field

`?type=` becomes `?adapter=` on the three listing/polling endpoints. This is a
breaking contract change with no shim: an adapter built against the old
contract sends `?type=` and gets a 400 naming the new parameter, which is a
far better failure than being silently served an empty list and appearing to
work while delivering nothing.

## D4 — Migration must preserve annotations

`adapter` is immutable (as `type` was), so live Channels and SignalSources are
delete-and-recreate. Adapter cursor state is annotation-backed on those very
objects (`/channel/state/{channel}/{key}` writes
`agentops.dev/state-<key>`), so a naive recreate loses the Telegram
`getUpdates` offset and the bot re-processes old updates. The migration
captures annotations first and restores them with `kubectl annotate` AFTER the
manifest is applied — folding them into the applied manifest instead puts them
in `last-applied-configuration`, so the next `kubectl apply` of the
annotation-free site manifest strips them again.

Ordering per surface: capture → delete → recreate with `adapter:` + saved
annotations → confirm `Served=True`. Channels are recreated before the manager
rollout completes only if the adapter is briefly idle; a missing Channel makes
inbound posts 404 for a few seconds, which the adapters retry.

## D5 — Rollout order

The CRD accepts `adapter` before any workload uses it, so: apply CRDs →
recreate CRs with the new field → upgrade manager + adapter images together.
Manager and adapters must move in one step, since the query parameter changes
on both sides of the contract simultaneously.
