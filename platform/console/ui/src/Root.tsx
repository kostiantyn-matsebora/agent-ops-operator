import { type ComponentType, type LazyExoticComponent, Suspense, lazy, useEffect, useLayoutEffect, useMemo } from 'react'
import { useSession } from './api/hooks'
import { BUILD_DEFAULT_LOOK, LOOKS, type LookEntry } from './looks/registry'
import { LookContext } from './looks/shared/LookContext'
import { applyLook, resolveLook, useLookStore } from './theme/useLook'

// The root: resolves the look in force and mounts it, loaded lazily so a
// viewer downloads only that look.

const shells = new WeakMap<LookEntry, LazyExoticComponent<ComponentType>>()

/** One lazy component per entry, so a re-render never re-imports a look. */
function shellOf(entry: LookEntry): LazyExoticComponent<ComponentType> {
  let shell = shells.get(entry)
  if (!shell) {
    shell = lazy(() => entry.load().then((m) => ({ default: m.Shell })))
    shells.set(entry, shell)
  }
  return shell
}

export function Root({
  looks = LOOKS,
  buildDefault = BUILD_DEFAULT_LOOK,
}: Readonly<{ looks?: readonly LookEntry[]; buildDefault?: string }>) {
  const choice = useLookStore((s) => s.choice)
  const forced = useLookStore((s) => s.forced)
  const session = useSession()

  const registered = looks.map((l) => l.id)
  const resolution = resolveLook({
    query: forced,
    stored: choice,
    installDefault: session.data?.defaultLook,
    registered,
    buildDefault,
  })
  // With neither the URL nor the viewer naming a look, the install's default
  // decides, and it arrives with the session. Deciding before it would load
  // the build default's look only to swap it out.
  const waiting =
    session.isLoading && !registered.includes(forced ?? '') && !registered.includes(choice ?? '')

  useEffect(() => {
    if (resolution.staleStored) useLookStore.getState().forget()
  }, [resolution.staleStored])

  useLayoutEffect(() => {
    if (!waiting) applyLook(resolution.look)
  }, [waiting, resolution.look])

  const context = useMemo(() => ({ active: resolution.look, looks }), [resolution.look, looks])

  if (waiting) return null
  const entry = looks.find((l) => l.id === resolution.look)
  if (!entry) return null
  const Shell = shellOf(entry)

  return (
    <LookContext.Provider value={context}>
      <Suspense fallback={null}>
        <Shell />
      </Suspense>
    </LookContext.Provider>
  )
}
