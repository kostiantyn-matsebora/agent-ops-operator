## 1. Run the system tier in a cloud sandbox

- [x] 1.1 In a remote session, probe fix 1 (containerd `restrict_oom_score_adj`), then 2, then 3 from design D1, each verified by a trivial pod starting
- [x] 1.2 Put the working fix in `platform/manager/test/system/cluster.go` as the default for that host, with the probe recorded in `gotchas.md`
- [x] 1.3 If none works, make the station stop with the named failure and ask the owner about design D1 step 4

## 2. Incremental rounds

- [x] 2.1 `coverage-diff.py` takes several Go profiles and counts a block covered when any covers it
- [x] 2.2 The system pack takes `SYSTEM_RUN` to select lanes and keeps the cluster with `SYSTEM_REUSE=1`
- [x] 2.3 `test-change.md`: round 1 full, later rounds re-run failed and added tests, one closing full run judges
- [x] 2.4 `testing-station.py judge --partial <tier>` marks a tier whose numbers come from a partial run, and the judge refuses to pass it (a tier is passed only on a full run)

## 3. Remove the dispatch to CI

- [x] 3.1 `test-change.md`, `remote-session.md` and the `gotchas.md` row drop the `system-full.yml` and `system-smoke.yml` dispatch instructions

## 4. Unit tests

- [x] 4.1 `.github/tests/run.sh` covers the profile merge, the `judge` refusal and the stop-instead-of-dispatch path
- [x] 4.2 `go test ./...` in `platform/manager` for the lane selection

## 5. System tests

- [x] 5.1 Run the pack in a cloud session with `SYSTEM_COVER=1`, then re-run one lane with `SYSTEM_RUN` and `SYSTEM_REUSE=1`

## 6. Documentation

- [x] 6.1 Reference docs: `docs/testing.md` states the incremental rounds and that the system tier runs in the session
- [x] 6.2 Adopter site: no page names the station, so state that and tick it
