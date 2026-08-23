# Security policy

## Supported versions

This project is `v1alpha1` and pre-1.0. **Only the most recent released chart
version, and the image tags that chart references by default, are supported.**
There are no backports to earlier chart versions — a fix ships as a new patch
release, and the upgrade path is in
[docs/CHANGELOG.md](docs/CHANGELOG.md).

| What | Supported |
|---|---|
| the latest `chart-v*` release and the images it defaults to | yes |
| any earlier chart or image tag | no — upgrade first |
| a build from `master` between releases | best effort |

## Reporting a vulnerability

**Report privately, never in a public issue.** Use GitHub's private
vulnerability reporting:

> **Security** tab → **Report a vulnerability**
> — <https://github.com/kostiantyn-matsebora/agent-ops-operator/security/advisories/new>

Blank issues are disabled and there is deliberately **no security issue
template**: a public form for a confidential report is the disclosure it exists
to prevent.

If the advisory form is unavailable to you, contact the maintainer through their
GitHub profile — [@kostiantyn-matsebora](https://github.com/kostiantyn-matsebora)
— and ask for a private channel. Do not include the detail in that first
message.

### What to include

The three things every diagnosis here starts from, plus the finding:

1. The chart version and the image tags (`helm list`, then the image refs on the
   affected pods).
2. What an attacker gains, and what access they need first — an adopter has to
   decide whether they are exposed before they can act.
3. Reproduction steps, or the manifest that demonstrates it.

### What to expect

**This project has one maintainer, so the target is one a single person can
keep:**

| Stage | Target |
|---|---|
| acknowledgement | within **7 days** |
| an assessment — accepted, needs more, or not a vulnerability | within **30 days** |
| a fix, for an accepted report | in the next release, and credited unless you ask otherwise |

A slower promise that holds is worth more than a fast one broken in public.

## Scope

**In scope** — anything in this repository: the operator, the adapters, the
runtime image, the gateway, the console, and the Helm chart's defaults. A chart
default that grants more than it needs is a finding, not a configuration
choice.

**Out of scope**

- Vulnerabilities in upstream projects the chart references (Kubernetes, the
  MCP servers, the agent CLI). Report those to their own projects; if the chart's
  default makes one reachable that would not otherwise be, that part is in scope.
- An install that grants an agent more cluster power than the defaults do.
  `allowPodExecution` and `rbacMode` are documented decisions — see
  [docs/installation.md](docs/installation.md).
- The model's output itself. An agent that says something wrong is a prompt
  problem; an agent that *reaches* something its wiring did not grant is a
  finding.
