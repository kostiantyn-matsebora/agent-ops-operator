import { describe, expect, it } from 'vitest'
import type { ConversationSummary } from '../api/types'
import { budgetLines, counted, escalation } from './coordinator'

describe('counted', () => {
  it('states the ceiling when there is one and the count alone when not', () => {
    expect(counted(2, 5)).toBe('2 of 5')
    expect(counted(undefined, 5)).toBe('0 of 5')
    expect(counted(3, undefined)).toBe('3')
  })
})

describe('budgetLines', () => {
  it('is empty without a budget', () => {
    expect(budgetLines(undefined)).toEqual([])
  })
  it('adds the deadline only when one is set', () => {
    expect(budgetLines({ agentsInvoked: 1, maxAgents: 4, turns: 2 }).map((l) => l.label)).toEqual(['Agents invoked', 'Turns'])
    expect(budgetLines({ deadline: '2025-06-10T22:00:00Z' }).map((l) => l.label)).toEqual(['Agents invoked', 'Turns', 'Deadline'])
  })
})

describe('escalation', () => {
  const base = { name: 'c' } as ConversationSummary
  it('is absent until the root escalated', () => {
    expect(escalation(base)).toBeUndefined()
  })
  it('carries the moment and a label', () => {
    const e = escalation({ ...base, escalatedAt: '2025-06-10T22:00:00Z' })
    expect(e?.at).toBe(Date.parse('2025-06-10T22:00:00Z'))
    expect(e?.label).toMatch(/^Escalated · /)
  })
})
