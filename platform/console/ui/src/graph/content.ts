import type { ActivityEvent, ConversationDetail } from '../api/types'

// WHAT A HOP CARRIED, joined in the browser.
//
// The event keeps ids and bounded facts. Everything with a durable home — an
// input's text, a run's result, an op's message — is read from the
// conversation's own status by conversation, run, op and input id, never from
// the event. The buffer stays lossy and carries no content, and what is shown
// is what the cluster recorded.

export interface Carried {
  rows: [string, string][]
  /** The one long thing a hop carried, from its durable home. */
  body?: { label: string; text: string; truncated?: boolean }
  /** Where the body came from, so a missing one is explained rather than blank. */
  note?: string
}

interface RecordedInput {
  id: string
  text?: string
  truncated?: boolean
  surface?: string
  sender?: string
  receivedAt?: string
}

interface RunView {
  runId: string
  status?: string
  exitCode?: number
  result?: string
  finishedAt?: string
  delivered?: string[]
  inputIds?: string[]
  inputs?: RecordedInput[]
}

interface ConversationView {
  spec?: { inputs?: { id: string; type?: string; payload?: string; receivedAt?: string }[] }
  status?: { runs?: RunView[]; runtimeContextId?: string; sessionId?: string }
}

/** A send op's id is `send:<conversation>:<channel>:<runId>`. */
export function parseOpId(opId?: string): { kind: string; channel?: string; runId?: string } | undefined {
  if (!opId) return undefined
  const parts = opId.split(':')
  if (parts[0] === 'send' && parts.length >= 4) {
    return { kind: 'send', channel: parts[parts.length - 2], runId: parts[parts.length - 1] }
  }
  return { kind: parts[0] }
}

const DATA_LABELS: Record<string, string> = {
  model: 'Model', tokensIn: 'Tokens in', tokensOut: 'Tokens out', cacheReadTokens: 'Cache reads',
  stopReason: 'Stop reason', tool: 'Tool', server: 'Target', resultBytes: 'Result bytes',
}

export function hopContent(ev: ActivityEvent, detail?: ConversationDetail): Carried {
  const rows: [string, string][] = []
  const shown = new Set<string>()
  const datum = (key: string) => {
    const v = ev.data?.[key]
    if (v === undefined) return
    shown.add(key)
    rows.push([DATA_LABELS[key] ?? key, v])
  }
  const obj = detail?.object as unknown as ConversationView | undefined
  const runs = obj?.status?.runs ?? []
  const summary = detail?.conversation
  const inputById = (id?: string): RecordedInput | undefined => {
    if (!id) return undefined
    for (const r of runs) for (const i of r.inputs ?? []) if (i.id === id) return i
    const q = obj?.spec?.inputs?.find((i) => i.id === id)
    return q ? { id: q.id, text: q.payload, receivedAt: q.receivedAt } : undefined
  }
  const runById = (id?: string) => (id ? runs.find((r) => r.runId === id) : undefined)
  let body: Carried['body']
  let note: string | undefined
  const noStatus = 'This conversation\'s status is not loaded.'

  switch (ev.kind) {
    case 'signal.received':
    case 'signal.claimed':
    case 'signal.dropped':
      if (ev.code) rows.push(['Signal', ev.code])
      if (ev.detail) rows.push(['Detail', ev.detail])
      break
    case 'input.queued':
    case 'channel.inbound':
    case 'conversation.created': {
      if (ev.inputId) rows.push(['Input', ev.inputId])
      const input = inputById(ev.inputId)
      if (input?.sender) rows.push(['Sender', input.sender])
      if (input?.surface) rows.push(['Surface', input.surface])
      if (input?.text !== undefined) body = { label: 'Input', text: input.text, truncated: input.truncated }
      else note = detail ? 'The input is no longer on the conversation.' : noStatus
      break
    }
    case 'run.dispatched': {
      if (ev.runId) rows.push(['Run', ev.runId])
      if (summary?.toolsets?.length) rows.push(['Toolsets', summary.toolsets.join(', ')])
      if (summary?.mcpConfigs?.length) rows.push(['MCP configs', summary.mcpConfigs.join(', ')])
      const handle = obj?.status?.runtimeContextId || obj?.status?.sessionId
      if (handle) rows.push(['Context handle', handle])
      const run = runById(ev.runId)
      const texts = (run?.inputs ?? []).map((i) => i.text ?? '').filter(Boolean)
      if (texts.length) body = { label: 'Inputs', text: texts.join('\n\n') }
      if (!detail) note = noStatus
      break
    }
    case 'run.completed': {
      if (ev.runId) rows.push(['Run', ev.runId])
      const run = runById(ev.runId)
      if (run) {
        if (run.status) rows.push(['Status', run.status])
        if (run.exitCode !== undefined) rows.push(['Exit', String(run.exitCode)])
        if (run.finishedAt) rows.push(['Finished', run.finishedAt])
        if (run.result !== undefined) body = { label: 'Result', text: run.result }
      } else {
        if (ev.detail) rows.push(['Exit', ev.detail])
        note = detail ? 'This run is not on the conversation\'s status.' : noStatus
      }
      break
    }
    case 'channel.op.enqueued':
    case 'channel.op.completed': {
      const op = parseOpId(ev.opId)
      if (ev.opId) rows.push(['Op', ev.opId])
      if (ev.code) rows.push(['Message type', ev.code])
      if (ev.to?.kind === 'channel') rows.push(['Channel', ev.to.name])
      const run = runById(op?.runId)
      if (ev.kind === 'channel.op.completed') {
        rows.push(['Report', ev.status === 'error' ? `failed${ev.detail ? `: ${ev.detail}` : ''}` : 'delivered'])
        if (ev.adapter) rows.push(['Adapter', ev.adapter])
      }
      if (run && op?.channel) rows.push(['Recorded delivered', (run.delivered ?? []).includes(op.channel) ? 'yes' : 'no'])
      if (op?.kind === 'send' && run?.result !== undefined) body = { label: 'Body', text: run.result }
      else if (op?.kind === 'send') note = detail ? 'The run this op sends is not on the conversation\'s status.' : noStatus
      break
    }
    case 'model.call':
      for (const k of ['model', 'tokensIn', 'tokensOut', 'cacheReadTokens', 'stopReason']) datum(k)
      break
    case 'tool.call':
      for (const k of ['tool', 'server', 'resultBytes']) datum(k)
      if (!ev.to) rows.push(['Target', 'built-in'])
      if (ev.latencyMs) rows.push(['Duration', `${ev.latencyMs}ms`])
      break
    default:
      if (ev.detail) rows.push(['Detail', ev.detail])
  }
  for (const [k, v] of Object.entries(ev.data ?? {})) if (!shown.has(k)) rows.push([DATA_LABELS[k] ?? k, v])
  return { rows, body, note }
}

/** One line for a feed row. */
export function hopSummary(ev: ActivityEvent): string {
  const d = ev.data ?? {}
  if (ev.kind === 'model.call') {
    const parts = [d.model, d.tokensIn && `${d.tokensIn} in`, d.tokensOut && `${d.tokensOut} out`, d.stopReason]
    return parts.filter(Boolean).join(' · ')
  }
  if (ev.kind === 'tool.call') return [d.tool, d.server ?? (ev.to ? undefined : 'built-in')].filter(Boolean).join(' · ')
  return ev.detail ?? ''
}
