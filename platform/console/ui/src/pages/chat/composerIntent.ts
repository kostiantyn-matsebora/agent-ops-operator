import { create } from 'zustand'

/**
 * The bridge from a start chip — mounted wherever the open thread is — to the
 * masthead's ONE `NewConversation` instance.
 *
 * State stays local until sharing is a decision (this file's whole reason to
 * exist): a quick-chip click two components away from the composer it opens
 * is exactly the case design D-H asks for, and duplicating the composer's
 * modal just to reach it from here would be the second implementation this
 * app avoids everywhere else.
 */
interface ComposerIntentState {
  requestedTask: string | null
  openWith: (task: string) => void
  clear: () => void
}

export const useComposerIntent = create<ComposerIntentState>((set) => ({
  requestedTask: null,
  openWith: (task) => set({ requestedTask: task }),
  clear: () => set({ requestedTask: null }),
}))
