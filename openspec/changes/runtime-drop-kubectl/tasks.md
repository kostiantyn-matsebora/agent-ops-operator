> **Entry gate (design D1).** Do not start until the MCP path from
> `k8s-mcp-tooling` has answered real cluster questions in a live cluster without
> falling back to kubectl. This is the one irreversible step in the arc; shipping
> it on the strength of the design alone risks an image that cannot inspect a
> cluster. If the gate fails, narrow or abandon this change — the earlier two
> still stand on their own.

## 1. Confirm the gate

- [ ] 1.1 Exercise `mcp__kubernetes__*` tools against a live cluster on the questions the demo advisor is documented to answer (crashlooping pods, node pressure, failed workloads); record what worked and what needed kubectl
- [ ] 1.2 Decide go/no-go from 1.1 and record it in `design.md` — including a no-go, so the reasoning survives

## 2. Runtime image

- [ ] 2.1 `runtime-claude/Dockerfile`: remove the kubectl download layer; confirm nothing else in the image or `runtime.js` invokes it
- [ ] 2.2 Bump the image tag (never overwrite a pushed tag) and build for linux/amd64

## 3. Manager-side assumption

- [ ] 3.1 `internal/dispatch/templates/task.md`: replace the "e.g. `kubectl` under this pod's ServiceAccount" wording with tool-agnostic guidance (use what the allowlist grants; observe before acting) — it ships to every profile, including agents with no Kubernetes involvement
- [ ] 3.2 Update the `internal/dispatch` fixtures pinning that template, deliberately per the repo rule that dispatch semantics change by changing tests on purpose
- [ ] 3.3 Grep the repo for any remaining assumption that the runtime has kubectl (prompts, templates, docs, samples)

## 4. Docs

- [ ] 4.1 README: the runtime is now generic (vendor × trust level only); the derived-image escape hatch from design D3 verbatim, so operators who need kubectl have a supported path
- [ ] 4.2 README + `k8s-bundle` values: rewrite the `rbac.mode` story as two walls (MCP server identity + tool allowlist) rather than "RBAC is the only wall"; state that cluster reach now requires the MCP component
- [ ] 4.3 CLAUDE.md: `runtime-claude/` map entry drops kubectl and gains the genericity note
- [ ] 4.4 Decide the design's open question — NOTES.txt warning vs render failure for `rbac.mode: full` with MCP disabled — and implement the choice
- [ ] 4.5 Release notes: mark BREAKING, name the previous image tag as the hold position (`AgentRuntime.spec.image` makes rollback a one-line change)

## 5. Verification

- [ ] 5.1 `go build ./... && go vet ./...`, envtest suite with `KUBEBUILDER_ASSETS`, `helm lint` + template smoke
- [ ] 5.2 Live check: a bundle install on the new image answers a cluster question end to end through MCP, and the runtime pod has no kubectl binary
