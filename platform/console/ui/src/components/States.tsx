// THE SHARED EMPTY / ERROR / LOADING STATES, and they live here rather than in
// `App.tsx` because of a CIRCULAR IMPORT that only some Node versions survive.
//
// `App.tsx` imports every page, and the pages imported these three back out of
// it. Loading a page FIRST — which is what `await import('./Topology')` does in
// the tests, and what a lazy route would do in the browser — enters the cycle
// from the other side, so `App`'s exports are not initialised yet and `Loading`
// is `undefined`. React then throws "Element type is invalid", naming the page
// rather than the cycle.
//
// It was invisible for a while because Node 24 happens to resolve the cycle in
// the working order and Node 22 does not. The tests passed locally and failed in
// CI, which pins 22 — so CI was right and the local pass was the lie.
import type { ReactNode } from 'react'
import { Bullseye, EmptyState, EmptyStateBody, Spinner } from '@patternfly/react-core'

export function Loading() {
  return (
    <Bullseye>
      <Spinner aria-label="loading" />
    </Bullseye>
  )
}

export function ErrorState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <EmptyState titleText={title} headingLevel="h4" status="danger">
      <EmptyStateBody>{children}</EmptyStateBody>
    </EmptyState>
  )
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <EmptyState titleText={title} headingLevel="h4">
      <EmptyStateBody>{children}</EmptyStateBody>
    </EmptyState>
  )
}
