// Thin kubectl wrapper for arranging fixtures against the real cluster —
// never for assertions (those go through the console UI, per this suite's
// own black-box rule). Mirrors what `platform/manager/test/e2e`'s Go harness
// does with its typed client, just shelled out: this package has no
// client-go, and kubectl is already authenticated against whatever cluster
// E2E_LIVE_KUBE_CONTEXT names.

import { execFileSync, spawn, type ChildProcess } from 'node:child_process'
import { accessSync, constants } from 'node:fs'
import path from 'node:path'

/**
 * kubectl resolved ONCE to an absolute path, searching only the absolute
 * entries of PATH, so a relative or empty PATH entry (the current directory)
 * can never supply the binary. `E2E_LIVE_KUBECTL` overrides the search.
 */
function resolveKubectl(): string {
  const override = process.env.E2E_LIVE_KUBECTL
  if (override) return override
  for (const dir of (process.env.PATH ?? '').split(path.delimiter)) {
    if (!path.isAbsolute(dir)) continue
    const candidate = path.join(dir, 'kubectl')
    try {
      accessSync(candidate, constants.X_OK)
      return candidate
    } catch {
      // not in this directory
    }
  }
  throw new Error('kubectl not found in an absolute PATH entry (set E2E_LIVE_KUBECTL to its full path)')
}

const KUBECTL = resolveKubectl()

export const KUBE_CONTEXT = process.env.E2E_LIVE_KUBE_CONTEXT ?? 'k3d-agentops-e2e'
export const NAMESPACE = process.env.E2E_LIVE_NAMESPACE ?? 'agent-ops'

function kubectlBaseArgs(): string[] {
  return ['--context', KUBE_CONTEXT, '-n', NAMESPACE]
}

/** Runs kubectl, returns stdout. Throws with stderr attached on failure. */
export function kubectl(args: string[], input?: string): string {
  try {
    return execFileSync(KUBECTL,[...kubectlBaseArgs(), ...args], {
      input,
      encoding: 'utf8',
      maxBuffer: 64 * 1024 * 1024,
    })
  } catch (e) {
    const err = e as { stderr?: Buffer | string; message: string }
    const stderr = err.stderr ? err.stderr.toString() : ''
    throw new Error(`kubectl ${args.join(' ')} failed: ${err.message}\n${stderr}`)
  }
}

export function kubectlApply(yaml: string): void {
  kubectl(['apply', '-f', '-'], yaml)
}

export function kubectlDelete(kind: string, name: string): void {
  // --ignore-not-found: a fixture the suite already deleted (a re-run after a
  // partial failure) must not fail setup on a second attempt.
  kubectl(['delete', kind, name, '--ignore-not-found'])
}

export function getJSON<T>(kind: string, name?: string): T {
  const args = name ? ['get', kind, name, '-o', 'json'] : ['get', kind, '-o', 'json']
  return JSON.parse(kubectl(args)) as T
}

export function getSecretValue(secretName: string, key: string): string | undefined {
  try {
    const b64 = kubectl(['get', 'secret', secretName, '-o', `jsonpath={.data.${key}}`]).trim()
    if (!b64) return undefined
    return Buffer.from(b64, 'base64').toString('utf8')
  } catch {
    return undefined
  }
}

interface ConversationObject {
  metadata: { name: string; creationTimestamp: string }
  spec: {
    coordinatorRef?: { name: string }
    causedBy?: { parent: string; entry: string }
    pipelineRef?: { name: string }
    title?: string
  }
  status?: { phase?: string; runs?: { finishedAt?: string }[] }
}

/** Lists every Conversation CR as plain objects — read-only, for LOCATING a
 * fixture this suite just created (a generated name, a causedBy member), not
 * for asserting on. Assertions read the console's own rendering. */
export function listConversations(): ConversationObject[] {
  return getJSON<{ items: ConversationObject[] }>('conversations.agentops.dev').items
}

/** Polls until `find` returns a match, or throws after `timeoutMs`. */
export async function waitFor<T>(
  what: string,
  find: () => T | undefined,
  timeoutMs = 120_000,
  intervalMs = 2000,
): Promise<T> {
  const deadline = Date.now() + timeoutMs
  let lastErr: unknown
  for (;;) {
    try {
      const found = find()
      if (found !== undefined) return found
    } catch (e) {
      lastErr = e
    }
    if (Date.now() > deadline) {
      const cause = lastErr instanceof Error ? lastErr.message : String(lastErr ?? '')
      const suffix = cause ? `: ${cause}` : ''
      throw new Error(`timed out after ${timeoutMs}ms waiting for ${what}${suffix}`)
    }
    await new Promise((r) => setTimeout(r, intervalMs))
  }
}

/**
 * `metadata.creationTimestamp` is RFC3339 at SECOND precision — Kubernetes
 * truncates, never rounds. A `Date.now()`-derived cutoff carries
 * milliseconds, so an object created in the same wall-clock second as the
 * cutoff but a few hundred ms EARLIER serializes to a timestamp that reads
 * as before it, even though nothing was actually out of order. Flooring the
 * cutoff to the second it falls in removes that false exclusion — measured
 * live: a root created 10ms after its cutoff was invisible to this
 * comparison for a full two-minute poll until this floor was added.
 */
function flooredToSecond(d: Date): number {
  return Math.floor(d.getTime() / 1000) * 1000
}

export function findConversationByCoordinatorRoot(coordinatorName: string, after: Date): ConversationObject | undefined {
  const cutoff = flooredToSecond(after)
  return listConversations().find(
    (c) =>
      c.spec.coordinatorRef?.name === coordinatorName &&
      !c.spec.causedBy &&
      new Date(c.metadata.creationTimestamp).getTime() >= cutoff,
  )
}

export function findConversationByTitle(title: string): ConversationObject | undefined {
  return listConversations().find((c) => c.spec.title === title)
}

export function findMembersOf(parentName: string): ConversationObject[] {
  return listConversations().filter((c) => c.spec.causedBy?.parent === parentName)
}

export function runCount(conv: ConversationObject): number {
  return (conv.status?.runs ?? []).filter((r) => r.finishedAt).length
}

/** A background `kubectl port-forward`, resolved once the local port is
 * known and the target answers. The chosen port is random (`:targetPort`)
 * so this suite never fights another session's own forward on a fixed one. */
export interface Forward {
  localPort: number
  stop: () => void
}

export async function portForward(service: string, targetPort: number): Promise<Forward> {
  const child: ChildProcess = spawn(
    KUBECTL,
    [...kubectlBaseArgs(), 'port-forward', `svc/${service}`, `:${targetPort}`],
    { stdio: ['ignore', 'pipe', 'pipe'] },
  )
  const localPort = await new Promise<number>((resolve, reject) => {
    let buf = ''
    const onData = (chunk: Buffer) => {
      buf += chunk.toString()
      // "Forwarding from 127.0.0.1:XXXXX -> 8080"
      const m = /Forwarding from 127\.0\.0\.1:(\d+)/.exec(buf)
      if (m) {
        child.stdout?.off('data', onData)
        resolve(Number(m[1]))
      }
    }
    child.stdout?.on('data', onData)
    child.stderr?.on('data', (chunk: Buffer) => {
      buf += chunk.toString()
    })
    child.on('exit', (code) => reject(new Error(`port-forward to ${service} exited (${code}): ${buf}`)))
    setTimeout(() => reject(new Error(`port-forward to ${service} did not come up in time: ${buf}`)), 20_000)
  })
  return {
    localPort,
    stop: () => {
      child.kill()
    },
  }
}
