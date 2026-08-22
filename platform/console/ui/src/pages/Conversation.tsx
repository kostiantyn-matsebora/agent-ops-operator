import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import {
  Alert, Button, Card, CardBody, CardTitle, ClipboardCopy,
  DescriptionList, DescriptionListDescription, DescriptionListGroup, DescriptionListTerm,
  Label, LabelGroup, Menu, MenuContent, MenuItem, MenuList,
  PageSection, Stack, StackItem, Tab, TabTitleText, Tabs, TextArea,
  Title, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { useParams } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../App'
import { useConversation, useConversationGraph, useMarkRead, useSession, useVocabulary } from '../api/hooks'
import { useStream } from '../api/stream'
import { PlainText } from '../components/Text'
import { Markdown } from '../components/Markdown'
import { Graph } from '../graph/Graph'
import { api, ApiError } from '../api/client'
import { Crumbs } from '../components/Crumbs'
import { ComposerHint } from '../components/ComposerHint'
import { Icon, stripLeadingIcon } from '../components/Icon'
import { matchEntries } from './NewConversation'
import type { VocabularyEntry } from '../api/types'
import { Yaml } from '../components/Yaml'
import { MetadataCard, age } from '../components/Metadata'
import type { ActivityEvent } from '../api/types'

// speaker names who said something, for a message carrying no sender. The
// transcript kinds are plumbing vocabulary: `local` means "typed on this
// console", which is a fact about where a message entered, not a person.
const SPEAKERS: Record<string, string> = {
  local: 'user',
  relay: 'user',
  agent: 'agent',
  ack: 'agent-ops',
  signal: 'signal',
}

function speaker(kind: string): string {
  return SPEAKERS[kind] ?? kind
}

/**
 * WHO SPOKE, AS A BADGE.
 *
 * Bold alone stopped working the moment the body could be bold too — an actor
 * merged into the markdown under it. So the attribution gets a shape of its
 * own: a glyph in a tinted disc, and the name in that speaker's colour.
 *
 * Colour carries meaning here, so it is never the ONLY signal — the glyph
 * differs per kind and the name is still written out.
 */
// Keyed on the kinds the BFF actually sends — `local`, `relay`, `agent`,
// `ack`, `signal`. A kind that is not here still renders, with the neutral
// glyph: an unknown speaker is a message to show, not a message to drop.
const SPEAKER_STYLE: Record<string, { icon: string; tint: string }> = {
  // A PERSON. Given the brand colour, because "did I say this or did it?" is
  // the question a transcript is scanned for.
  local: { icon: 'aops:user', tint: 'var(--ao-brand-strong)' },
  relay: { icon: 'aops:user', tint: 'var(--ao-brand-strong)' },
  agent: { icon: 'aops:agent', tint: 'var(--ao-text)' },
  ack: { icon: 'aops:system', tint: 'var(--ao-text-subtle)' },
  signal: { icon: 'aops:alert', tint: 'var(--ao-warning)' },
}

function speakerStyle(kind: string) {
  return SPEAKER_STYLE[kind] ?? { icon: 'aops:system', tint: 'var(--ao-text)' }
}

/** The avatar that sits in the gutter. */
function Avatar({ kind, icon }: { kind: string; icon?: string }) {
  const style = speakerStyle(kind)
  return (
    <span
      aria-hidden
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '2em',
        height: '2em',
        borderRadius: '50%',
        color: style.tint,
        background: 'var(--ao-surface-alt)',
        border: `1px solid ${style.tint}`,
      }}
    >
      {/* The AGENT wears its route's icon when the route declares one — the
          same glyph the composer completes — and falls back to the generic. */}
      <Icon icon={kind === 'agent' ? icon || style.icon : style.icon} size="1.1em" />
    </span>
  )
}

export function ConversationPage() {
  const { name = '' } = useParams()
  const { data, isLoading, error, refetch } = useConversation(name)
  const [tab, setTab] = useState<string | number>(0)

  // Opening a conversation reports its CONSOLE thread read, and reports again
  // as activity arrives while the view stays open.
  //
  // The watermark is never generated here — the server reads it off the
  // conversation's own state, and `unread` is what says the report would
  // advance anything at all, so a re-opened, already-read conversation sends
  // nothing. Observed conversations are skipped: no console thread, no
  // watermark to move.
  const markRead = useMarkRead()
  const summary = data?.conversation
  const reported = useRef('')
  const activity = summary?.lastActivity ?? summary?.created ?? ''
  const joinedUnread = Boolean(summary?.joined && summary?.unread)
  const pageVocabulary = useVocabulary()


  useEffect(() => {
    if (!summary || !joinedUnread) return
    const stamp = `${summary.name}:${activity}`
    if (reported.current === stamp) return
    reported.current = stamp
    markRead.mutate({ names: [summary.name] })
    // Deliberately keyed on the conversation and its activity, not on the
    // mutation handle: re-running on the handle would report on every render.
  }, [summary?.name, joinedUnread, activity])

  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Conversation not found">{String(error)}</ErrorState>

  const c = data.conversation
  // The route's declared icon, by name.
  const pipelineIcon = (pageVocabulary.data?.entries ?? []).find(
    (e) => e.kind === 'pipeline' && e.name === c.pipeline,
  )?.icon
  return (
    <>
      <Crumbs
        items={[
          { label: 'Conversations', to: '/conversations' },
          { label: stripLeadingIcon(c.title || c.name) },
        ]}
      />
      {/* PATTERNFLY'S OWN ANSWER, not a hand-rolled one.
          `isFilled` gives this section the space the page has left; the flex
          chain below then only has to keep `min-height: 0` at every level, which
          is the documented requirement for a scroll container inside flex.
          Two earlier attempts guessed a height and then fought PatternFly's own
          block wrappers — both were me not reading what the component offers. */}
      <PageSection isFilled hasOverflowScroll aria-label="conversation">
      <Stack hasGutter>
        <StackItem>
          {/* The ROUTE's icon, drawn from what the Pipeline declares. The lane
              emoji the manager wrote into the title is stripped so the two do
              not stack. */}
          <Title headingLevel="h1">
            <Icon icon={pipelineIcon} />{' '}
            <PlainText>{stripLeadingIcon(c.title || c.name)}</PlainText>
          </Title>
          {/* The whole identity of the run, as chips: phase, attribution,
              profile, the runtime pod, and the capabilities it MATERIALIZED. */}
          <LabelGroup numLabels={10}>
            <Label isCompact color="blue">{c.phase}</Label>
            {c.pipeline ? (
              <Label isCompact color="blue" icon={<Icon icon={pipelineIcon} />}>
                pipeline {c.pipeline}
              </Label>
            ) : (
              <Tooltip content="a Conversation records no pipelineRef; attribution is inferred from its bindings and left blank when ambiguous">
                <Label isCompact color="grey">unattributed</Label>
              </Tooltip>
            )}
            {c.profile && <Label isCompact color="purple">profile {c.profile}</Label>}
            {c.runtimePod && <Label isCompact color="grey">pod {c.runtimePod}</Label>}
            {c.errored && <Label isCompact status="danger">last run failed</Label>}
            <Label isCompact color="grey">{c.runCount} run(s)</Label>
            <Label isCompact color="grey">age {age(c.created)}</Label>
            {(c.toolsets ?? []).map((t) => (
              <Label key={t} isCompact color="orange">{t}</Label>
            ))}
            {(c.mcpConfigs ?? []).map((m) => (
              <Label key={m} isCompact color="teal">{m}</Label>
            ))}
          </LabelGroup>
        </StackItem>
        <StackItem>
          <Tabs activeKey={tab} onSelect={(_e, k) => setTab(k)}>
            <Tab eventKey={0} title={<TabTitleText>Transcript</TabTitleText>}>
              {/* `active` is passed so the transcript can re-pin itself to the
                  newest message when this tab is shown again. Its message list
                  is unchanged by a tab switch, so nothing else would tell it
                  to. */}
              {/* Nothing to re-read after a send WHILE THE STREAM IS UP: the
                  manager delivers the message back to this channel, so the
                  bubble arrives like any other event. The fallback is for when
                  it cannot — see the composer. */}
              <Transcript detail={data} onSentOffline={() => refetch()} active={tab === 0} />
            </Tab>
            <Tab eventKey={1} title={<TabTitleText>Runs</TabTitleText>}>
              <RunTimeline detail={data} />
            </Tab>
            <Tab eventKey={2} title={<TabTitleText>Graph</TabTitleText>}>
              <ConversationGraphTab name={name} />
            </Tab>
            <Tab eventKey={3} title={<TabTitleText>Sequence</TabTitleText>}>
              <Sequence events={data.events ?? []} />
            </Tab>
            <Tab eventKey={4} title={<TabTitleText>YAML</TabTitleText>}>
              <Stack hasGutter>
                <StackItem>
                  <MetadataCard meta={data.object.metadata} />
                </StackItem>
                <StackItem>
                  <Yaml value={data.yaml} title={`conversation ${c.name} YAML`} />
                </StackItem>
              </Stack>
            </Tab>
          </Tabs>
        </StackItem>
      </Stack>
      </PageSection>
    </>
  )
}

function Transcript({
  detail,
  onSentOffline,
  active,
}: {
  detail: NonNullable<ReturnType<typeof useConversation>['data']>
  onSentOffline: () => void
  active: boolean
}) {
  const session = useSession()
  const connected = useStream((s) => s.connected)
  const [text, setText] = useState('')
  // The composer attached to a conversation offers what ACTS on one: releasing
  // its runtime and ending it. It never offers a Pipeline — inside a thread that
  // text is input for the agent, not a command.
  //
  // The pair is presented TOGETHER by construction, because both carry
  // position `thread` and the filter takes the whole position. They are one
  // word apart and only one of them ends the conversation, so showing either
  // alone is what this avoids.
  const vocabulary = useVocabulary()
  const [dismissed, setDismissed] = useState(false)
  const [cursor, setCursor] = useState(0)
  const textRef = useRef<HTMLTextAreaElement>(null)
  const commands = dismissed
    ? null
    : matchEntries(text, vocabulary.data?.entries ?? [], 'thread')
  const activeCommand = commands ? Math.min(cursor, commands.length - 1) : 0

  function chooseCommand(entry: VocabularyEntry) {
    // These commands take no argument, so the whole message IS the command.
    const next = '/' + entry.name
    setText(next)
    setDismissed(true)
    setCursor(0)
    requestAnimationFrame(() => {
      textRef.current?.focus()
      textRef.current?.setSelectionRange(next.length, next.length)
    })
  }

  function onComposerKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // SHIFT+ENTER SENDS — see NewConversation for why it wins over the menu.
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault()
      if (!busy && text.trim()) void send()
      return
    }
    if (!commands) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setCursor((c) => (c + 1) % commands.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setCursor((c) => (c - 1 + commands.length) % commands.length)
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      chooseCommand(commands[activeCommand])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setDismissed(true)
    }
  }
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const messages = detail.transcript ?? []
  // Stick to the NEWEST message — on open, and on every arrival after it.
  //
  // Both cases matter and they are the same effect: a thread that opens at its
  // oldest line is the wrong end of the only view whose purpose is answering
  // what was just said, and one that does not follow an incoming answer makes
  // the reader hunt for the thing they were waiting for.
  //
  // useLayoutEffect, not useEffect: it runs before paint, so the thread appears
  // already scrolled instead of visibly jumping. Keyed on the last message's id
  // as well as the count, because a replaced final message (a pending local
  // reply becoming confirmed) is new content to look at even when the count is
  // unchanged.
  const listRef = useRef<HTMLDivElement>(null)
  const lastID = messages.length ? messages[messages.length - 1].id : ''
  useLayoutEffect(() => {
    if (!active) return
    const el = listRef.current
    if (!el) return
    const pin = () => {
      el.scrollTop = el.scrollHeight
    }
    pin()
    // A second pass after paint. Coming back to this tab, the container can
    // still be laying out when the effect runs — a hidden element has no
    // height, so scrollHeight is whatever it was a moment ago and pinning
    // reads as "did nothing". One frame later the measurement is real.
    const raf = requestAnimationFrame(pin)
    return () => cancelAnimationFrame(raf)
  }, [messages.length, lastID, active])
  // canWrite from the session, not writeEnabled: a console whose fronting proxy
  // forwards no identity has writes ON and nothing to attribute them to, and
  // the composer must say so rather than accept text the server will refuse.
  const canWrite = (session.data?.canWrite ?? false) && detail.conversation.joined && !detail.archived
  // The agent's avatar wears its ROUTE's icon, so the face in the thread and
  // the glyph in the composer are the same thing.
  const vocab = useVocabulary()
  const pipelineIcon = (vocab.data?.entries ?? []).find(
    (e) => e.kind === 'pipeline' && e.name === detail.conversation.pipeline,
  )?.icon
  const noIdentity =
    (session.data?.writeEnabled ?? false) && !(session.data?.canWrite ?? false)

  async function send() {
    setBusy(true)
    setError(undefined)
    try {
      await api.send(detail.conversation.name, text)
      setText('')
      // A sent message shows as `sending…` until the manager's confirmation
      // comes back — and that confirmation is a STREAM event. With the stream
      // down there is nothing to deliver it, so the bubble would sit unconfirmed
      // until somebody reloaded the page: the same "only true after F5" failure
      // the reconnect logic exists to prevent, wearing different clothes.
      //
      // So the read is not removed, it is CONDITIONED: it happens exactly when
      // the thing that replaced it cannot run.
      if (!connected) onSentOffline()
    } catch (e) {
      setError((e as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Stack hasGutter>
      {detail.joinHint && (
        <StackItem>
          {/* No composer, and the reason plus the exact patch. The console never
              edits a Pipeline — showing the edit IS the answer. */}
          <Alert variant="info" isInline title="This conversation has no console thread">
            <p>
              <PlainText>{detail.joinHint.reason}</PlainText>
            </p>
            <ClipboardCopy isReadOnly>{detail.joinHint.fix}</ClipboardCopy>
            {detail.joinHint.note && (
              <small>
                <PlainText>{detail.joinHint.note}</PlainText>
              </small>
            )}
          </Alert>
        </StackItem>
      )}
      {detail.archived && (
        <StackItem>
          <Alert variant="info" isInline title="This thread was archived">
            The conversation was closed. The transcript stays readable; there is nothing left to
            reply to.
          </Alert>
        </StackItem>
      )}
      {/* The message list SCROLLS and the composer does not.
          A long thread used to push the reply box off the bottom of the page,
          so answering meant scrolling to the end first — on the one view whose
          entire purpose is answering. The list gets the overflow; the composer
          below it stays put. */}
      <StackItem>
        <Card>
          <CardBody>
            {/* A PLAIN div holds the ref. PatternFly's CardBody does not
                forward one to its DOM node, so scrolling it never worked —
                listRef.current was null and the thread opened at its oldest
                line every time. */}
            <div
              ref={listRef}
              style={{
                minWidth: 0,
                // ONE SCROLLER, and it is the SECTION above. A second one here
                // is what put two scrollbars on the page, and made "which one
                // am I in" a question a reader had to answer.
                //
                // Sideways is still forbidden: anything too wide — a table, a
                // code block — scrolls inside its own box, never the page.
                overflowX: 'hidden',
              }}
            >
            {messages.length === 0 ? (
              <Empty title="No messages on the console thread yet" />
            ) : (
              messages.map((m, i) => {
                /* GROUPED, THE WAY EVERY MESSENGER DOES IT.
                   A run of messages from one speaker is ONE block: the avatar
                   and the name appear when the speaker changes and not again,
                   so a thread reads as a conversation instead of a log with the
                   same name stamped on every line.
                   Sixty seconds is the usual window — long enough to group a
                   burst, short enough that a later reply still says who. */
                const prev = messages[i - 1]
                const sameSpeaker =
                  prev && prev.kind === m.kind && (prev.sender ?? '') === (m.sender ?? '')
                const within = prev && Date.parse(m.at) - Date.parse(prev.at) < 60_000
                const startsGroup = !sameSpeaker || !within

                return (
                <article
                  key={m.id}
                  style={{
                    display: 'grid',
                    // The gutter holds the avatar; everything else lines up in
                    // one column beneath the name, grouped or not.
                    gridTemplateColumns: '2em 1fr',
                    columnGap: '0.75em',
                    padding: startsGroup ? '0.85em 0 0.15em' : '0.15em 0',
                    // The rule separates GROUPS, not every line — inside a run
                    // it would cut a single speaker's turn into slices.
                    borderTop: startsGroup && i > 0 ? '1px solid var(--ao-border)' : undefined,
                  }}
                >
                  <div style={{ gridColumn: 1 }}>
                    {startsGroup && <Avatar kind={m.kind} icon={pipelineIcon} />}
                  </div>
                  <div style={{ gridColumn: 2, minWidth: 0 }}>
                    {startsGroup && (
                      <header
                        style={{
                          display: 'flex',
                          alignItems: 'baseline',
                          justifyContent: 'space-between',
                          gap: '0.75em',
                          marginBottom: '0.2em',
                        }}
                      >
                        <strong style={{ color: speakerStyle(m.kind).tint, overflowWrap: 'anywhere' }}>
                          {/* `sender` when the speaker is known — a relayed
                              sibling-channel message, or one this console
                              posted and can attribute. Otherwise a WORD for who
                              spoke: the kinds are plumbing vocabulary, and
                              `local` printed as a name reads as though somebody
                              called "local" typed it. */}
                          <PlainText>{m.sender || speaker(m.kind)}</PlainText>
                        </strong>
                        <span style={{ display: 'inline-flex', alignItems: 'baseline', gap: '0.5em', flex: 'none' }}>
                          {m.pending && <Label isCompact color="grey">sending…</Label>}
                          {/* Reference, not the point: subdued, and pinned to
                              the far edge so the stamps line up in a column. */}
                          <time dateTime={m.at} style={{ color: 'var(--ao-text-subtle)', whiteSpace: 'nowrap' }}>
                            {new Date(m.at).toLocaleTimeString()}
                          </time>
                        </span>
                      </header>
                    )}
                    {/* AGENT PROSE IS MARKDOWN — the contract says so, and a
                        browser is the surface that can render all of it. A
                        namespace table arriving as thirty lines of pipes is what
                        plain text costs here.

                        Still never HTML: the renderer has raw HTML disabled and
                        the text is tag-stripped first, so nothing an agent
                        writes reaches the DOM as markup. */}
                    <Markdown>{m.text}</Markdown>
                  {m.choices && m.choices.length > 0 && (
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
                      {m.choices.map((c) => (
                        <Button
                          key={c.command}
                          variant="secondary"
                          isDisabled={!canWrite}
                          onClick={() => setText(c.command + ' ')}
                        >
                          <PlainText>{c.label || c.command}</PlainText>
                        </Button>
                      ))}
                    </div>
                  )}
                  </div>
                </article>
                )
              })
            )}
            </div>
          </CardBody>
        </Card>
      </StackItem>
      {noIdentity && detail.conversation.joined && !detail.archived && (
        <StackItem>
          {/* Writes are on; nobody said who is making them. Saying it here is
              the difference between a read-only console and a broken one. */}
          <Alert variant="info" isInline title="Replying needs an identity">
            {session.data?.externalAuthenticator || 'The proxy'} in front of this console
            authenticated you but forwarded no identity header, and every write is logged with the
            identity that made it. Configure it to forward X-Forwarded-Email or
            X-Auth-Request-User.
          </Alert>
        </StackItem>
      )}
      {canWrite && (
        <StackItem>
          <div style={{ position: 'relative' }}>
          {/* ABOVE THE BOX, OUT OF FLOW.
            The composer is pinned to the bottom and does not scroll — the
            message list takes the overflow — so a menu in normal flow is
            clipped by the region and pushes Send out of reach.
            It opens UPWARD because the composer sits against the bottom edge,
            and it clears the field entirely so what you typed stays visible
            while you choose. */}
          {commands && (
            <Menu
              aria-label="commands"
              data-testid="command-typeahead"
              isScrollable
              style={{
                position: 'absolute',
                bottom: '100%',
                left: 0,
                right: 0,
                zIndex: 200,
                marginBottom: 4,
                maxHeight: '40vh',
                overflowY: 'auto',
              }}
            >
            <MenuContent>
              <MenuList>
                {commands.map((c, i) => (
                  <MenuItem
                    key={c.name}
                    isFocused={i === activeCommand}
                    onClick={() => chooseCommand(c)}
                    description={c.description}
                  >
                    <Icon icon={c.icon} />{' '}
                    /{c.name}
                  </MenuItem>
                ))}
              </MenuList>
            </MenuContent>
            </Menu>
          )}
          <TextArea
            aria-label="message"
            ref={textRef}
            value={text}
            onChange={(_e, v) => {
              setText(v)
              // Typing re-opens the menu: dismissal applies to the text the
              // person escaped out of, not to the field forever.
              setDismissed(false)
              setCursor(0)
            }}
            onKeyDown={onComposerKeyDown}
            rows={3}
            placeholder="Reply to the agent…"
          />
          </div>
          {error && (
            <div style={{ marginTop: '0.75rem' }}>
              <Alert variant="danger" isInline title={error} />
            </div>
          )}
          {/* ONE ROW under the field: what the keys do on the left, the button
              on the right. Stacking them left-aligned put a button hard against
              the hint with nothing between them. */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              flexWrap: 'wrap',
              gap: '0.75rem',
              marginTop: '0.75rem',
            }}
          >
            <ComposerHint
              shortcuts={[
                { keys: ['/'], does: 'commands' },
                { keys: ['Shift', 'Enter'], does: 'send' },
              ]}
            />
            <Button onClick={send} isDisabled={busy || !text.trim()}>
              Send
            </Button>
          </div>
        </StackItem>
      )}
    </Stack>
  )
}

function RunTimeline({ detail }: { detail: NonNullable<ReturnType<typeof useConversation>['data']> }) {
  const runs = detail.conversation.runs ?? []
  return (
    <Stack hasGutter>
      <StackItem>
        <Card>
          <CardTitle>Runs</CardTitle>
          <CardBody>
            {runs.length === 0 ? (
              <Empty title="No completed runs" />
            ) : (
              <Table variant="compact" aria-label="runs">
                <Thead>
                  <Tr>
                    <Th>Run</Th>
                    <Th>Status</Th>
                    <Th>Exit</Th>
                    <Th>Finished</Th>
                    <Th>Result</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {runs.map((r) => (
                    <Tr key={r.runId}>
                      <Td dataLabel="Run">
                        <PlainText>{r.runId}</PlainText>
                      </Td>
                      <Td dataLabel="Status">
                        <Label status={r.status === 'succeeded' ? 'success' : 'danger'}>
                          <PlainText>{r.status}</PlainText>
                        </Label>
                      </Td>
                      <Td dataLabel="Exit">{r.exitCode ?? '—'}</Td>
                      <Td dataLabel="Finished">
                        {r.finishedAt ? new Date(r.finishedAt).toLocaleString() : '—'}
                      </Td>
                      <Td dataLabel="Result">
                        {r.result ? (
                          <PlainText multiline>{r.result}</PlainText>
                        ) : r.status !== 'succeeded' ? (
                          // A failure with NO output is the one an operator
                          // cannot act on, so say what it usually means rather
                          // than leaving the cell blank.
                          <Alert
                            variant="warning"
                            isInline
                            isPlain
                            title="failed before producing any output"
                          >
                            The agent process exited non-zero without a result. The common cause is
                            a session that could no longer be resumed — sessions live in the runtime
                            pod's <code>/data/home</code>, so they vanish when it is not backed by a
                            persistent volume. Check the runtime pod logs, and{' '}
                            <code>persistence.enabled</code> in the chart.
                          </Alert>
                        ) : (
                          <small>—</small>
                        )}
                      </Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            )}
          </CardBody>
        </Card>
      </StackItem>
      <StackItem>
        <Card>
          <CardTitle>Bindings and runtime</CardTitle>
          <CardBody>
            <DescriptionList isCompact>
              <DescriptionListGroup>
                <DescriptionListTerm>Inputs queued</DescriptionListTerm>
                <DescriptionListDescription>{detail.conversation.queued}</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Threads</DescriptionListTerm>
                <DescriptionListDescription>
                  {(detail.conversation.threads ?? []).map((t) => (
                    <Label key={t.channel} style={{ marginRight: 4 }}>
                      <PlainText>{`${t.channel}: ${t.threadId}`}</PlainText>
                    </Label>
                  ))}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Runtime pod</DescriptionListTerm>
                <DescriptionListDescription>
                  <PlainText>{detail.conversation.runtimePod || '—'}</PlainText>
                  {detail.runtimePodStatus?.problem && (
                    <Label status="danger">
                      <PlainText>{detail.runtimePodStatus.problem}</PlainText>
                    </Label>
                  )}
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>Toolsets it ran with</DescriptionListTerm>
                <DescriptionListDescription>
                  <PlainText>{(detail.conversation.toolsets ?? []).join(', ') || 'none'}</PlainText>
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </CardBody>
        </Card>
      </StackItem>
    </Stack>
  )
}

function ConversationGraphTab({ name }: { name: string }) {
  const { data, isLoading, error } = useConversationGraph(name)
  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorState title="Could not build the graph">{String(error)}</ErrorState>
  return (
    <Stack hasGutter>
      {data.diverged && (
        <StackItem>
          {/* The graph shows what the run ACTUALLY had. Reading the live
              pipeline instead would silently rewrite history, and the forensic
              value of this view is precisely that it does not. */}
          <Alert variant="info" isInline title="The pipeline has been re-wired since this ran">
            This graph shows the bindings this conversation materialized, not{' '}
            <PlainText>{data.pipeline}</PlainText>'s current wiring.
            <ul>
              {(data.drift ?? []).map((d, i) => (
                <li key={i}>
                  <PlainText>{d}</PlainText>
                </li>
              ))}
            </ul>
          </Alert>
        </StackItem>
      )}
      <StackItem>
        <Graph
          topology={data}
          liveEvents={data.events ?? []}
          emptyMessage="This conversation involved no elements the Display panel is showing."
        />
      </StackItem>
    </Stack>
  )
}

/**
 * The waterfall — hops in time order with per-hop latency.
 *
 * This is where "why did that take 40 seconds" gets answered, and it is the view
 * a graph cannot replace: a graph shows that an edge was used, not when or for
 * how long.
 */
function Sequence({ events }: { events: ActivityEvent[] }) {
  if (events.length === 0) {
    return (
      <Empty title="No recorded hops for this conversation">
        The manager's activity buffer is bounded — hops older than it are gone, and{' '}
        <code>status.runs[]</code> stays the durable record.
      </Empty>
    )
  }
  // MIN/MAX, not first/last. Events arrive in CURSOR order — emission order —
  // which is usually chronological and is not guaranteed to be. Taking the ends
  // of the list as the bounds produced negative offsets (`+-944.0s` in the
  // column) and a span far smaller than the real one, which then made a long
  // hop's bar hundreds of percent wide and let it escape its cell across the
  // whole table.
  const times = events.map((e) => new Date(e.ts).getTime()).filter((t) => !Number.isNaN(t))
  const start = times.length ? Math.min(...times) : 0
  const end = times.length ? Math.max(...times) : 0
  const span = Math.max(end - start, 1)

  return (
    <Card>
      <CardBody>
        <Table variant="compact" aria-label="sequence">
          <Thead>
            <Tr>
              <Th>Hop</Th>
              <Th>From → To</Th>
              <Th>At</Th>
              <Th>Latency</Th>
              <Th>Timeline</Th>
            </Tr>
          </Thead>
          <Tbody>
            {events.map((e) => {
              const at = new Date(e.ts).getTime()
              // CLAMPED. A bar is a proportion of the span and can never be
              // more than the track, however odd the underlying timestamps
              // are. Without this an out-of-order event or an unusually long
              // hop overflows an absolutely-positioned div across the page.
              const offset = Math.min(Math.max(((at - start) / span) * 100, 0), 100)
              const rawWidth = e.latencyMs ? (e.latencyMs / span) * 100 : 1
              const width = Math.min(Math.max(rawWidth, 1), 100 - offset)
              return (
                <Tr key={e.cursor}>
                  <Td dataLabel="Hop">
                    <PlainText>{e.kind}</PlainText>
                    {e.status === 'error' && <Label status="danger">error</Label>}
                  </Td>
                  <Td dataLabel="From → To">
                    <small>
                      <PlainText>
                        {`${e.from ? `${e.from.kind}/${e.from.name}` : '∅'} → ${
                          e.to ? `${e.to.kind}/${e.to.name}` : '∅'
                        }`}
                      </PlainText>
                    </small>
                    {/* Detail belongs to ITS hop. It used to be concatenated
                        into one blob under the table, where a reader could see
                        the text and not which row produced it — useless for the
                        one question detail answers. Free text from the cluster,
                        so rendered plain and never as markup. */}
                    {e.detail && (
                      <div>
                        <small style={{ color: 'var(--ao-text-subtle)', wordBreak: 'break-word' }}>
                          <PlainText>{e.detail}</PlainText>
                        </small>
                      </div>
                    )}
                  </Td>
                  <Td dataLabel="At">{`+${((at - start) / 1000).toFixed(1)}s`}</Td>
                  <Td dataLabel="Latency">
                    {e.latencyMs ? `${(e.latencyMs / 1000).toFixed(2)}s` : '—'}
                  </Td>
                  <Td dataLabel="Timeline">
                    <div
                      style={{
                        position: 'relative',
                        height: 10,
                        background: 'var(--ao-surface-alt)',
                        // The clamp above is the fix; this is the guard that
                        // keeps any future arithmetic mistake inside the cell.
                        overflow: 'hidden',
                      }}
                    >
                      <div
                        style={{
                          position: 'absolute',
                          left: `${offset}%`,
                          width: `${width}%`,
                          height: '100%',
                          background: e.status === 'error' ? 'var(--ao-danger)' : 'var(--ao-brand)',
                        }}
                      />
                    </div>
                  </Td>
                </Tr>
              )
            })}
          </Tbody>
        </Table>
      </CardBody>
    </Card>
  )
}
