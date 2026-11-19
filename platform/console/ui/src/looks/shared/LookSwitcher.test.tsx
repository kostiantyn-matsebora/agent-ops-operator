import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { LookEntry } from '../registry'
import { LOOK_STORAGE_KEY, useLookStore } from '../../theme/useLook'
import { LookContext } from './LookContext'
import { LookSwitcher } from './LookSwitcher'

const looks: LookEntry[] = [
  { id: 'classic', label: 'Classic', load: () => Promise.reject(new Error('not loaded here')) },
  { id: 'workspace', label: 'Workspace', load: () => Promise.reject(new Error('not loaded here')) },
  { id: 'third', label: 'Third', load: () => Promise.reject(new Error('not loaded here')) },
]

const mount = (active: string) =>
  render(
    <LookContext.Provider value={{ active, looks }}>
      <LookSwitcher />
    </LookContext.Provider>,
  )

beforeEach(() => {
  localStorage.clear()
  useLookStore.setState({ choice: undefined, forced: undefined })
})

describe('the look switcher', () => {
  it('lists every registered look by its label', () => {
    mount('classic')
    const select = screen.getByRole('combobox', { name: 'Look' })
    expect(within(select).getAllByRole('option').map((o) => o.textContent)).toEqual(['Classic', 'Workspace', 'Third'])
  })

  it('marks the active look', () => {
    mount('workspace')
    expect(screen.getByRole('combobox', { name: 'Look' })).toHaveValue('workspace')
    expect(screen.getByRole('option', { name: 'Workspace' })).toHaveProperty('selected', true)
    expect(screen.getByTestId('look-switcher')).toHaveAttribute('data-active', 'workspace')
  })

  it('stores a choice, ending a URL override', async () => {
    mount('classic')
    useLookStore.getState().force('classic')
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Look' }), 'Workspace')
    expect(useLookStore.getState()).toMatchObject({ choice: 'workspace', forced: undefined })
    expect(JSON.parse(localStorage.getItem(LOOK_STORAGE_KEY)!).state.choice).toBe('workspace')
  })

  it('renders from the real registry with no provider', () => {
    render(<LookSwitcher />)
    expect(screen.getByRole('option', { name: 'Classic' })).toHaveProperty('selected', true)
  })
})
