import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ProblemsCard } from './Overview'
import type { Problem } from '../api/types'

// The Problems table drops Problem.since (the condition's lastTransitionTime)
// even though the backend already computes and sorts by it — these pin the
// Since column showing a relative age where the row carries one, and an em
// dash where it does not (pod- and console-derived rows have no condition to
// date).

function problem(over: Partial<Problem> = {}): Problem {
  return { kind: 'pipelines', name: 'nightly', type: 'Ready', source: 'reported', ...over }
}

describe('ProblemsCard', () => {
  it('shows a relative age for a row with since set, and an em dash for one without', () => {
    const since = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString()
    render(
      <MemoryRouter>
        <ProblemsCard
          problems={[
            problem({ name: 'reported-row', since }),
            problem({ name: 'pod-row', source: 'pod', kind: 'pods' }),
          ]}
        />
      </MemoryRouter>,
    )
    const rows = screen.getAllByRole('row')
    // row[0] is the header
    expect(within(rows[1]).getByText(/ago/)).toBeInTheDocument()
    expect(within(rows[2]).getByText('—')).toBeInTheDocument()
  })

  it('still reports nothing to show when there are no problems', () => {
    render(
      <MemoryRouter>
        <ProblemsCard problems={[]} />
      </MemoryRouter>,
    )
    expect(screen.getByText('Nothing is reporting a problem')).toBeInTheDocument()
  })
})
