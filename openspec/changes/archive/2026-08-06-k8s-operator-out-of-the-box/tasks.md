# Tasks: k8s-operator-out-of-the-box

## 1. Runtime image

- [ ] 1.1 Resolve and pin the `mcp-server-kubernetes` version; verify its non-destructive env flag name against that version's docs
- [ ] 1.2 Create `runtime-k8s/Dockerfile` (`FROM` pinned `agentops-runtime-claude`, baked MCP install, back to `USER node`); build `agentops-runtime-k8s:0.1.0` and verify in-container: `mcp-server-kubernetes` on PATH, kubectl present, `runtime.js` entrypoint intact
- [ ] 1.3 Push the image

## 2. Chart bundle

- [ ] 2.1 Add `k8sOperator.*` values (enabled true, fullAccess false with a loud comment, runtimeImage, maxTurns, credentialsSecret) to `values.yaml`
- [ ] 2.2 Add `chart/templates/k8s-operator.yaml`: dedicated SA, AgentRuntime (image/SA/TTL/home/credential env), AgentProfile (inline stdio MCP, mode-aware non-destructive env, allowlist, runtimeRef), RBAC — read-only bindings (`view` + cluster-ro incl. agentops.dev reads) XOR cluster-admin binding per `fullAccess`
- [ ] 2.3 Chart minor bump; `helm lint` + `helm template` across default / `enabled=false` / `fullAccess=true`, asserting: bundle present by default, absent when disabled, binding swap + MCP env flip on fullAccess, SA never the shared runtime SA

## 3. Verification, docs, live

- [ ] 3.1 Live on home-data-center: `helm upgrade` (bundle appears; agentops-claude Secret already present from demo values shape), then a real `POST /task {"profile":"k8s-operator","task":"..."}` small task verifying MCP + read-only rights (a mutation attempt is denied); confirm `enabled=false` removal path with `helm template` only
- [ ] 3.2 README (out-of-the-box section replacing/annotating the demo pitch, fullAccess warning, credential prerequisite) + CLAUDE.md (map: `runtime-k8s/`, build line, image list)
