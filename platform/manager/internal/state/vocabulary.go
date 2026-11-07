package state

// The record carries plain strings (design D-1: no Kubernetes type crosses
// this boundary), so the closed vocabularies the manager compares against
// are spelled once here. Each constant mirrors a typed constant of
// api/v1alpha1; internal/state/kube pins the equality in a test, so a value
// added on one side without the other fails there.

// Conversation phases.
const (
	PhasePending = "Pending"
	PhaseIdle    = "Idle"
	PhaseQueued  = "Queued"
	PhaseWorking = "Working"
	PhaseClosed  = "Closed"
)

// Input types.
const (
	InputTask       = "task"
	InputAlert      = "alert"
	InputReply      = "reply"
	InputRecurrence = "recurrence"
	InputJob        = "job"
)

// Input origin kinds.
const (
	OriginSignal  = "signal"
	OriginChannel = "channel"
	OriginMember  = "member"
	OriginManager = "manager"
)

// SignalKindChat is the chat lane's name on InputOrigin.SignalKind.
const SignalKindChat = "chat"

// Reserved labels a chat signal carries into a conversation, read by the
// delivery rule (InputItem.OriginSurface/OriginSender).
const (
	LabelChatChannel = "agentops.dev/channel"
	LabelChatSender  = "agentops.dev/sender"
)

// MaxRecordedInputText bounds how much of one message a run keeps inline
// (InputItem.Record).
const MaxRecordedInputText = 2000
