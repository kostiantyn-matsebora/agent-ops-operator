import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BUILD_DEFAULT_LOOK, type LookEntry, type LookModule } from './looks/registry'
import { LookSwitcher } from './looks/shared/LookSwitcher'
import { Root } from './Root'
import { LOOK_STORAGE_KEY, useLookStore } from './theme/useLook'

// The root resolves the look and loads ONLY that look's module. Two test looks
// with spy loaders stand in for the registry, so the assertion is on which
// loader ran, not on bundle names.

const session = vi.hoisted(() => ({ current: { isLoading: false, data: {} as { defaultLook?: string } | undefined } }))
vi.mock('./api/hooks', () => ({ useSession: () => session.current }))

const noPage = () => null
const moduleFor = (id: string): LookModule => ({
  Shell: () => (
    <div>
      <p>{id} look</p>
      <LookSwitcher />
    </div>
  ),
  pages: {
    Overview: noPage, Queues: noPage, Config: noPage, ConfigKind: noPage, ConfigDetail: noPage,
    Topology: noPage, Conversations: noPage, Login: noPage,
  },
})

let looks: LookEntry[]
beforeEach(() => {
  localStorage.clear()
  useLookStore.setState({ choice: undefined, forced: undefined })
  document.documentElement.removeAttribute('data-look')
  session.current = { isLoading: false, data: {} }
  looks = ['alpha', 'beta'].map((id) => ({
    id,
    label: id.toUpperCase(),
    load: vi.fn(() => Promise.resolve(moduleFor(id))),
  }))
})

const [alpha, beta] = [0, 1]
const mount = () => render(<Root looks={looks} buildDefault="alpha" />)

describe('only the active look loads', () => {
  it('loads the build default alone when nothing names a look', async () => {
    mount()
    expect(await screen.findByText('alpha look')).toBeInTheDocument()
    expect(looks[alpha].load).toHaveBeenCalledTimes(1)
    expect(looks[beta].load).not.toHaveBeenCalled()
    expect(document.documentElement.getAttribute('data-look')).toBe('alpha')
  })

  it('loads the stored look alone', async () => {
    useLookStore.getState().choose('beta')
    mount()
    expect(await screen.findByText('beta look')).toBeInTheDocument()
    expect(looks[alpha].load).not.toHaveBeenCalled()
    expect(document.documentElement.getAttribute('data-look')).toBe('beta')
  })
})

describe('the resolution, through the root', () => {
  it('opens in the install default when the viewer chose nothing', async () => {
    session.current = { isLoading: false, data: { defaultLook: 'beta' } }
    mount()
    expect(await screen.findByText('beta look')).toBeInTheDocument()
    expect(looks[alpha].load).not.toHaveBeenCalled()
  })

  it('lets the viewer’s choice win over the install', async () => {
    session.current = { isLoading: false, data: { defaultLook: 'beta' } }
    useLookStore.getState().choose('alpha')
    mount()
    expect(await screen.findByText('alpha look')).toBeInTheDocument()
  })

  it('pins a look from the URL without overwriting the stored choice', async () => {
    useLookStore.getState().choose('beta')
    useLookStore.getState().force('alpha')
    mount()
    expect(await screen.findByText('alpha look')).toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem(LOOK_STORAGE_KEY)!).state.choice).toBe('beta')
  })

  it('waits for the session before letting the install decide, loading nothing meanwhile', () => {
    session.current = { isLoading: true, data: undefined }
    const { container } = mount()
    expect(container).toBeEmptyDOMElement()
    expect(looks[alpha].load).not.toHaveBeenCalled()
    expect(looks[beta].load).not.toHaveBeenCalled()
  })

  it('does not wait for the session when the viewer already chose', async () => {
    session.current = { isLoading: true, data: undefined }
    useLookStore.getState().choose('beta')
    mount()
    expect(await screen.findByText('beta look')).toBeInTheDocument()
  })

  it('replaces a stored look this build no longer registers', async () => {
    useLookStore.getState().choose('retired')
    mount()
    expect(await screen.findByText('alpha look')).toBeInTheDocument()
    expect(useLookStore.getState().choice).toBeUndefined()
    expect(JSON.parse(localStorage.getItem(LOOK_STORAGE_KEY)!).state.choice).toBeUndefined()
  })
})

describe('a first visit', () => {
  // No query, no stored choice, no install default, and Root's own default
  // parameter left untouched — task 7.4's claim that this resolves to
  // workspace, the real BUILD_DEFAULT_LOOK, not a test override.
  it('opens in the build default look, workspace', async () => {
    const firstVisitLooks: LookEntry[] = ['classic', 'workspace'].map((id) => ({
      id, label: id, load: vi.fn(() => Promise.resolve(moduleFor(id))),
    }))
    session.current = { isLoading: false, data: {} }
    render(<Root looks={firstVisitLooks} />)
    expect(await screen.findByText('workspace look')).toBeInTheDocument()
    expect(document.documentElement.getAttribute('data-look')).toBe('workspace')
    expect(BUILD_DEFAULT_LOOK).toBe('workspace')
  })
})

describe('switching', () => {
  it('applies the chosen look without a reload, and persists it', async () => {
    mount()
    await screen.findByText('alpha look')
    await userEvent.selectOptions(screen.getByLabelText('Look'), 'beta')
    expect(await screen.findByText('beta look')).toBeInTheDocument()
    expect(document.documentElement.getAttribute('data-look')).toBe('beta')
    expect(JSON.parse(localStorage.getItem(LOOK_STORAGE_KEY)!).state.choice).toBe('beta')
    expect(looks[beta].load).toHaveBeenCalledTimes(1)
  })
})
