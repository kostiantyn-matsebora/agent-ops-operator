import { type ReactElement, type ReactNode, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Block, ConversationSummary, Message } from '../../../api/types'
import {
  copyText, isOwn, replyBlocks, signalAttributes, signalSeverity,
} from '../../../model/message'
import type { RunDetails as RunDetailsFact, WorkingRun } from '../../../model/run'
import { TESTID } from '../../../model/testids'
import { chainStepLabel } from '../../../model/thread'
import { Chevron, Icon, SEVERITY_ICON } from '../icons'
import { clock, elapsed } from '../format'
import { CopyButton } from './CopyButton'
import { Markdown } from './Markdown'
import { RunDetails } from './RunDetails'

// One entry of a thread each: a signal, an agent's reply, a person's message,
// a notice, the working card. Each takes the facts the thread model shaped and
// draws them; none computes one.

/**
 * A message, wrapped with its copy control. The control is positioned over the
 * wrapper's corner, so it takes no space and no message changes size when it
 * shows (console-workspace-look).
 */
function Copyable({
  message, text, last, children,
}: Readonly<{ message: Message; text: string; last?: boolean; children: (body: React.RefObject<HTMLDivElement>) => ReactNode }>) {
  const body = useRef<HTMLDivElement>(null)
  return (
    <article className={`wk-r${last ? ' nb' : ''}`} data-testid={TESTID.message} data-kind={message.kind} data-id={message.id}>
      {children(body)}
      <CopyButton text={text} target={body} />
    </article>
  )
}

// ---- a signal ----------------------------------------------------------------

export function SignalCard({
  message, conversation, last,
}: Readonly<{ message: Message; conversation?: Pick<ConversationSummary, 'source'>; last?: boolean }>): ReactElement {
  const [open, setOpen] = useState(false)
  const severity = signalSeverity(message)
  const attributes = signalAttributes(message, conversation)
  return (
    <Copyable message={message} text={copyText(message, conversation)} last={last}>
      {(body) => (
        <div className="wk-inc">
          {/* SEVERITY IS AN ICON, never a chip, and an unrated signal shows none
              — the avatar then carries the plain signal glyph. */}
          <span className={`wk-av${severity ? ` sev-${severity}` : ''}`} data-severity={severity ?? 'unrated'}>
            {severity ? <Icon name={SEVERITY_ICON[severity]} label={`${severity} signal`} /> : <Icon name="bell" />}
          </span>
          <div className="wk-msg sig">
            {message.sender && <b className="from">{message.sender}</b>}
            <div ref={body}>
              <Markdown>{message.text}</Markdown>
            </div>
            <button type="button" className="wk-fold" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
              <span className="c">
                <Chevron dir="right" />
              </span>
              Attributes
            </button>
            {open && (
              <>
                <dl className="wk-attrs">
                  {attributes.map(([k, v]) => (
                    <div key={k} style={{ display: 'contents' }}>
                      <dt>{k}</dt>
                      <dd>{v}</dd>
                    </div>
                  ))}
                </dl>
                {message.payload && <Markdown>{'```\n' + message.payload.trim() + '\n```'}</Markdown>}
              </>
            )}
            <small>
              {clock(message.at)}
              {conversation?.source ? ` · ${conversation.source}` : ''} · signal
            </small>
          </div>
        </div>
      )}
    </Copyable>
  )
}

// ---- an agent's reply ----------------------------------------------------------

function humanLabel(label: string): string {
  const words = label.replace(/[-_]+/g, ' ').trim()
  return words ? words.charAt(0).toUpperCase() + words.slice(1) : ''
}

function DetailsFold({ text }: Readonly<{ text: string }>) {
  const [open, setOpen] = useState(false)
  const lines = text.split('\n').filter((l) => l.trim()).length
  return (
    <>
      <div className="wk-rm">
        <span>
          Details ({lines} line{lines === 1 ? '' : 's'})
        </span>
        <button type="button" className="wk-lnk" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
          {open ? 'collapse' : 'show'}
        </button>
      </div>
      {open && (
        <div className="wk-sec">
          <Markdown>{text}</Markdown>
        </div>
      )}
    </>
  )
}

function ReplyBody({ blocks }: Readonly<{ blocks: Block[] }>) {
  const details = blocks.filter((b) => b.role === 'details')
  return (
    <>
      {blocks.map((b, i) => {
        if (b.role === 'title') {
          return (
            <div key={i} className="wk-ttl">
              {b.text}
            </div>
          )
        }
        if (b.role === 'details') return null
        return (
          <section key={i} className="wk-sec">
            {b.label && <b>{humanLabel(b.label)}</b>}
            <Markdown>{b.text}</Markdown>
          </section>
        )
      })}
      {details.length > 0 && (
        <div className="wk-mr">
          {details.map((d, i) => (
            <DetailsFold key={i} text={d.text} />
          ))}
        </div>
      )}
    </>
  )
}

export const INVOKED_TESTID = (name: string) => `invoked-${name}`

/** A member the reply opened: always visible, with the control that opens it. */
function Invoked({ member }: Readonly<{ member: ConversationSummary }>) {
  return (
    <div className="wk-rm" data-testid={INVOKED_TESTID(member.name)}>
      <span>
        Invoked {chainStepLabel(member)} · {(member.phase ?? 'unknown').toLowerCase()}
      </span>
      <Link className="wk-lnk" to={`/conversations/${encodeURIComponent(member.name)}`} title={`Open ${member.name}`}>
        open conversation →
      </Link>
    </div>
  )
}

export function Reply({
  message, members = [], details, last,
}: Readonly<{ message: Message; members?: ConversationSummary[]; details?: RunDetailsFact; last?: boolean }>): ReactElement {
  const blocks = replyBlocks(message) ?? [{ role: 'section' as const, text: message.text }]
  return (
    <Copyable message={message} text={copyText(message)} last={last}>
      {(body) => (
        <div className="wk-ent">
          <span className="wk-av agent">
            <Icon name="bot" />
          </span>
          <div style={{ minWidth: 0 }}>
            <div className="hd">
              <span className="who">{message.sender || 'agent'}</span>
              <time className="wk-at" dateTime={message.at}>
                {clock(message.at)}
              </time>
            </div>
            <div ref={body}>
              <ReplyBody blocks={blocks} />
            </div>
            {(members.length > 0 || details) && (
              <div className="wk-mr">
                {members.map((m) => (
                  <Invoked key={m.name} member={m} />
                ))}
                {details && <RunDetails details={details} />}
              </div>
            )}
          </div>
        </div>
      )}
    </Copyable>
  )
}

// ---- a person's message ----------------------------------------------------------

export function PersonMessage({ message, last }: Readonly<{ message: Message; last?: boolean }>): ReactElement {
  const own = isOwn(message)
  return (
    <Copyable message={message} text={copyText(message)} last={last}>
      {(body) => (
        <div className={`wk-msg ${own ? 'own' : 'relay'}${message.pending ? ' pending' : ''}`}>
          {!own && message.sender && <b className="from">{message.sender}</b>}
          {/* A person's words are drawn as written: never parsed for blocks. */}
          <div ref={body}>
            <Markdown>{message.text}</Markdown>
          </div>
          <small>
            {message.pending ? 'sending…' : <>{clock(message.at)}{own ? ' · delivered ✓' : ''}</>}
          </small>
        </div>
      )}
    </Copyable>
  )
}

export function Notice({ message }: Readonly<{ message: Message }>): ReactElement {
  return (
    <div className="wk-notice" data-testid={TESTID.message} data-kind={message.kind}>
      <Markdown>{message.text}</Markdown>
    </div>
  )
}

// ---- the run in flight -------------------------------------------------------------

/** ONE card of a fixed size while a run works: elapsed time, the latest event and a count. */
export function WorkingCard({ working }: Readonly<{ working: WorkingRun }>): ReactElement {
  return (
    <div className="wk-working" data-testid={TESTID.typing} role="status" aria-label="the agent is working">
      <div className="l1">
        <span>Working</span>
        <span className="wk-num">{elapsed(working.elapsed)}</span>
      </div>
      <div className="l2">
        {working.latest ? working.latest.label : 'waiting for the runtime'}
        {working.count > 0 ? ` · ${working.count} event${working.count === 1 ? '' : 's'}` : ''}
      </div>
      <div className="wk-shimmer" />
    </div>
  )
}
