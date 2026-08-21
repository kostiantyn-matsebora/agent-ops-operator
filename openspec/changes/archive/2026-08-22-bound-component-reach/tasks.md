## 1. Opt-in surface

- [x] 1.1 Add the egress-mediation stanza to `AgentRuntime` in `api/v1alpha1/`, absent-means-today, and verify `go build ./... && go vet ./...` passes
- [x] 1.2 Regenerate deepcopy and CRDs with controller-gen and verify `chart/files/crds/` diff contains only the new optional field
- [x] 1.3 Add a unit test asserting a runtime with no stanza produces today's pod byte-for-byte (containers, env, security context), so the absent case can never drift

## 2. The proxy module

- [x] 2.1 Create `egress-proxy/` as a dependency-free Go module with its own `go.mod`, and verify `go build ./... && go vet ./... && go test ./...` passes inside it
- [x] 2.2 Implement destination recovery for redirected connections and opaque TCP forwarding, and verify with a test that a non-MCP connection is byte-identical end to end including a streamed response
- [x] 2.3 Implement pattern matching for the `mcp__<server>__<tool>` convention — literal and trailing `*` — and verify with a table test covering both forms and a non-match
- [x] 2.4 Implement qualified tool-name composition from server key plus local tool name (design D5) and verify against fixtures drawn from the shipped k8s and HA toolsets
- [x] 2.5 Implement MCP request enforcement — refuse an ungranted `tools/call` with an MCP-level error, never a transport failure — and verify with tests covering granted, ungranted and unknown tools
- [x] 2.6 Filter `tools/list` responses with the predicate that gates invocation, and verify a test asserts listing and invocation agree for the same binding
- [x] 2.10 Log what the binding resolved to at the filtered listing — N of M granted, and a warning when nothing is — and verify a test covers the granted-nothing case, which is what a mistyped pattern looks like
- [x] 2.7 Learn the access decision from the work unit in flight (design D3) and verify a test shows a mid-conversation toolset change taking effect on the next unit with no restart
- [x] 2.8 Deny MCP before the first work unit is seen, and verify a test asserts the closed initial state
- [x] 2.9 Report an unenforceable bound endpoint (design D7) and verify a test covers an `https` MCP URL producing a report rather than pass-through

## 3. Runtime pod wiring

- [x] 3.1 Add the interception init container to the pod builder, privilege confined to startup, and verify a builder test asserts the agent container holds no such capability
- [x] 3.2 Add the proxy as a native sidecar following the `context-sync` pattern, and verify a test asserts the ordering that makes fail-closed true (design D2)
- [x] 3.3 Harden the agent container — non-root identity distinct from the proxy's, no privilege escalation, no capabilities — and verify a test pins all three, so soundness stops being incidental
- [x] 3.4 Pass the bound MCP endpoints to the proxy at pod creation so it can tell mediated destinations from pass-through, and verify a builder test covers a conversation with two bound servers
- [x] 3.5 Cover IPv6 in interception, and verify a test asserts both families are redirected — single-family interception is a bypass, not a limitation

## 4. Manager-side reporting

- [x] 4.1 Surface the proxy's unenforceable-endpoint report as a conversation condition, and verify an envtest asserts the condition appears and names the endpoint
- [x] 4.2 Surface proxy failure as a conversation condition rather than reduced tooling, and verify an envtest covers the failure path

## 5. Network restriction in the chart

- [x] 5.1 Add default-deny ingress plus per-flow allows for manager, adapters, console and runtime pods, keyed on existing labels, and verify `helm template` renders no policy by default and a complete set when enabled
- [x] 5.2 Add values for additional permitted callers and verify a render test covers a collector and an ingress controller from another namespace
- [x] 5.3 Add policies for the MCP server workloads in `k8s-bundle` and `ha-bundle`, reading the decision from global scope, and verify each subchart renders them independently
- [x] 5.4 Add a chart render test pinning every wired flow's allow rule, so tightening the policy cannot silently sever one
- [x] 5.5 Write the post-install warning (design D9) — unenforced objects protect nothing, what stays reachable, how to check — and verify a render test asserts it appears both when enabled and when disabled

## 6. Images

- [x] 6.1 Build and push `agentops-egress-proxy` multi-arch, and verify with `docker manifest inspect` that both architectures are present
- [x] 6.2 Wire the image reference through chart values and the pod builder, and verify a render test pins the default tag

## 7. Documentation

- [x] 7.1 Update `docs/concepts.md` so capability resolution names the enforcement distinction (spec `mcp-toolset-model`), and verify the page no longer describes the CLI allowlist as the boundary
- [x] 7.2 Update `docs/k8s-bundle.md` and `docs/ha-bundle.md` to qualify the two-walls claim, and verify both state what remains when an agent has shell
- [x] 7.3 Document both decisions in `docs/installation.md` — what enabling mediation requires of the namespace, and what network restriction does and does not guarantee — and verify the adopter-prose lint passes
- [x] 7.4 Add the CHANGELOG entry with upgrade steps for both opt-ins, newest first
- [x] 7.5 Move `docs/adr/0001-bound-component-reach.md` to Accepted and record anything the implementation forced to change

## 8. Verification

- [x] 8.1 Add an integration test proving a shell-capable agent cannot reach a bound MCP server unmediated, which is the whole point of the change
- [x] 8.2 Run the full suite including envtest, and every module's own `go build ./... && go vet ./... && go test ./...`
- [x] 8.3 Verify interception on the **nft** backend (debian:bookworm-slim, iptables v1.8.9 nf_tables): rules land in AGENTOPS_EGRESS in the correct order, exclusions before the catch-all. Verified a backend mismatch FAILS LOUDLY (exit 1, kubelet-visible) rather than no-opping
- [x] 8.5 Verified the **legacy** backend on a cluster node (kernel 6.8.0-138-generic, x86_64, `ip_tables`+`iptable_nat` loaded): `iptables v1.8.9 (legacy)`, installer exit 0, all five rules land in the legacy nat table in the correct order. The Rancher Desktop VM cannot test this — its module tree ships no `ip_tables` at all, so enabling Kubernetes there would not change that
- [x] 8.4 Live: both halves enabled on the reference install. Five surfaces confirmed unreachable from an unwired pod (both MCP servers, manager /work, console API, telegram adapter). Conversations answer normally. A shell in a runtime pod calling its bound MCP server directly was REFUSED by the proxy, and its tool listing filtered to 10 of 17. Live testing found four defects the suite could not: address-based endpoint matching (socket-LB rewrites the destination), a blocking bufio.Peek that stalled the work contract, a kubelet probe blocked by policy, and the same on the MCP server's shared port
