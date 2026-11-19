import type { ConversationBudget, ConversationSummary } from '../api/types'

// A COORDINATOR ROOT'S OWN FACTS: its budget, per level, and the moment it
// reached a person. Every look shows them; none recomputes them.

/** `n of max`, or just `n` when the ceiling is unset. */
export function counted(used: number | undefined, max: number | undefined): string {
  const count = used ?? 0
  return max ? `${count} of ${max}` : String(count)
}

export interface BudgetLine {
  label: string
  value: string
}

/** The budget as labelled lines, or none when the conversation carries no budget. */
export function budgetLines(budget: ConversationBudget | undefined): BudgetLine[] {
  if (!budget) return []
  const lines: BudgetLine[] = [
    { label: 'Agents invoked', value: counted(budget.agentsInvoked, budget.maxAgents) },
    { label: 'Turns', value: counted(budget.turns, budget.maxTurns) },
  ]
  if (budget.deadline) lines.push({ label: 'Deadline', value: new Date(budget.deadline).toLocaleString() })
  return lines
}

/** When the root escalated to a person, as a timestamp the thread places by time; undefined if it never did. */
export function escalation(c: ConversationSummary): { at: number; label: string } | undefined {
  if (!c.escalatedAt) return undefined
  return { at: Date.parse(c.escalatedAt) || 0, label: `Escalated · ${new Date(c.escalatedAt).toLocaleString()}` }
}
