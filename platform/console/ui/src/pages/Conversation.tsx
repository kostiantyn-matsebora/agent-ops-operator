import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  Alert, Button, Card, CardBody, CardTitle, ClipboardCopy,
  DescriptionList, DescriptionListDescription, DescriptionListGroup, DescriptionListTerm,
  Label, LabelGroup, Menu, MenuContent, MenuItem, MenuList,
  PageSection, Stack, StackItem, Tab, TabTitleText, Tabs, TextArea,
  Title, Tooltip,
} from '@patternfly/react-core'
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Empty, ErrorState, Loading } from '../components/States'
import {
  useConversation, useConversationGraph, useConversations, useMarkRead, useSession, useTopology, useVocabulary,
} from '../api/hooks'
import { useStream } from '../api/stream'
import { PlainText, RawText } from '../components/Text'
import { Markdown } from '../components/Markdown'
import { Blocks, Fold, agentText } from '../components/Blocks'
import { parse } from '../api/blocks'
import { fence } from '../api/fence'
import { Graph } from '../graph/Graph'
import { useDisplay } from '../graph/display'
import { mergeEvents } from '../graph/hops'
import { api, ApiError } from '../api/client'
import { Crumbs } from '../components/Crumbs'
import { PipelineName } from '../components/PipelineName'
import { ComposerHint } from '../components/ComposerHint'
import { Icon, stripLeadingIcon } from '../components/Icon'
import { matchEntries } from './NewConversation'
import type { VocabularyEntry } from '../api/types'
import { Yaml } from '../components/Yaml'
import { MetadataCard, age } from '../components/Metadata'
import type { ActivityEvent, ConversationDetail, ConversationSummary, Run } from '../api/types'

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
  const [searchParams] = useSearchParams()
  // ?tab=incident opens straight on the Incident tab — used by the fixed
  // fixture's screenshot, and by a link from a member's "member of" banner
  // that wants the reader looking at the root's timeline, not its Transcript.
  const [tab, setTab] = useState<string | number>(searchParams.get('tab') || 0)

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
            {/* EVERY CHIP CARRIES AN ICON AND A HINT.
                The icon makes the row scannable by shape, which is why no chip
                repeats its kind in words — "source cluster-events" beside a
                source icon says it twice.
                THAT IS EXACTLY WHY THE HINT IS NOT OPTIONAL, and why it is on
                ALL of them: an icon alone does not distinguish a profile from a
                pipeline, so a row where only one chip explains itself is worse
                than one where none did. */}
            <Chip hint="the conversation's phase" color="blue" icon="aops:system">
              {c.phase}
            </Chip>
            {/* WHERE IT CAME FROM, right after the phase. It is the first thing
                anybody asks of an alert, and it used to be nowhere on this page
                — the header could name the phase, the pipeline, the profile and
                the pod of a conversation and not what started it. */}
            {c.source && (
              <Chip hint="the SignalSource that opened this conversation" color="red" icon="aops:alert">
                {c.source}
              </Chip>
            )}
            {c.pipeline ? (
              <Chip hint="the Pipeline that routed it — the wiring" color="blue" icon={pipelineIcon}>
                {c.pipeline}
              </Chip>
            ) : (
              <Chip
                hint="a Conversation records no pipelineRef; attribution is inferred from its bindings and left blank when ambiguous"
                color="grey"
              >
                unattributed
              </Chip>
            )}
            {c.profile && (
              <Chip hint="the AgentProfile that answers — who the agent is" color="purple" icon="aops:agent">
                {c.profile}
              </Chip>
            )}
            {c.runtimePod && (
              <Chip hint="the runtime pod executing this conversation" color="grey" icon="aops:workload">
                {c.runtimePod}
              </Chip>
            )}
            {c.errored && (
              <Tooltip content="the last run finished non-zero">
                <Label isCompact status="danger">last run failed</Label>
              </Tooltip>
            )}
            {/* An UN-escalated closure is a state worth its own mark: the
                conversation ended without ever opening a thread anybody could
                read the reason on, so the console is the only place it shows.
                An escalated one has `escalatedAt` and its reason is read on
                the thread it opened instead. */}
            {c.phase === 'Closed' && !c.escalatedAt && c.closeReason && (
              <Chip hint="closed without escalating a thread — the reason the manager recorded" color="orange" icon="aops:system">
                {`closed: ${c.closeReason}`}
              </Chip>
            )}
            <Chip hint="completed runs on this conversation" color="grey" icon="aops:observe">
              {c.runCount} run(s)
            </Chip>
            <Chip hint={`created ${new Date(c.created ?? '').toLocaleString()}`} color="grey" icon="⏱">
              {age(c.created)}
            </Chip>
            {(c.toolsets ?? []).map((t) => (
              <Chip key={t} hint="a bound MCPToolset — tools this conversation may call" color="orange" icon="aops:operate">
                {t}
              </Chip>
            ))}
            {(c.mcpConfigs ?? []).map((m) => (
              <Chip key={m} hint="a bound MCPConfig — the MCP servers behind those tools" color="teal" icon="aops:kubernetes">
                {m}
              </Chip>
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
            {/* Shown whenever this conversation is relevant to a coordination
                — its own root, or a member invoked by one. There is no new
                route: the tab IS the incident view (design D-G). */}
            {Boolean(c.coordinator || c.causedBy) && (
              <Tab eventKey="incident" title={<TabTitleText>Incident</TabTitleText>}>
                <IncidentTab conversation={c} />
              </Tab>
            )}
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
                        writes reaches the DOM as markup.

                        WHEN THE MANAGER PARSED IT, render the STRUCTURE — the
                        conclusion above a fold the reader opens. A message with
                        no blocks is manager-composed text, or came from a
                        manager older than contract 3, and renders exactly as it
                        did before. */}
                    {agentText(m.kind) ? <Blocks blocks={parse(m.text)} /> : <Markdown>{m.text}</Markdown>}
                    {/* A SIGNAL's event document, behind its own control. It is
                        the tallest thing in a card and the least often read —
                        the same argument `<details>` makes for an answer, and
                        what the Telegram adapter does with an expandable quote. */}
                    {m.payload && <Fold text={fence(m.payload)} label="Payload" />}
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
                          /* VERBATIM, TAGS AND ALL. This column is the RECORD —
                             what the agent actually printed — and the transcript
                             above is where it is rendered. Stripping the block
                             tags here showed a version of the answer nobody
                             produced, which is the one thing a record must not
                             do. */
                          <RawText>{r.result}</RawText>
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
                            accumulated context that could no longer be resumed — it lives in the
                            runtime pod's <code>/data/context</code>, so it vanishes when that is not
                            backed by a persistent volume. Check the runtime pod logs, and{' '}
                            <code>persistence.context.enabled</code> in the chart.
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

/**
 * The Incident tab.
 *
 * A ROOT (its own `coordinator` set) gets the interleaved timeline. A MEMBER
 * with no `coordinator` of its own gets a banner pointing at its root instead
 * — the tab's whole purpose there is telling the reader where the real
 * timeline lives, not duplicating a slice of it.
 *
 * A conversation that is BOTH (a member that is itself a nested Coordinator's
 * root) gets the timeline: design D-G says a member expands to its own nested
 * timeline in place, and this is that same rule applied to the page it is
 * reached from directly rather than through an ancestor's accordion.
 */
function IncidentTab({ conversation }: { conversation: ConversationSummary }) {
  if (conversation.coordinator) {
    return <CoordinatorIncident rootName={conversation.name} />
  }
  if (conversation.causedBy) {
    return (
      <Alert variant="info" isInline title="This is a member conversation">
        <p>
          Invoked via <strong>{conversation.causedBy.entry}</strong> from{' '}
          <Link to={`/conversations/${conversation.causedBy.parent}?tab=incident`}>
            {conversation.causedBy.parent}
          </Link>
          .
        </p>
        <p>The full incident — every member, interleaved — lives on that conversation's Incident tab.</p>
      </Alert>
    )
  }
  return null
}

/**
 * Fetches what a root's timeline needs: the root's own detail, and every
 * conversation the console currently holds so members can be found by
 * `causedBy.parent`.
 *
 * There is NO server-side "list my members" endpoint (design D-G) — a member
 * is any conversation whose `causedBy.parent` names this one, so membership is
 * derived client-side from an ordinary conversations page. This is therefore
 * bounded to what that one page returns; a root with more members than fit on
 * one page shows only the ones that do, which is the pragmatic reading of "no
 * depth limit, but also no new endpoint".
 */
function CoordinatorIncident({ rootName }: { rootName: string }) {
  const root = useConversation(rootName)
  const membersParams = useMemo(() => new URLSearchParams({ limit: '200' }), [])
  const members = useConversations(membersParams)
  if ((root.isLoading && !root.data) || (members.isLoading && !members.data)) return <Loading />
  if (root.error || !root.data) {
    return <ErrorState title="Could not load this conversation">{String(root.error)}</ErrorState>
  }
  if (members.error || !members.data) {
    return <ErrorState title="Could not load member conversations">{String(members.error)}</ErrorState>
  }
  return <CoordinatorTimeline rootDetail={root.data} allConversations={members.data.items} depth={0} />
}

interface TimelineEntry {
  at: number
  run?: Run
  member?: ConversationSummary
}

/** Root runs and direct members, merged into one chronological list. */
function buildTimeline(rootDetail: ConversationDetail, allConversations: ConversationSummary[]): TimelineEntry[] {
  const rootName = rootDetail.conversation.name
  const entries: TimelineEntry[] = (rootDetail.conversation.runs ?? []).map((run) => ({
    at: Date.parse(run.startedAt || run.finishedAt || '') || 0,
    run,
  }))
  for (const member of allConversations) {
    if (member.causedBy?.parent === rootName) {
      entries.push({ at: Date.parse(member.created || '') || 0, member })
    }
  }
  entries.sort((a, b) => a.at - b.at)
  return entries
}

/** The one timeline, root runs and member accordions interleaved by time. */
function CoordinatorTimeline({
  rootDetail,
  allConversations,
  depth,
}: {
  rootDetail: ConversationDetail
  allConversations: ConversationSummary[]
  depth: number
}) {
  const entries = useMemo(() => buildTimeline(rootDetail, allConversations), [rootDetail, allConversations])
  const budget = rootDetail.conversation.budget
  return (
    <Stack hasGutter>
      {budget && (
        <StackItem>
          <DescriptionList isCompact isHorizontal>
            <DescriptionListGroup>
              <DescriptionListTerm>Agents invoked</DescriptionListTerm>
              <DescriptionListDescription>
                {`${budget.agentsInvoked ?? 0}${budget.maxAgents ? ` of ${budget.maxAgents}` : ''}`}
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Turns</DescriptionListTerm>
              <DescriptionListDescription>
                {`${budget.turns ?? 0}${budget.maxTurns ? ` of ${budget.maxTurns}` : ''}`}
              </DescriptionListDescription>
            </DescriptionListGroup>
            {budget.deadline && (
              <DescriptionListGroup>
                <DescriptionListTerm>Deadline</DescriptionListTerm>
                <DescriptionListDescription>{new Date(budget.deadline).toLocaleString()}</DescriptionListDescription>
              </DescriptionListGroup>
            )}
          </DescriptionList>
        </StackItem>
      )}
      <StackItem>
        {entries.length === 0 ? (
          <Empty title="Nothing has happened on this incident yet" />
        ) : (
          <Stack>
            {entries.map((e) =>
              e.run ? (
                <StackItem key={`run-${e.run.runId}`}>
                  <RunEntry run={e.run} />
                </StackItem>
              ) : e.member ? (
                <StackItem key={`member-${e.member.name}`}>
                  <MemberEntry member={e.member} allConversations={allConversations} depth={depth} />
                </StackItem>
              ) : null,
            )}
          </Stack>
        )}
      </StackItem>
    </Stack>
  )
}

/** One of the root's own runs, on the timeline. */
function RunEntry({ run }: { run: Run }) {
  return (
    <div style={{ display: 'flex', alignItems: 'baseline', gap: '0.5em', padding: '0.35em 0' }}>
      <Label isCompact color="grey" icon={<Icon icon="aops:observe" />}>
        run
      </Label>
      <PlainText>{run.runId}</PlainText>
      <Label isCompact status={run.status === 'succeeded' ? 'success' : 'danger'}>
        <PlainText>{run.status}</PlainText>
      </Label>
      <small style={{ color: 'var(--ao-text-subtle)' }}>
        {run.finishedAt ? new Date(run.finishedAt).toLocaleString() : run.startedAt ? new Date(run.startedAt).toLocaleString() : ''}
      </small>
    </div>
  )
}

/**
 * One member, EXPANDABLE and collapsed by default.
 *
 * A native `<details>` rather than a component library widget: it is
 * keyboard-operable and announces its own state for free, and the only thing
 * this needs from it is an open/close event to drive the lazy fetch.
 */
function MemberEntry({
  member,
  allConversations,
  depth,
}: {
  member: ConversationSummary
  allConversations: ConversationSummary[]
  depth: number
}) {
  const [expanded, setExpanded] = useState(false)
  // Lazy: a member's own transcript and runs are fetched only once its row is
  // opened, so a root with many members does not fetch all of them at once.
  const detail = useConversation(member.name, expanded)
  return (
    <details
      style={{ marginLeft: depth * 20, borderLeft: depth ? '2px solid var(--ao-border)' : undefined, paddingLeft: depth ? '0.75em' : undefined }}
      onToggle={(e) => setExpanded((e.target as HTMLDetailsElement).open)}
    >
      <summary style={{ cursor: 'pointer', display: 'flex', alignItems: 'baseline', gap: '0.5em', flexWrap: 'wrap' }}>
        <Icon icon="aops:agent" />
        <strong>{stripLeadingIcon(member.brief || member.title || member.name)}</strong>
        {member.causedBy?.entry && (
          <Label isCompact color="purple">
            <PlainText>{member.causedBy.entry}</PlainText>
          </Label>
        )}
        <Label isCompact color={member.phase === 'Closed' ? 'grey' : 'blue'}>
          <PlainText>{member.phase}</PlainText>
        </Label>
        {member.phase === 'Closed' && !member.escalatedAt && member.closeReason && (
          <Label isCompact color="orange">
            <PlainText>{`closed: ${member.closeReason}`}</PlainText>
          </Label>
        )}
        {member.coordinator && <Label isCompact color="teal">sub-coordinator</Label>}
      </summary>
      <div style={{ padding: '0.5em 0 0.75em 1.75em' }}>
        {!expanded ? null : detail.isLoading && !detail.data ? (
          <Loading />
        ) : detail.error || !detail.data ? (
          <ErrorState title="Could not load this member">{String(detail.error)}</ErrorState>
        ) : detail.data.conversation.coordinator ? (
          // A member that is ITSELF a nested Coordinator's root: recurse,
          // in place, rather than stopping at "this is also a coordinator".
          <CoordinatorTimeline rootDetail={detail.data} allConversations={allConversations} depth={depth + 1} />
        ) : (detail.data.conversation.runs ?? []).length === 0 ? (
          <Empty title="No completed runs" />
        ) : (
          <Stack>
            {(detail.data.conversation.runs ?? []).map((r) => (
              <StackItem key={r.runId}>
                <RunEntry run={r} />
              </StackItem>
            ))}
          </Stack>
        )}
        <div style={{ marginTop: '0.5em' }}>
          <Link to={`/conversations/${member.name}`}>Open full transcript →</Link>
        </div>
      </div>
    </details>
  )
}

function ConversationGraphTab({ name }: { name: string }) {
  // The drift report reads the bindings this conversation MATERIALIZED; the
  // picture is the install's own graph, opened on this conversation's replay,
  // so every view dims what its run did not touch.
  const { data, isLoading, error } = useConversationGraph(name)
  const windowSeconds = useDisplay((s) => s.windowSeconds)
  const topology = useTopology(windowSeconds)
  const live = useStream((s) => s.events)
  const events = useMemo(() => mergeEvents(data?.events ?? [], live), [data?.events, live])
  if ((isLoading && !data) || (topology.isLoading && !topology.data)) return <Loading />
  if (error || !data) return <ErrorState title="Could not build the graph">{String(error)}</ErrorState>
  if (topology.error || !topology.data) {
    return <ErrorState title="Could not load the topology">{String(topology.error)}</ErrorState>
  }
  const oldest = topology.data.oldestEvent
  return (
    <Stack hasGutter>
      {data.diverged && (
        <StackItem>
          {/* The run's bindings are what it ACTUALLY had. Reading the live
              pipeline instead would silently rewrite history, and the forensic
              value of this view is precisely that it does not. */}
          <Alert variant="info" isInline title="The pipeline has been re-wired since this ran">
            This conversation materialized different bindings from{' '}
            <PipelineName name={data.pipeline ?? 'the pipeline'} />'s current wiring, which the graph below draws.
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
          topology={topology.data.topology}
          events={events}
          bufferStart={oldest ? Date.parse(oldest) : undefined}
          conversation={name}
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

/**
 * Chip is one header fact: an icon for its KIND, a value, and a hint saying
 * which kind that is.
 *
 * All three together, in one component, because the row only works if it is
 * consistent — the chips carry no words for their kind, so a chip without a
 * hint is a coloured glyph and a bare value.
 */
function Chip({
  children,
  hint,
  color,
  icon,
}: {
  children: React.ReactNode
  hint: string
  color: React.ComponentProps<typeof Label>['color']
  icon?: string
}) {
  return (
    <Tooltip content={hint}>
      <Label isCompact color={color} icon={icon ? <Icon icon={icon} /> : undefined}>
        {children}
      </Label>
    </Tooltip>
  )
}
