import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HistoryCharts } from './History'

// The history card folds to its title and stays on the page, so the canvas
// above it can take the viewport and the charts are one chevron away.

describe('the history card', () => {
  it('folds to its title from its chevron', async () => {
    const onToggle = vi.fn()
    const { rerender } = render(<HistoryCharts available={false} expanded onToggle={onToggle} />)
    expect(screen.getByText('No metrics backend is configured')).toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Toggle history'))
    expect(onToggle).toHaveBeenCalledTimes(1)
    rerender(<HistoryCharts available={false} expanded={false} onToggle={onToggle} />)
    expect(screen.queryByText('No metrics backend is configured')).not.toBeInTheDocument()
    expect(screen.getByText('History')).toBeInTheDocument()
  })
})
