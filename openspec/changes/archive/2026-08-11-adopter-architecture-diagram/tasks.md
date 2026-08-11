## 1. Vocabulary and icons

- [x] 1.1 Settle the visual vocabulary: line art, one accent, dotted zones
- [x] 1.2 Hand-draw the icon set (19 SVGs) and embed them as `data:` URIs so the file is self-contained
- [x] 1.3 Confirm the drawing has no external asset references

## 2. Source material

- [x] 2.1 Extract the signal kinds, the three declarations and the runtime facts from the code, not from prose
- [x] 2.2 Verify the `Pipeline` manifest field-by-field against `config/samples/samples.yaml` and `api/v1alpha1`
- [x] 2.3 Verify every number on the page against the repo (11 CRDs, 3 contracts, 0 Secrets, 3 bundles)
- [x] 2.4 Cross-check the claims against the `CLAUDE.md` invariants — in particular that the defaults are read-only

## 3. The diagram

- [x] 3.1 Lay out signals, the cluster boundary, the CRD declarations, the operator, and the verb ladder
- [x] 3.2 Settle the positioning: chips, headline, subhead, three differentiators (design.md D8)
- [x] 3.3 Add the real copy-pasteable `Pipeline` manifest
- [x] 3.4 Add the stat chips, including ready-made bundles
- [x] 3.5 Draw pluggability: a socket badge on each extensible part, wired to its tile
- [x] 3.6 Mark `Acts` as conditional so the page cannot imply autonomous remediation by default
- [x] 3.7 Include a non-infrastructure signal so domain range is shown, not asserted
- [x] 3.8 Correct the channel wording — it is two-way, not an outbox
- [x] 3.9 Strip the empty and rejected pages so the file carries one finished page

## 4. Verification

- [x] 4.1 Render the page and confirm no clipped text, no colliding labels, and legible type
- [x] 4.2 Confirm no Go, chart, CRD or README file was touched
- [x] 4.3 Run `openspec validate adopter-architecture-diagram --strict`
