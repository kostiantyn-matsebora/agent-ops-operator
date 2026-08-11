## 1. The interval matcher

- [x] 1.1 Add the config types to `signal-k8s-events` — `TimeInterval` (name + `times`/`weekdays`/`daysOfMonth`/`months`/`years`/`location`) and `MuteTimeInterval` (name reference + optional `matchers`) — under `route`, using Alertmanager's field names verbatim
- [x] 1.2 Implement interval matching against an INJECTED `now`, so every case is a unit test rather than a wait
- [x] 1.3 Resolve `location` as an IANA zone at config load; an unparseable zone or interval fails the source's Ready condition with the reason rather than being ignored
- [x] 1.4 Union overlapping intervals — muted when any referenced interval matches
- [x] 1.5 Tests: inside, outside, boundary minutes, weekday selection, day/month/year selection, a zone whose offset differs from UTC, and behaviour across a DST transition

## 2. Wiring it into the emit path

- [x] 2.1 Evaluate the mute AFTER the dwell queue and BEFORE the emit cap, so a persistent problem still surfaces once the window closes
- [x] 2.2 Apply a mute entry's `matchers` when present; with none, mute everything from that source
- [x] 2.3 Ensure muted events do not consume the emit cap and are discarded from the dwell queue rather than accumulating
- [x] 2.4 Tests: transient noise inside the window emits nothing; a failure persisting past the window emits after it; an event confirmed inside the window is muted; a matcher-narrowed window lets an unrelated reason through; the emit cap is untouched by muting

## 3. Making the mute visible

- [x] 3.1 Count muted events per source
- [x] 3.2 Report an ACTIVE mute on the source's Ready condition, naming the interval, mirroring how emit-cap clipping is already surfaced
- [x] 3.3 Report the muted count when the window closes
- [x] 3.4 Tests: the condition names the active interval, and the count is reported after the window

## 4. Chart

- [x] 4.1 Add the values to `chart/charts/k8s-bundle` for the events source, defaulting to no intervals
- [x] 4.2 Ship the nightly maintenance window as a worked, commented example that names an IANA location and explains the DST consequence
- [x] 4.3 Chart render tests: defaults declare no intervals; configured values reach the rendered SignalSource

## 5. Documentation

- [x] 5.1 `docs/k8s-bundle.md` — the suppression section gains the time axis beside `rules` and `route`, with the reason none of the existing three axes can express a scheduled window
- [x] 5.2 Record in `CLAUDE.md` that the time axis is Alertmanager vocabulary too, and that mute is evaluated at EMIT — the property that keeps a persistent problem reportable
- [x] 5.3 `CHANGELOG.md` entry for the new config keys

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` in `signal-k8s-events`
- [x] 6.2 Full root test run with `KUBEBUILDER_ASSETS` (unit + envtest), since chart render tests live there
- [x] 6.3 Live: configure the nightly window on the reference cluster, force a matching event inside a temporary test window, and confirm no conversation is created while the source's Ready condition names the interval
- [x] 6.4 Live: confirm an event outside the window still produces a conversation, so the window is not silently permanent
