# Changelog archive — chart 8.0.0

Migration guides for chart version **8.0.0**, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](../CHANGELOG.md), which holds the ten most recent
versions.

## [8.0.0] — 2026-08-23

**The chart could not be installed on a cluster that did not already have its
CRDs**, and it had been that way for the project's whole life. Every install
until now landed where a previous one had left the CRDs behind, so nothing ever
surfaced it.

**BREAKING: `crds.enabled` and `crds.keep` are gone.** Setting either now FAILS
the render naming the replacement, rather than being silently ignored.

| Was | Now |
|---|---|
| `crds.enabled: false` | `helm install --skip-crds` |
| `crds.keep: true` | inherent — Helm never deletes CRDs it installed from `crds/` |

### Fixed

- **The CRDs moved from `templates/` to the chart's `crds/` directory.** Helm
  applies that directory out-of-band, invalidates discovery and waits for the
  CRDs to establish BEFORE it builds the rest of the manifest.

  This chart ships eleven CRDs beside eight instances OF them — Pipelines,
  Channels, profiles, toolsets. Helm resolves every kind in a manifest before
  applying any of it, so as templates those instances could not map and a clean
  install aborted with `ensure CRDs are installed first`.

  Helm's own guidance names two methods and no third: this directory, or two
  separate charts. There is no annotation that orders resources within one
  release.

- **Cluster-scoped RBAC now carries the release namespace.** Every `ClusterRole`
  and `ClusterRoleBinding` the chart renders is suffixed with it, so two installs
  in one cluster no longer collide. Previously a second release failed with
  `ClusterRole "agentops-signal-k8s-events-events" … cannot be imported`, which
  made a side-by-side demo or a staging namespace impossible.

  ServiceAccount subjects are untouched — those are namespaced already.

- **The console no longer serves the wrong auth mode at startup.** It read its
  browser token only after the manager's channel listing arrived, and retried
  that listing on the steady 60-second cadence — so a fresh install prompted for
  a token for a full minute, even where a proxy authenticates instead.

  The credentials are projected into the pod before the process starts. The
  console now reads them from its own environment, and retries an unresolved
  listing every second rather than every minute. Console image **0.38.0**.

### Changed

- **Helm no longer upgrades the CRDs either**, which is the documented cost of
  the `crds/` directory. When a release changes a CRD field its entry here says
  so and gives the command:

  ```sh
  kubectl apply -f https://raw.githubusercontent.com/kostiantyn-matsebora/agent-ops-operator/master/chart/crds/
  ```

  This release changes no CRD field, so there is nothing to apply.

### Upgrade

**Nothing to do**, unless you set `crds.enabled` or `crds.keep` yourself — remove
them, or the render fails naming the replacement.

Your existing CRDs are already in the cluster and Helm adopts them where the
annotations match. An install onto a cluster with no agentops CRDs now works
with no flags and no pre-step, which is what this release is for.
