import type { Page } from '@playwright/test'

// The start verb (POST /api/conversations) for a spec: the shared mock answers
// every /api request as a read, so a spec that starts a conversation answers
// the POST itself, and records what was sent.

export interface StartMock {
  bodies: { task: string; source?: string }[]
}

export async function mockStart(page: Page, opts: { fail?: string } = {}): Promise<StartMock> {
  const seen: StartMock = { bodies: [] }
  await page.route('**/api/conversations', async (route) => {
    const req = route.request()
    if (req.method() !== 'POST') return route.fallback()
    seen.bodies.push(req.postDataJSON())
    if (opts.fail) return route.fulfill({ status: 422, contentType: 'application/json', body: JSON.stringify({ error: opts.fail }) })
    return route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ source: 'console', note: 'accepted' }) })
  })
  return seen
}
