# Changelog archive — chart 13.0.0

Migration guides for chart version **13.0.0**, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](../CHANGELOG.md), which holds the ten most recent
versions.

## [13.0.0] — 2026-08-24

**The one-flag demo works on a laptop cluster, with its memory intact.**

### Changed

**`persistence.context.accessModes` and `.workspace.accessModes` ship EMPTY, and
the chart answers them.** `ReadWriteMany` for an ordinary install, exactly as
before. `ReadWriteOnce` under `global.demo.enabled`.

The demo failed on every cluster a reader is likely to try it on. `local-path`
is the only storage class rancher-desktop, k3d, kind and minikube ship, and it
REFUSES an RWX claim:

```
failed to provision volume with StorageClass "local-path":
NodePath only supports ReadWriteOnce and ReadWriteOncePod (1.22+) access modes
```

The claim sat `Pending`, no runtime pod was ever created, and the conversation
waited forever. Getting started documented the workaround —
`persistence.context.enabled=false` — which buys a working demo by taking away
the thing the demo is showing.

- **It is not about node COUNT.** Any cluster whose only provisioner is
  RWO-only hits it. A single-node cluster with an RWX provisioner never does.
- **RWO is CORRECT under demo, not merely tolerable.** Many pods share an RWO
  volume on one node and the volume's affinity is what puts them there, so the
  default cap of five concurrent conversations still holds.
- **An explicit value wins in both modes.** The default is empty rather than a
  mode because an empty list is the one thing a chart can tell apart from a
  choice somebody typed.

### Upgrade

**A DEMO install whose context claim already BOUND cannot be upgraded in place.**
A PVC's `accessModes` is immutable, so Helm's patch is refused by the API server
and the upgrade fails naming the field.

Two ways out, and the first is the one a demo deserves:

```sh
# the demo is disposable — reinstall it
helm uninstall agent-ops -n agent-ops && kubectl delete ns agent-ops

# or keep the claim exactly as it is
helm upgrade ... --set 'persistence.context.accessModes={ReadWriteMany}'
```

**No ordinary install is affected.** Outside demo mode the rendered claim is
`ReadWriteMany` before and after, so there is no patch to refuse.
