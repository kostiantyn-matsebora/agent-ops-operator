import { api } from '../api/client'
import { useChannelFeed, useLiveStream as openLiveStream, useSession } from '../api/hooks'

// The session and the console-wide facts a look's shell shows: who the viewer
// is and whether the stream is live, and the channel feed. Kept here so a
// look never reaches the API client or its hooks for them (design D2, rule 2).

/** Presents a console token. Resolves true when it was accepted. */
export async function signIn(token: string): Promise<boolean> {
  try {
    await api.login(token)
    return true
  } catch {
    // Deliberately no reason: an unconfigured console and a wrong token answer
    // alike, and the form says the same thing for both.
    return false
  }
}

/** Ends the session's cookie. */
export function signOut(): Promise<void> {
  return api.logout()
}

/** The console session: authentication, identity and what the viewer may do. */
export function useConsoleSession() {
  return useSession()
}

/**
 * Opens the one live stream for the app's lifetime and reports whether it is
 * connected. A look's shell calls this once.
 */
export function useLiveStream(): boolean {
  return openLiveStream()
}

/** The channel's general feed: the thread-less sends no conversation holds. */
export function useGeneralFeed() {
  return useChannelFeed()
}

/**
 * The route name the general feed is opened under. Never a real
 * conversation's: a Kubernetes object name carries no `:`.
 */
export const GENERAL_FEED_NAME = 'channel:general'
