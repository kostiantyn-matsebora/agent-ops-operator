import { type FormEvent, type ReactElement, useId, useState } from 'react'
import { signIn } from '../../../model/session'
import { Logo } from '../../../components/Logo'

// Signing in. The token IS the boundary, and an unconfigured console answers
// a guess exactly as it answers a wrong token — so the form says the same
// thing for both, and the notice is about the install, not the guess.

export function LoginPage({ configured, onLogin }: Readonly<{ configured: boolean; onLogin: () => void }>): ReactElement {
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const id = useId()

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    const accepted = await signIn(token)
    setBusy(false)
    if (accepted) onLogin()
    else setError('That token was not accepted.')
  }

  return (
    <main className="wk-pane wk-page" aria-label="Sign in">
      <form className="wk-login" onSubmit={submit}>
        <h1 className="wk-brand">
          <Logo size={28} /> agent-ops console
        </h1>
        {!configured && (
          <div className="wk-alert" role="note">
            Nobody can sign in until the console Channel supplies a <code>uiToken</code> credential, or{' '}
            <code>UI_TOKEN</code> is set.
          </div>
        )}
        <label htmlFor={id}>Token</label>
        <input
          id={id}
          type="password"
          autoComplete="current-password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
        {error && (
          <div className="wk-alert bad" role="alert">
            {error}
          </div>
        )}
        <button type="submit" className="wk-btn primary" disabled={busy || !token}>
          Sign in
        </button>
      </form>
    </main>
  )
}
