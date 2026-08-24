# Tasks — public exposure

The last task flips a switch that cannot be flipped back. Everything above it
exists so that the switch is safe, and §7 is the gate that says so.

## 1. Licence

- [x] 1.1 Add `LICENSE` — Apache-2.0, verbatim, with the copyright line filled
      in. Verbatim matters: a modified licence text is a licence nobody's legal
      review recognises
- [x] 1.2 Replace "License TBD" in the README's status section with the licence,
      and correct the roadmap line, which still promises a chart that shipped

## 2. Community health files

- [x] 2.1 `SECURITY.md`: which versions are supported, the private advisory
      route, and an acknowledgement target a single maintainer can keep. A
      policy promising more than one person can deliver is broken in public
- [x] 2.2 `CODE_OF_CONDUCT.md`: Contributor Covenant by reference, with the
      reporting route and who enforces it. By reference, not copied — a
      vendored copy silently forks from its upstream
- [x] 2.3 `CONTRIBUTING.md`: how a change is proposed HERE. The openspec
      workflow, the commit convention, the build and test commands, and the fact
      that documentation is part of a change rather than a follow-up. A
      contributor cannot infer any of it
- [x] 2.4 Cross-link the three from the README's index, one line each

## 3. Issue and pull request templates

- [x] 3.1 `.github/ISSUE_TEMPLATE/config.yml`: blank issues disabled, and a
      contact link routing security reports to private advisories
- [x] 3.2 A bug report template that asks for the chart version, the image tags,
      and the condition on the object — the three things every diagnosis in this
      repository has started from
- [x] 3.3 A feature request template that asks what the reader is trying to do
      before what they want built
- [x] 3.4 NO security template — see `design.md` D5. The route is the contact
      link, because a public form for a confidential report is the disclosure it
      exists to prevent
- [x] 3.5 `PULL_REQUEST_TEMPLATE.md`: what changed, what it affects, and whether
      the documentation the change made untrue was updated in the same commit

## 4. The two-minute README

- [x] 4.1 Settle the open question in `design.md`: whether the CRD kind table
      stays. Decide it against the two-minute promise, not by preference
- [x] 4.2 Rewrite to the four questions: what this is, whether it is real, how
      to try it, where to go next. The site owns everything else
- [x] 4.3 The install command resolves a PUBLISHED artifact. This task cannot
      complete before `sdlc-setup` publishes the chart — see `design.md` D3
- [ ] 4.4 Every link resolves. Run them, including the site links, which fail
      until §6 enables Pages
- [x] 4.5 `wc -l README.md` is within budget, and nothing that moved out was
      dropped rather than linked

## 5. Hygiene

- [x] 5.1 Remove the committed build artifact from the tree. The git OBJECT
      survives until history is rewritten — do not add a second rewrite for it;
      `scrub-identity` is already doing one, and this file being absent from the
      tree beforehand is what lets that pass drop it
- [x] 5.2 Correct the ignore list against the CURRENT tree. It names modules by
      their pre-restructure paths, which is why the artifact was committable —
      derive the entries from the component layout rather than listing them
- [x] 5.3 Add `.gitattributes` normalising line endings to LF. Endings flipped
      under editing twice in one week here, and a public repository adds
      contributors on platforms where the default differs
- [x] 5.4 Confirm the packed repository size after §5.1 and the rewrite, so the
      artifact is known to be gone rather than assumed

## 6. Repository settings and the site

- [x] 6.1 Description and topics: what it is, and the words someone searching
      for this would use — the ecosystem, the runtime, the surfaces
- [x] 6.2 Homepage set to the documentation site
- [x] 6.3 Enable Pages from `master` `/docs`, and verify the site builds and
      every navigation entry resolves. **CANNOT PRECEDE THE FLIP ON THIS PLAN** —
      the API refuses with "your current plan does not support GitHub Pages for
      this repository", because Pages is unavailable for a PRIVATE repository on
      Free. See `design.md` D4a: this runs IMMEDIATELY AFTER 7.6, not before.
      **DONE** — source `master` `/docs`, serving at
      `kostiantyn-matsebora.github.io/agent-ops-operator/`, HTTPS enforced
- [x] 6.4 Discussions on, Issues on, Wiki off, Projects off. An unmaintained
      surface reads as abandonment
- [x] 6.5 Branch protection on `master` matching how the project actually works,
      rather than a policy that will be bypassed on the first hotfix. **ALSO
      PLAN-BLOCKED WHILE PRIVATE** (HTTP 403, "upgrade to GitHub Pro or make this
      repository public"), so it runs immediately after 7.6 too. The shape is
      SETTLED and is in `design.md` D4a — deletion blocked, **`ci-green` required
      and nothing else**, admins exempt so direct commits to `master` keep
      working, no required reviews because a solo maintainer cannot
      self-approve, and force pushes left ALLOWED until the flip is done.
      **REQUIRING THE SIX INDIVIDUAL JOBS WOULD NOW BREAK PULL REQUESTS**, which
      is why the shape changed rather than merely aged: CI builds only what
      changed, a skipped job never reports its name, and a rule requiring one
      waits forever — on the cheapest pull requests, the documentation-only ones

## 7. The gate

Each line is a condition that becomes impossible or expensive after the flip.

- [x] 7.1 The publication guard is green on `master` (`sdlc-setup` §9)
- [x] 7.2 Identifying content is absent from the tree, the archive AND the
      history, and the rewrite has been force-pushed (`scrub-identity`)
- [x] 7.3 The published specs are true (`truthful-specs`) — a stranger reading a
      contradiction reads it once and remembers it
- [x] 7.4 Images are public and the chart installs from the registry by a person
      who has cloned nothing (`sdlc-setup`), verified anonymously
- [x] 7.5 Licence, community files, templates and settings are in place.
      **THE SITE IS NOT PART OF THIS GATE, AND CANNOT BE.** This read "and the
      site is live", which no ordering can satisfy: 6.3 records that Pages is
      refused for a private repository on this plan, so the site cannot exist
      until after 7.6 — the very step this line gates. 6.3 owns it, immediately
      after the flip
- [x] 7.6 **Flip the repository to public.** Then verify as a stranger would:
      open it signed out, follow the README's install command, and follow every
      link in its index
