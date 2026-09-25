import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { BUILTIN_ICONS } from '../icons/builtin'
import { PipelineName } from './PipelineName'

// Every mention of a Pipeline draws the icon it declares, whether the caller
// knew it or the vocabulary did.

vi.mock('../api/hooks', () => ({
  usePipelineIcon: () => (name?: string) => (name === 'k8s-ops' ? 'aops:kubernetes' : undefined),
}))

describe('a pipeline\'s name', () => {
  it('carries the icon the caller hands it', () => {
    render(<PipelineName name="anything" icon="aops:alert" />)
    expect(screen.getByText('anything')).toBeInTheDocument()
    expect(document.querySelector('svg path')?.getAttribute('d')).toBe(BUILTIN_ICONS.alert)
  })

  it('looks the icon up by name otherwise, and draws only the name for a pipeline without one', () => {
    const { unmount } = render(<PipelineName name="k8s-ops" />)
    expect(document.querySelector('svg path')?.getAttribute('d')).toBe(BUILTIN_ICONS.kubernetes)
    unmount()
    render(<PipelineName name="plain" />)
    expect(screen.getByText('plain')).toBeInTheDocument()
    expect(document.querySelector('svg')).toBeNull()
  })
})
