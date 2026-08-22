# Design — truthful specs

## The size is the point, not an obstacle

385 requirements across 59 capabilities. Reading all of them is the only thing
that establishes the contract still matches the system after a directory
restructure, a component rename, several deletions and a year's worth of
reversals.

A sample would tell us the rate of drift. It would not tell us **which**
requirements are wrong, and a spec that is wrong in an unknown place is worth
less than no spec, because it is trusted.

So the design question is not how to avoid the work. It is how to make the work
**provable** — how a reader six months from now can tell that every requirement
was looked at rather than most of them.

## D1. A ledger, one line per capability

`tasks.md` carries one checkbox per capability, not per requirement. 59 lines,
each of which is "this capability's requirements have verdicts".

Per-requirement checkboxes would be 385 lines nobody reads and everybody ticks
in batches. Per-capability is the largest unit that is still honestly
completable in one sitting, and the boundary at which a reader can go and check.

## D2. Three verdicts, and the code decides

Each requirement is one of:

| Verdict | Means |
|---|---|
| **keep** | the code does this |
| **correct** | the code does something else, and the spec is the defect |
| **delete** | the code does not do this and should not — the requirement describes a removed capability |

**The code is right by definition here.** This change edits documents; it does
not change behaviour.

**The exception is the finding that matters most.** If the code does not satisfy
a requirement and *should*, that is a BUG this audit discovered. It is recorded
and raised as its own change — never resolved by weakening the requirement to
match what the code happens to do. That move launders a defect into a decision,
and it is the specific failure a spec audit is prone to.

## D3. A mechanical pre-pass, then reading

Grep produces candidates. Reading produces verdicts. The distinction is not
pedantic — the same string is correct in one spec and wrong in another:

```
  pipeline-model      "the oldest-claimant tiebreak is REMOVED"      ✓ keep
  channel-type-model  "comes from its oldest Ready Pipeline"         ✗ correct
```

Same phrase, opposite verdicts, because one documents a removal and the other
asserts the removed thing. No pattern distinguishes them. So the pre-pass ranks
capabilities by suspicion and nothing more.

## D4. This guard is a DENYLIST, and that is the right shape

`scrub-identity` argues an allowlist, because naming a private thing in the
guard publishes it.

Here the opposite holds. Retired vocabulary — a removed CRD field, a withdrawn
tiebreak, a superseded command — is **public by construction**. Listing it is
free, and the list is exactly the value: it is the record of what this project
stopped doing, in the one place that fails a build when someone reintroduces it.

Two guards, opposite shapes, each following from what its subject is. Neither is
a template for the other.

## D5. A placeholder Purpose is a defect, not a formatting nit

23 of 59 capabilities open with `Purpose: TBD - created by archiving change
<name>`. That is the scaffolding tool's text, surviving the archive that created
the capability.

It is worth fixing for a reason beyond tidiness: a capability whose purpose
nobody wrote is a capability nobody has had to justify the boundary of. Several
of the 23 are console capabilities split fine — writing their purposes is the
cheapest way to discover that two of them are one.

**So the task is "write the purpose OR merge the capability",** and the second
outcome is a success, not a failure of the first.

## D6. Order by consequence, not by alphabet

The audit starts where a wrong spec costs the most:

1. **The contradiction** — one spec asserts what another says was removed.
2. **CRD surface** — a spec naming a field that does not exist misleads anyone
   writing a manifest, which is the first thing an adopter writes.
3. **Contracts** — the work, channel and signal contracts are what a third-party
   implementer builds against. Wrong here is wrong in someone else's code.
4. **Everything else**, by requirement count descending, so the large specs are
   done while attention is freshest.

## D7. A capability with no true requirements is DELETED

`openspec/specs/` describes what IS. A capability whose requirements are all
false describes nothing, and a note saying so is not a specification.

The trace is not lost: the archive holds why it existed and the change that
retired it. Keeping an empty capability publishes a heading that has to be read
before it can be dismissed.
