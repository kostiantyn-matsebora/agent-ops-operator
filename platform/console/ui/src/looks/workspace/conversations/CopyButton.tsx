import { type ReactElement, type RefObject, useEffect, useRef, useState } from 'react'
import { CheckGlyph, CopyGlyph } from '../icons'

// THE COPY CONTROL on every message (console-workspace-look: "Every message
// can be copied").
//
// It copies the text it is given — the markdown the message was written as,
// never the rendered HTML. It confirms IN PLACE, a changed icon and label for
// a moment, and raises no toast. Where the browser refuses the clipboard it
// selects the message's text instead and says to copy it by hand.
//
// The clipboard and the selection are the only side effects, and they are
// the two functions below, so the control itself is testable with either one
// replaced.

/** How long each confirmation stays, in ms — the prototype's. */
export const COPIED_MS = 1500
export const MANUAL_MS = 3000

/** Writes text to the clipboard. Resolves false where the browser refuses. */
export async function writeClipboard(text: string): Promise<boolean> {
  try {
    if (!navigator.clipboard?.writeText) return false
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

/** Selects an element's text, so the viewer can copy it themselves. */
export function selectContents(el: Element | null): void {
  if (!el) return
  const selection = window.getSelection()
  if (!selection) return
  const range = document.createRange()
  range.selectNodeContents(el)
  selection.removeAllRanges()
  selection.addRange(range)
}

type CopyState = 'idle' | 'done' | 'manual'

const LABEL: Record<CopyState, string> = {
  idle: 'Copy message',
  done: 'Copied',
  manual: 'Copy by hand',
}

export function CopyButton({
  text,
  target,
  write = writeClipboard,
}: Readonly<{ text: string; target: RefObject<Element | null>; write?: (text: string) => Promise<boolean> }>): ReactElement {
  const [state, setState] = useState<CopyState>('idle')
  const timer = useRef<ReturnType<typeof setTimeout>>()
  useEffect(() => () => clearTimeout(timer.current), [])

  async function copy() {
    const ok = await write(text)
    if (!ok) selectContents(target.current)
    const next: CopyState = ok ? 'done' : 'manual'
    setState(next)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setState('idle'), ok ? COPIED_MS : MANUAL_MS)
  }

  return (
    <button
      type="button"
      className="wk-cp"
      data-state={state}
      aria-label={LABEL[state]}
      title={LABEL[state]}
      onClick={() => void copy()}
    >
      {state === 'done' ? <CheckGlyph /> : <CopyGlyph />}
      {state !== 'idle' && (
        <span className="said" aria-live="polite">
          {LABEL[state]}
        </span>
      )}
    </button>
  )
}
