import { useState } from 'react'
import {
  ActionGroup, Alert, Bullseye, Button, Card, CardBody, CardTitle,
  Form, FormGroup, TextInput,
} from '@patternfly/react-core'
import { api } from '../api/client'

/**
 * The token IS the boundary: whoever holds it can read every CR the console's
 * ServiceAccount can read and instruct any joined agent. An UNCONFIGURED console
 * authorizes nobody and answers identically to a wrong token — so this form
 * shows the same message either way, and the notice below is about the install,
 * not about whether the guess was close.
 */
export function LoginPage({ configured, onLogin }: { configured: boolean; onLogin: () => void }) {
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.login(token)
      onLogin()
    } catch {
      setError('That token was not accepted.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Bullseye>
      <Card style={{ minWidth: 420 }}>
        <CardTitle>agent-ops console</CardTitle>
        <CardBody>
          {!configured && (
            <Alert variant="warning" isInline title="This console has no token configured">
              Nobody can sign in until the console Channel supplies a <code>uiToken</code>{' '}
              credential, or <code>UI_TOKEN</code> is set.
            </Alert>
          )}
          <Form onSubmit={submit}>
            <FormGroup label="Token" fieldId="token">
              <TextInput
                id="token"
                type="password"
                value={token}
                onChange={(_e, v) => setToken(v)}
                aria-label="console token"
              />
            </FormGroup>
            {error && <Alert variant="danger" isInline title={error} />}
            <ActionGroup>
              <Button type="submit" isDisabled={busy || !token}>
                Sign in
              </Button>
            </ActionGroup>
          </Form>
        </CardBody>
      </Card>
    </Bullseye>
  )
}
