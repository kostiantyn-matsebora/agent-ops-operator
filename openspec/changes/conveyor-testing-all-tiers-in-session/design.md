## Context

`test-change.md` runs each tier per round and, for the system tier on a cloud session,
dispatches CI. A gotcha records nine failed probes against runc `oom_score_adj: Permission
denied` on every pod sandbox.

## Decisions

### D1. Run the system tier in the session; CI is never a fallback

Order of attempts, each verified by a trivial pod starting, stopping at the first that works:

1. containerd `restrict_oom_score_adj = true` in the k3s containerd template. It clamps the
   value to the process's own, so nothing is lowered. Kubelet gives the pause sandbox and
   Guaranteed pods a negative score, and lowering needs `CAP_SYS_RESOURCE`. **Verified in a cloud
   sandbox.** The drop-in is mounted as a DIRECTORY: a single-file mount makes `k3d kubeconfig get`
   fail with `mkdirat ... file exists`, so a run against a live cluster cannot start.
2. `KubeletInUserNamespace=true` on the kubelet (not needed, untried).
3. Rootless k3s.
4. A real API server and etcd without a kubelet, only if 1 to 3 fail, and only with the
   owner's decision, since it drops the pack's pod-lifecycle lanes.

If none works, the station stops with the reason. No dispatch.

### D2. Incremental rounds

| Round | Runs | Coverage |
|---|---|---|
| 1 | every tier, in full | full profiles, kept |
| 2..n | the failed tests by name (`-run`), plus tests the tester added | their profile merged into round 1's |
| closing | every tier, in full, once | full profiles, the ones judged |

- A tier is judged passed only on the closing full run.
- The pack takes `SYSTEM_RUN` (a lane name pattern) and `SYSTEM_REUSE=1` keeps the cluster up,
  so a failed lane re-runs in minutes.
- Alternative: a full run every round. Rejected, it is the problem.

### D3. Profile merge

`coverage-diff.py` accepts several Go profiles and counts a block covered when any of them
covers it, as `coverage-floor.sh` does.
