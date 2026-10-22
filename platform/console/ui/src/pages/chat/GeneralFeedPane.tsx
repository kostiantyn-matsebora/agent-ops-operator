import { Alert, Button, Label } from '@patternfly/react-core'
import { HashtagIcon } from '@patternfly/react-icons'
import { useChannelFeed } from '../../api/hooks'
import { ErrorState, Loading } from '../../components/States'
import { PlainText } from '../../components/Text'
import { Timeline } from './Timeline'

/**
 * A sentinel `name` for the thread list's route param — never a real
 * conversation's, since a Kubernetes object name is lowercase
 * alphanumerics and `-` only (RFC 1123), and this carries a `:`. Picking it
 * selects this pane instead of `ThreadPane` (ChatView.tsx).
 */
export const GENERAL_FEED_NAME = 'channel:general'

/**
 * The channel's general feed (B1, UI half) — every thread-less send the
 * adapter parks on its `channel:<name>` pseudo-thread, surfaced so it is
 * VISIBLE rather than silently dropped because it has no conversation to
 * attach to (design: "mirroring Telegram's general topic").
 *
 * Reuses the SAME `Timeline` a conversation's own transcript renders with
 * (same message bubbles, same choices-degrade rule, B2) — never a bespoke
 * renderer. Read-only: a thread-less send has no conversation behind it to
 * reply into, so there is no composer here, the same shape `MemberOwnBody`
 * already uses for its own read-only transcript (ThreadPane.tsx).
 */
export function GeneralFeedPane({ onBack }: Readonly<{ onBack?: () => void }>) {
  const { data, isLoading, error } = useChannelFeed()

  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not load the channel feed">{String(error)}</ErrorState>

  return (
    <div style={{ flex: 1, minWidth: 0, background: 'var(--ao-surface)', display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '12px 24px 10px', borderBottom: '1px solid var(--ao-border)', display: 'flex', flexDirection: 'column', gap: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          {onBack && (
            <Button variant="plain" aria-label="back to the list" onClick={onBack}>
              ←
            </Button>
          )}
          <span style={{ fontSize: '1.1em', fontWeight: 700, flex: 1, minWidth: 0 }}>
            <HashtagIcon /> General feed
          </span>
          {data.channel && (
            <Label color="grey">
              <PlainText>{data.channel}</PlainText>
            </Label>
          )}
        </div>
      </div>
      <div style={{ padding: '8px 24px 0' }}>
        <Alert variant="info" isInline title="Channel-wide notices, not a conversation">
          A send with no conversation behind it — like an <code>/agents</code> reply — lands here instead of
          being dropped. Read-only: there is nothing to reply into.
        </Alert>
      </div>
      <Timeline messages={data.messages} events={[]} presence={false} />
    </div>
  )
}
