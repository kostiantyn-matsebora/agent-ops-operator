package commandbus

// Sequence is a Feed resume token: a strictly increasing position across
// every queue's task-state transitions, assigned once per transition rather
// than read from a wall clock (design D-14, 2.6 — "a gap-free resumable
// feed"), so two transitions landing in the same instant still order and
// resume correctly. The zero Sequence means "start from now", mirroring
// internal/state.Revision's empty-resume convention.
type Sequence uint64

// TaskEvent is one entry on Feed: a task's state immediately after one
// transition. Sequence is both this event's own position and the resume
// token to continue strictly after it.
type TaskEvent struct {
	Sequence     Sequence
	UID          string
	Queue        Queue
	Kind         Kind
	Key          Key
	CommandID    CommandID
	InvocationID string
	State        TaskState
	Attempt      int
	Code         ErrorCode
}
