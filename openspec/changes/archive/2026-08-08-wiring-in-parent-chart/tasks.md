## 1. Remove wiring from the subcharts

- [x] 1.1 `k8s-bundle`: delete the events Pipeline from `templates/events.yaml`, keeping the SignalSource
- [x] 1.2 `k8s-bundle`: delete the addressable Pipeline from `templates/profile.yaml`
- [x] 1.3 `vm-bundle`: delete the Pipeline from `templates/alertmanager.yaml`, keeping the SignalSource
- [x] 1.4 Prune the values that only fed those templates, and the helper + render guards that resolved them
- [x] 1.5 Reword the values comments that claimed a bundle wires itself

## 2. Parent chart

- [x] 2.1 `chart/templates/pipelines.yaml`: render each `pipelines:` entry, failing on a missing name or profile
- [x] 2.2 `pipelines: []` in values, documented with a worked example
- [x] 2.3 `NOTES.txt`: point at `pipelines:` instead of a hand-applied Pipeline

## 3. Verify

- [x] 3.1 `helm lint` and render with every bundle enabled: exactly the declared Pipelines, no more
- [x] 3.2 Server-side dry run against a live cluster
- [x] 3.3 Live: one Pipeline claiming sources from two bundles, `Ready=True`, both sources `Wired=True`

## 4. Docs

- [x] 4.1 CLAUDE.md: no bundle ships a Pipeline; name pipelines for the job
- [x] 4.2 README: bundle sections describing rendered Pipelines
