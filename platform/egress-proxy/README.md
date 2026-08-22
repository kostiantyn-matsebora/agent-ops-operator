# egress-proxy

Enforces a conversation's bound tool access from **inside the runtime pod**, on
traffic the agent cannot route around.

Decided in [ADR 0001](../docs/adr/0001-bound-component-reach.md).

## Why it exists

The tool allowlist reaches the agent as `--allowedTools` on a CLI running beside
it. That configures a **cooperating** agent. An agent with a shell can open a
socket to a bound MCP server and call anything that server registers, so the
allowlist binds nothing it does not choose to be bound by.

This process is the wall for an agent that does not cooperate.

## How

| Stage | What happens |
|---|---|
| `install-redirect` | A privileged init container writes an iptables REDIRECT for the agent's TCP egress, excluding the proxy's own uid. It exits before the agent starts |
| `proxy` | A native sidecar answering every redirected connection |

Three behaviours, chosen by the destination the caller intended — recovered from
the socket, never from anything the caller said:

- **A bound MCP endpoint** — parsed. `tools/call` for an ungranted tool is
  refused before the server sees it. `tools/list` is filtered to match.
- **The work contract** — forwarded verbatim, and read on the way past. This is
  where the access decision comes from.
- **Everything else** — copied through as opaque bytes. No TLS interception, no
  inspection, no new trust anchor.

## What it holds

Nothing. No Kubernetes credential, no ServiceAccount token, no RBAC, and no
allowlist at startup. The decision arrives on the work unit it is already
forwarding, which is what keeps enforcement and configuration from drifting
apart and what makes a per-conversation proxy cheap.

Before the first work unit, MCP is denied.

## What it does not cover

- **stdio MCP servers.** They are child processes of the agent container, so no
  network proxy can see them.
- **HTTPS MCP endpoints.** Enforcing one means terminating TLS inside the pod
  that runs untrusted model output. The manager reports these on the
  conversation instead — see `EgressMediated`.
- **Tool arguments.** This decides who may call what, not what they may ask for.
