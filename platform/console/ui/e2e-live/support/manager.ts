// A thin client of the manager's own HTTP surface — the SAME contract
// `platform/manager/test/e2e`'s Go harness drives (wiring.go's PostSignal,
// coordinator.go's coordinateCall) — used here only to ARRANGE fixtures.
// Every assertion this suite makes reads the console's rendering instead.

import { createHmac } from 'node:crypto'

/** `chat.DeriveCoordinatorToken` (platform/manager/internal/chat/token.go),
 * ported: HMAC-SHA256(masterKey, "coordinator:<coordinator>:<conversation>"),
 * base64url — the exact bytes the manager re-derives to validate a
 * coordinated conversation's own `/coordinate/*` calls. Nothing is minted or
 * stored; this just computes the same value in TypeScript. */
export function deriveCoordinatorToken(masterKey: string, coordinatorName: string, conversationName: string): string {
  return createHmac('sha256', masterKey)
    .update(`coordinator:${coordinatorName}:${conversationName}`)
    .digest('base64url')
}

export class ManagerClient {
  constructor(
    private readonly baseUrl: string,
    private readonly adapterToken: string,
  ) {}

  private async post(path: string, token: string, body: unknown): Promise<{ status: number; json: Record<string, unknown> }> {
    const res = await fetch(this.baseUrl + path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify(body),
    })
    const text = await res.text()
    let json: Record<string, unknown> = {}
    try {
      json = text ? JSON.parse(text) : {}
    } catch {
      json = { raw: text }
    }
    return { status: res.status, json }
  }

  /** POSTs one normalized signal to a SignalSource — `/signal/inbound`,
   * exactly as every real adapter does. */
  async postSignal(source: string, signal: Record<string, unknown>): Promise<Record<string, unknown>> {
    const { status, json } = await this.post('/signal/inbound', this.adapterToken, {
      source,
      signals: [signal],
    })
    if (status !== 200) throw new Error(`POST /signal/inbound (${source}): ${status} ${JSON.stringify(json)}`)
    return json
  }

  /** Posts a task-lane signal with an explicit title — the shape a machine
   * caller (cron, a webhook) uses, and the one that lets a fixture pin an
   * exact row title to test prefix-stripping against. */
  async postTask(source: string, fingerprint: string, title: string, payload: string): Promise<void> {
    await this.postSignal(source, { fingerprint, kind: 'task', title, payload })
  }

  /** Posts a chat-lane signal addressing a Pipeline or Coordinator by name —
   * `/<name> <task>` — on a chat-capable source. Discoverable addressing
   * reaches the target whether or not it claims this source. */
  async postChatCommand(source: string, channel: string, fingerprint: string, addressed: string): Promise<Record<string, unknown>> {
    return this.postSignal(source, {
      fingerprint,
      kind: 'chat',
      payload: addressed,
      labels: { 'agentops.dev/channel': channel, 'agentops.dev/sender': 'e2e-ui' },
    })
  }

  /** `/coordinate/invoke`, bearer-authenticated as the CALLER conversation —
   * the same path `platform/mcp-aops` forwards a tool call through. */
  async invoke(coordinatorName: string, caller: string, agent: string, task: string): Promise<string> {
    const token = deriveCoordinatorToken(this.adapterToken, coordinatorName, caller)
    const { status, json } = await this.post('/coordinate/invoke', token, { conversation: caller, agent, task })
    if (status !== 200) throw new Error(`invoke(${agent}) from ${caller}: ${status} ${JSON.stringify(json)}`)
    const member = json.member
    if (typeof member !== 'string' || !member) throw new Error(`invoke(${agent}): no member name in ${JSON.stringify(json)}`)
    return member
  }
}
