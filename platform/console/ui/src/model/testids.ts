// THE STABLE TEST IDENTIFIERS, part of the model contract (console-looks: "A
// selector a spec relies on SHALL be a stable test identifier supplied by the
// model layer's contract, not a class or a component name").
//
// Every look renders these on the element that carries the named fact or
// action, and every browser spec (`e2e/`, `e2e-live/`) selects through them.
// A look is free to draw anything around them, so a spec written against this
// map runs unchanged against every registered look.
//
// This file imports nothing, so a Playwright spec can import it straight from
// the source tree without pulling React or the API client into Node.
//
// Renaming a value here is a contract change: every look and every spec moves
// with it, in one change.

/** Fixed identifiers, one element each. */
export const TESTID = {
  // ---- the header ----
  /** The live-stream state. Carries `data-state` = `connected` | `disconnected`. */
  connection: 'connection',
  /** The application's own navigation panel. Hidden from assistive tech while closed. */
  appNav: 'app-nav',
  themeSwitcher: 'theme-switcher',
  lookSwitcher: 'look-switcher',

  // ---- the conversations workspace ----
  chatView: 'chat-view',
  inbox: 'inbox',
  inboxCollapsed: 'inbox-collapsed',
  mobileInboxDrawer: 'mobile-inbox-drawer',
  selectionBar: 'selection-bar',
  markRead: 'mark-read',
  markUnread: 'mark-unread',
  closeSelected: 'close-selected',
  deleteSelected: 'delete-selected',
  /**
   * A transient overlay message (a toast, a banner). A look raises one only for
   * something the viewer did (console-workspace-feedback), and every one it
   * raises carries this id, so a spec can assert an arrival raised none.
   */
  toast: 'toast',

  // ---- starting a conversation ----
  newConversation: 'new-conversation',
  newConversationUnavailable: 'new-conversation-unavailable',
  destinationCards: 'destination-cards',
  destinationFilter: 'destination-filter',
  startConversation: 'start-conversation',

  // ---- one conversation ----
  timeline: 'timeline',
  /** One message in a thread — a person's, an agent's or a signal's. Carries `data-kind`. */
  message: 'message',
  /** One run's lifecycle events, where a look shows them as their own line. */
  runEvents: 'run-events',
  /** The working indicator while a run is in flight. */
  typing: 'typing',
  jumpPill: 'jump-pill',
  runsView: 'runs-view',
  quickChips: 'quick-chips',
  commandTypeahead: 'command-typeahead',
  /** The command a conversation without a console thread can apply to join one — its value is the fix. */
  joinHintFix: 'join-hint-fix',
  closeConfirmModal: 'close-confirm-modal',
  closeConfirmOk: 'close-confirm-ok',
  /** The close dialog's "don't ask again" box — the control itself, checkable. */
  closeConfirmSkip: 'close-confirm-skip',
  /** The control that opens or folds one member invocation. */
  memberInvocationToggle: 'member-toggle',
} as const

/** Identifiers keyed by the object they belong to. */
export const testid = {
  /** An inbox scope, by its visible label ("Open", "Mine", "Closed", a Pipeline's name). */
  scope: (label: string) => `scope-${label}`,
  /** A conversation's secondary view control: `runs`, `graph`, `sequence` or `yaml`. */
  view: (id: string) => `view-${id}`,
  /** A conversation's list row. */
  row: (name: string) => `row-${name}`,
  /** The control that opens a conversation from its row. */
  open: (name: string) => `open-${name}`,
  /** A row's unread count. Absent while the conversation has nothing unread. */
  unread: (name: string) => `unread-${name}`,
  /** A row's selection box — the control itself, checkable — while selecting. */
  select: (name: string) => `select-${name}`,
  /** A destination offered by the new-conversation dialog. */
  destination: (name: string) => `destination-${name}`,
  /**
   * One member a Coordinator root invoked, inline in the root's transcript.
   * Carries `data-expanded` = `true` | `false`.
   */
  memberInvocation: (name: string) => `member-invocation-${name}`,
  /** A colour-scheme option: `light`, `dark` or `system`. */
  theme: (scheme: string) => `theme-${scheme}`,
} as const

/** Matches every conversation row, whatever its name. */
export const ANY_ROW = /^row-/
/** Matches every member invocation, whatever its name. */
export const ANY_MEMBER_INVOCATION = /^member-invocation-/
