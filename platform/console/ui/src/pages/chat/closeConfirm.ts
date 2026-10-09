// `/close` can be destructive once the retention window has run — see
// `invariants.md`'s "`/exit` RELEASES THE RUNTIME — `/close` ENDS THE
// CONVERSATION" for why the two are not interchangeable. `/exit` needs no
// confirmation at all: it is fully recoverable. `/close` gets an ordinary
// confirm dialog, with a "don't ask again" opt-out.
//
// The opt-out is a per-browser CONVENIENCE, not conversation state — the same
// category as `layout.ts`'s pane widths, never the "nothing CORRECTNESS-BEARING
// is persisted" rule `docs/console.md` states for cluster state and
// transcripts. Guarded on both sides exactly like `layout.ts`, so a private
// tab or a full/disabled store still renders and still asks.

const SKIP_CLOSE_CONFIRM_KEY = 'agentops.console.skipCloseConfirm'

/** True once the operator has opted out of the confirm dialog for `/close`. */
export function skipCloseConfirm(): boolean {
  try {
    return localStorage.getItem(SKIP_CLOSE_CONFIRM_KEY) === '1'
  } catch {
    return false
  }
}

/** writeSkipCloseConfirm never throws — a full or disabled store simply does not remember. */
export function writeSkipCloseConfirm(skip: boolean): void {
  try {
    if (skip) localStorage.setItem(SKIP_CLOSE_CONFIRM_KEY, '1')
    else localStorage.removeItem(SKIP_CLOSE_CONFIRM_KEY)
  } catch {
    // The dialog already behaved as asked this time; the next session asks again.
  }
}

/**
 * Whether typed text IS the close command — exactly `/close`, or `/close`
 * followed by a reason. Never a prefix match: `/closeup` is ordinary text,
 * not the command.
 */
export function isCloseCommand(text: string): boolean {
  const t = text.trim()
  return t === '/close' || t.startsWith('/close ')
}
