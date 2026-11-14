import { type ReactElement, useEffect, useRef } from 'react'

// TODO(4.x): the workspace's other pages. Section 4 of console-visual-refresh
// restyles Overview, Queues, Configuration and Topology with the sections,
// order and fields they have under `classic`, and the new-conversation flow.
// Until then each route renders this marker — acceptable only while
// `workspace` is not the build default (BUILD_DEFAULT_LOOK stays `classic`
// until task 7.4), and replaced before that flip.

export function PendingPage({ title }: Readonly<{ title: string }>): ReactElement {
  return (
    <main className="wk-pane wk-page" aria-label={title} data-todo="4.x">
      <h1>{title}</h1>
      <div className="wk-card">
        <b>Not drawn in this look yet.</b>
        <div className="wk-note">
          {/* TODO(4.x) */}
          This page arrives in the workspace look in the next step. Switch the look to Classic in the header to use it
          now.
        </div>
      </div>
    </main>
  )
}

export const OverviewPage = () => <PendingPage title="Overview" />
export const QueuesPage = () => <PendingPage title="Queues and capacity" />
export const ConfigPage = () => <PendingPage title="Configuration" />
export const ConfigKindPage = () => <PendingPage title="Configuration" />
export const ConfigDetailPage = () => <PendingPage title="Configuration" />
export const TopologyPage = () => <PendingPage title="Topology" />

/** TODO(4.x): the new-conversation flow, as a dialog over the workspace. */
export function NewConversationDialog({ open, onClose }: Readonly<{ open: boolean; onClose: () => void }>): ReactElement {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    if (open && !el.open) el.showModal?.()
    if (!open && el.open) el.close?.()
  }, [open])
  return (
    <dialog ref={ref} className="wk-dialog" aria-labelledby="wk-new-title" onClose={onClose} data-todo="4.x">
      <h2 id="wk-new-title">New conversation</h2>
      <p className="wk-note">
        Starting a conversation arrives in the workspace look in the next step. Switch the look to Classic in the header
        to start one now.
      </p>
      <button type="button" className="wk-btn" onClick={onClose}>
        Close
      </button>
    </dialog>
  )
}
