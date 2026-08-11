## 1. Tooling, generator and vocabulary

- [x] 1.1 Reconnect the draw.io MCP server; record the real constraint (one session owns the container) in design.md D6
- [x] 1.2 Create `docs/diagrams/` with the generator `build-why.py` and `icons/`, emitting pages `why`, `components`, `domain`
- [x] 1.3 Settle the visual vocabulary: line art, one accent, dotted zones — hex values for BOTH themes recorded in the spec
- [x] 1.4 Hand-draw the icon set (19 SVGs) and derive the dark set by substitution rather than drawing twice
- [x] 1.5 Prove the SVG export path: text-as-text, opaque background, no embedded source, no `foreignObject` fallback

## 2. Source material

- [x] 2.1 Extract the component inventory: the manager plus its five reader-facing packages (`httpapi`, controllers, `chat`, `dispatch`, `ingest`), the six adapter processes, `telegram-router`, the runtime pod, the Kubernetes API server, and the external systems
- [x] 2.2 Extract the edge inventory from `internal/httpapi/server.go`, `signals.go`, `activity.go` and `docs/contracts.md`: every endpoint, its caller, its direction, and whether it long-polls
- [x] 2.3 Extract the domain edges from `api/v1alpha1` and `docs/concepts.md`: every reference between kinds with cardinality, and every ownership edge
- [x] 2.4 Cross-check both inventories against the `CLAUDE.md` invariants — in particular that the manager calls no adapter, that `AgentProfile` carries no capabilities, and that `Conversation` holds materialized bindings and no `pipelineRef`

## 3. Page 3 — domain

- [ ] 3.1 REDO in the icon vocabulary: place all eleven CRD kinds from `chart/files/crds/` using the CRD-kind style, laid out with `Pipeline` as the visual hub
- [ ] 3.2 Draw reference edges with cardinality at both ends, and ownership edges with a filled diamond at the owner
- [ ] 3.3 Make the two easily-missed facts visible: no capability edge touches `AgentProfile`, and `Conversation` carries materialized bindings rather than an edge back to `Pipeline`
- [ ] 3.4 Confirm no CRD fields appear anywhere on the page — kinds, edges and cardinalities only
- [ ] 3.5 Export `domain-light.svg` and `domain-dark.svg`; confirm every kind name is present as SVG text and both files are under 250 KB

## 4. Page 2 — components

- [ ] 4.1 Lay out the container boundaries left to right: external systems, adapter processes, the manager, the Kubernetes API, runtime pods
- [ ] 4.2 Nest the five manager components inside the manager boundary and nothing else from `internal/`
- [ ] 4.3 Draw every edge from the 2.2 inventory with the arrowhead on the receiver of the request, labelled with the concrete endpoint
- [ ] 4.4 Add the repeat glyph to each long-poll edge and dash the watch/informer/SSE edges
- [ ] 4.5 Count the nodes and cut to at most 24 excluding the legend; collapse each external system to one box
- [ ] 4.6 Re-read the finished page against the rule "no edge originates at the manager and terminates at an adapter" and fix any that do
- [ ] 4.7 Export `components-light.svg` and `components-dark.svg` and check size and text-as-text

## 5. Page 1 — why

- [x] 5.1 Lay out signals, the cluster boundary, the CRD declarations, the operator, and the verb ladder
- [x] 5.2 Settle the positioning: chips, headline, subhead, three differentiators (design.md D8)
- [x] 5.3 Add the real copy-pasteable `Pipeline` manifest, verified field-by-field against `config/samples/samples.yaml`
- [x] 5.4 Add the stat chips and verify every number against the repo (11 CRDs, 3 contracts, 0 Secrets, 3 bundles)
- [x] 5.5 Draw pluggability: a socket badge on each extensible part, wired to its tile
- [x] 5.6 Mark `Acts` as conditional so the page cannot imply autonomous remediation by default
- [ ] 5.7 Export `why-light.svg` and `why-dark.svg`; check size and text-as-text

## 6. Dark variants

- [x] 6.1 Add a `light|dark` switch to the generator; derive dark icons by substitution
- [x] 6.2 Verify the dark palette per cell and render a probe covering every colour pair
- [ ] 6.3 Regenerate `components` and `domain` in dark once those pages exist
- [ ] 6.4 Render all six SVGs in greyscale and confirm shape and line style alone still separate the categories

## 7. Documentation pages

- [ ] 7.1 Write `docs/architecture.md`: the two diagram embeds with one orienting paragraph each, and links into `docs/contracts.md` and `docs/concepts.md` for the detail
- [ ] 7.2 Verify `docs/architecture.md` contains no CRD field semantics, endpoint payloads, or subchart values
- [ ] 7.3 Replace the ASCII block at `README.md` lines 10–26 with the `why` `<picture>` embed and descriptive `alt` text
- [ ] 7.4 Add the `docs/architecture.md` row to the README's Documentation index
- [ ] 7.5 Run `wc -l README.md` and confirm the file is still at or under 150 lines

## 8. Freshness enforcement

- [ ] 8.1 Add `internal/integration/diagram_test.go`: read every `chart/files/crds/*.yaml`, collect `spec.names.kind`, and assert each appears as text in `docs/diagrams/domain-light.svg`
- [ ] 8.2 Make the assertion one-directional and give the failure message the list of missing kinds
- [ ] 8.3 Verify the test fails when a kind is removed from the SVG, then restore
- [ ] 8.4 Note in the test's doc comment that endpoints and call direction are governed by the `CLAUDE.md` routing rule, not by this test, and why

## 9. Contributor rules

- [ ] 9.1 Add `docs/diagrams/` to the `CLAUDE.md` map: `build-why.py` + `icons/` are SOURCE, `.drawio` and `*.svg` are OUTPUTS
- [ ] 9.2 Add the diagram row to the `CLAUDE.md` "After changes" routing table
- [ ] 9.3 Record in `CLAUDE.md` that outputs are never hand-edited, that a nudge in the app must be folded back, and that adding a CRD kind means regenerating `domain`
- [ ] 9.4 Add the documentation entry to `CHANGELOG.md` under `Unreleased` with no chart version bump

## 10. Verification

- [ ] 10.1 Run the full test suite in the container per `CLAUDE.md`, including the new diagram test
- [ ] 10.2 Resolve every relative link and anchor in `README.md`, `docs/architecture.md` and `CLAUDE.md` and confirm each target exists
- [ ] 10.3 Preview `README.md` and `docs/architecture.md` on GitHub in both themes and confirm the correct variant renders each time
- [ ] 10.4 Confirm `go build ./...`, `go vet ./...` and the chart render are unaffected — this change touches no Go source outside the new test and no chart template
- [ ] 10.5 Run `openspec validate adopter-architecture-diagram --strict`
