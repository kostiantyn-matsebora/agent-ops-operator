import { type ReactElement, useState } from 'react'
import { TESTID } from '../../../model/testids'
import { Dialog } from './Dialog'

// THE CONFIRMATION BEFORE `/close`. Never `/exit`, which only releases the
// runtime and loses nothing. "Don't ask again" is a per-browser convenience,
// re-defaulted to unchecked every time the dialog opens: an opt-in that
// remembers itself is not one.

export function CloseConfirm({ onConfirm, onCancel }: Readonly<{ onConfirm: (dontAskAgain: boolean) => void; onCancel: () => void }>): ReactElement {
  const [skip, setSkip] = useState(false)
  return (
    <Dialog
      title="Close this conversation?"
      testid={TESTID.closeConfirmModal}
      onClose={onCancel}
      footer={
        <>
          <button type="button" className="wk-btn danger" data-testid={TESTID.closeConfirmOk} onClick={() => onConfirm(skip)}>
            Close
          </button>
          <button type="button" className="wk-btn link" onClick={onCancel}>Cancel</button>
        </>
      }
    >
      <p>
        The agent says goodbye and the thread is archived. The conversation itself stays: its answers and its
        workspace are kept, and it can be reopened.
      </p>
      <label className="wk-check">
        <input type="checkbox" data-testid={TESTID.closeConfirmSkip} checked={skip} onChange={(e) => setSkip(e.target.checked)} />
        Don't ask again
      </label>
    </Dialog>
  )
}
