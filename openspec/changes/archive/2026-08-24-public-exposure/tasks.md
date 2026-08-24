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
- [x] 4.4 Every link resolves. Run them, including the site links, which fail
      until §6 enables Pages.
      **DONE, against the LIVE site.** 50 absolute links across README and every
      `docs/` page resolve, and all 13 `_data/nav.yml` entries resolve — checked
      by fetching them, not by reading them
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
      who has cloned nothing (`sdlc-setup`), verified anonymously.
      **TICKED ONCE WITHOUT BEING VERIFIED, AND REOPENED.** The images were
      public. The CHART was not checked, and checking it found the registry held
      8.0.0 and nothing since — 9, 10, 11 and 12 were never published, and there
      were no `chart-v` tags at all. The install command on the adopter site
      therefore RESOLVED, which is worse than failing: it installed a chart four
      majors behind, with the values shape from before the release-wide
      permission mode was deleted and before the vendor bundle carried its own
      image and credential.
      **VERIFIED BOTH MODES, AND THE DEMO HALF FAILED FIRST.**
      DEMO: on a cluster with NO `agentops.dev` CRDs at all — Helm installed the
      eleven itself — following the Getting started page verbatim with an
      anonymous registry config. The context claim was REFUSED by `local-path`,
      which is the only storage class rancher-desktop, k3d, kind and minikube
      ship: it serves RWO and RWOP alone. The claim sat `Pending`, no runtime
      pod was ever created, and the question waited forever. Fixed in chart
      13.0.0 — demo mode asks for `ReadWriteOnce` and the demo keeps its memory
      rather than trading it away. Re-run: claim BOUND, pod 3/3 with the
      context-sync sidecar, the page's own question answered, the answer durable
      in `status.runs[].result`.
      FULL: the four-bundle install, upgraded to 13.0.1, every pod ready, all
      four Pipelines `Ready=True`, the RWX context claim untouched — the
      immutable-accessModes hazard is demo-only, as the CHANGELOG says. Smoked
      with a live addressed task through a claiming Pipeline.
      A bare chat signal to a source TWO pipelines claim was refused with the
      choice list, which is the wiring rule holding in production rather than in
      a fixture.
- [x] 7.5 Licence, community files, templates and settings are in place.
      **THE SITE IS NOT PART OF THIS GATE, AND CANNOT BE.** This read "and the
      site is live", which no ordering can satisfy: 6.3 records that Pages is
      refused for a private repository on this plan, so the site cannot exist
      until after 7.6 — the very step this line gates. 6.3 owns it, immediately
      after the flip
- [x] 7.6 **Flip the repository to public.** Then verify as a stranger would:
      open it signed out, follow the README's install command, and follow every
      link in its index

## 8. Documentation

- [x] 8.1 **The reference docs.** `docs/CHANGELOG.md` carries 13.0.0 and 13.0.1
      — the demo access-mode fix this change's own install proof uncovered, with
      the upgrade hazard stated (a demo claim that already bound cannot be
      patched, because `accessModes` is immutable) and the fact that no ordinary
      install is affected. `docs/installation.md` names the published version
      and states what `persistence.context.accessModes` now defaults to and why
      an explicit value still wins.
- [x] 8.2 **The adopter site.** `docs/getting-started.md` no longer opens with a
      storage decision the reader has no basis to make. Its table used to send
      anyone without an RWX provisioner to `persistence.context.enabled=false`,
      which bought a working demo by removing the memory the demo exists to
      show. It now says there is nothing to decide, because demo mode asks for
      `ReadWriteOnce` — which is what `local-path` serves, and `local-path` is
      what rancher-desktop, k3d, kind and minikube ship. The failure table's RWX
      row is replaced by the one cause that survives: no provisioner at all.
      Verified by running the page verbatim on a cluster holding no CRDs.
