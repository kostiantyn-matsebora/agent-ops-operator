## 1. Cluster Events adapter

- [x] 1.1 In `signals/k8s-events/pending.go` replace `recurred bool` with `lastSeen time.Time`, add `stillRecurring(e, now)` computing the closing window as `max(30s, (deadline−firstSeen)/3)`, and route rung 2 through it; verify `go build ./... && go vet ./...` in the worktree via `docker exec -w "$PWD" agentops-go`
- [x] 1.2 Extend the evidence payload with `last seen <d> before the window closed` and log the dropped-as-quiet verdict at debug level; verify with the existing evidence test updated to assert the line
- [x] 1.3 Rewrite `pending_test.go` from `recurred` to arrival timelines and add the three spec scenarios (quiet burst dropped, live retry emitted, shortened dwell uses the 30s floor); verify `go test ./...` passes in the worktree

## 2. Home Assistant adapter

- [x] 2.1 Apply the same change to `signals/ha/pending.go` — `lastSeen`, `stillRecurring`, evidence line, debug log — with a comment naming the sibling file; verify build and vet in the worktree
- [x] 2.2 Add the two spec scenarios to `signals/ha/pending_test.go` (thirty-second blip dropped, still-logging emitted) and rewrite the `recurred` assertions; verify `go test ./...` passes in the worktree

## 3. Release and pin

- [x] 3.1 Pin `signal-k8s-events` 0.4.4 and `signal-ha` 0.2.4 in `chart/charts/kubernetes/values.yaml` and `chart/charts/home-assistant/values.yaml`, bump both bundle charts and the parent chart (13.2.0 — a behaviour change); verify `helm lint chart` and the chart template tests pass in the worktree's test container
- [x] 3.2 POST-MERGE, as #98 did: `signal-k8s-events-v0.4.4` and `signal-ha-v0.2.4` tagged from the merge commit `d52c9d5` in one push; both images published with `linux/amd64` and `linux/arm64`. `chart-v13.2.0` is DEFERRED by decision — the chart pins the images but is not released yet, so nothing installable references them

## 4. Verification on the reference install

- [x] 4.1 Deploy the worktree's chart (`--state-values-set chartPath=...`), then induce a self-healing burst on a kind with no predicate and a persisting one; verify the first opens no conversation and the second opens one whose signal card names the last-arrival gap

## 5. Documentation

### Reference docs

- [x] 5.1 `docs/integrations/kubernetes.md` — the verification section states rung 2 as "still recurring as the window closed" with the derived closing window; verify the page reads through `python3 .github/scripts/docs-generate.py --check` and the site lint
- [x] 5.2 `docs/integrations/home-assistant.md` — the re-check table row becomes "was it still recurring as the window closed"; verify the same check
- [x] 5.3 `chart/charts/kubernetes/values.yaml` and `chart/charts/home-assistant/values.yaml` — the `rules` comment describing the re-check says "still recurring at the close", and `python3 .github/scripts/docs-generate.py` is re-run because a chart value changed; verify `--check` passes
- [x] 5.4 `docs/CHANGELOG.md` — entry for the new chart version: fewer conversations from self-healing bursts on kinds without a predicate, the slow-backoff trade-off, and the "shorter `for`" restatement for anyone wanting the old behaviour; verify the version matches `chart/Chart.yaml`

### Adopter site

- [x] 5.5 Confirm the landing page, `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*` make no claim about rung 2 that is now untrue, and that `installation.md` prints the new chart version; verify `python3 .github/scripts/docs-generate.py --check` and `python3 .github/scripts/retired-vocabulary-guard.py` both pass
