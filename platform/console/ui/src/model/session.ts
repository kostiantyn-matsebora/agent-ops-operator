import { api } from '../api/client'

// Signing in and out — the session's two writes, kept here so a look never
// reaches the API client for them (design D2, rule 2).

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
