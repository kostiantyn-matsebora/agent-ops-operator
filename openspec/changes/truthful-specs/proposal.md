# Truthful specs: every published spec describes the system as it is

## Why

`openspec/specs/` is published with the repository. It claims to describe the
current system, and in places it describes one that was replaced.

**Two published specs contradict each other today:**

| Spec | Says |
|---|---|
| `pipeline-model` | the "oldest claimant" tiebreak **is REMOVED** |
| `channel-type-model` | a channel's default profile "comes from its **oldest Ready Pipeline**" |

The second also asserts `spec.type`, `spec.delivery` and `defaultProfileRef` on
the `Channel` CRD. The API type carries `Adapter`, `CredentialsSecretRef` and
`Config` — and nothing else. A live `#### Scenario` documents behaviour that
does not exist.

A stranger reading both learns that the project disagrees with itself, and the
half that is wrong describes a rule this repository's own context calls a
regression to re-add.

**Measured drift**, from a scan of all 59 capabilities:

| Kind | Extent |
|---|---|
| `Purpose: TBD - created by archiving change <name>` | **23 of 59 specs** — template text, never filled in |
| Retired CRD fields asserted as current | `spec.type` ×3, `spec.delivery` ×1, `defaultProfileRef` ×1 |
| Retired vocabulary presented as the interface | the retired listing command ×2 |
| Renamed things | the metrics bundle's old name ×2, a module path that moved ×2 |

No spec is thin or empty — 10,350 lines across 385 requirements and 1,247
scenarios. The problem is not padding, it is **currency**.

## What Changes

- **Every requirement gets a verdict against the code**: keep, correct, or
  delete. 385 of them, ledgered per capability so completion is provable rather
  than asserted.
- **The 23 placeholder purposes** are written, or the capability is merged into
  the one that absorbed it. A `Purpose` that says TBD is not a summary, it is a
  confession that nobody wrote one.
- **Contradictions are resolved in favour of the code**, and the losing spec
  says what replaced it rather than quietly dropping the claim.
- **A retired-vocabulary guard in CI.** Unlike the identity guard, this one is a
  DENYLIST and that is correct: `spec.type`, `defaultProfileRef`, the removed
  tiebreak and the retired command are public names, and the point is to keep
  them from coming back.

## Impact

- **No behaviour changes.** This change edits documents to match code that is
  already correct; where a spec and the code disagree, the code is right by
  definition and the spec is the defect.
- **If the audit finds a requirement the code does NOT satisfy**, that is a bug
  discovered by this change. It is recorded and raised as its own change — never
  fixed by weakening the spec to match, which would launder a defect into a
  decision.
- **Affected specs**: potentially all 59. `documentation-structure` gains the
  durable requirement; per-capability corrections are produced BY the audit and
  folded in as they land.
- **Independent of `scrub-identity`** — this touches wording, not identifiers,
  and `openspec/specs/` is already clean of every identity class. The two can
  run in parallel and neither blocks the other.

## Out of scope

- **`openspec/changes/archive/`.** It is a historical record. A change that
  argued for something later reversed stays as it was argued — the reversal is
  the most useful thing the record holds. Only identifiers are substituted
  there, and `scrub-identity` owns that.
- **Rewriting requirements to be better.** The test is "is this true", not "is
  this well put". A true requirement stays as written.
