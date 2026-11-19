import { createContext } from 'react'
import { BUILD_DEFAULT_LOOK, LOOKS, type LookEntry } from '../registry'

/** The look in force and the registered looks, provided by the root. */
export interface LookContextValue {
  active: string
  looks: readonly LookEntry[]
}

export const LookContext = createContext<LookContextValue>({ active: BUILD_DEFAULT_LOOK, looks: LOOKS })
