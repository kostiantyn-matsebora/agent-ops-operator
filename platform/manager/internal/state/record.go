package state

import "time"

// Key names one record within a Store's fixed scope. A Store is constructed
// bound to one namespace (design D-2: "one install per namespace"), so a Key
// is the record's name alone — never a namespace, never a selector string.
type Key string

// Revision is an opaque compare-and-swap token. A backend mints and compares
// its own values; the manager only ever carries one forward from a Get or a
// successful write into the next Update. Never parsed, never constructed by
// a caller.
type Revision string

// Ref names another object this record points at (same scope), the contract
// analog of api/v1alpha1.ObjectRef. Empty means "no reference" — every
// non-empty ref this package writes is non-empty by construction upstream, so
// the empty string is never itself a valid name.
type Ref = string

// ToolsetBinding binds MCPToolset refs plus the tool-composition mode. See
// api/v1alpha1.ToolsetBinding, which this mirrors field for field.
type ToolsetBinding struct {
	Mode string
	Refs []Ref
}

// ToolingBinding binds MCPConfig refs. See api/v1alpha1.ToolingBinding.
type ToolingBinding struct {
	Refs []Ref
}

// Provenance names the conversation that invoked this one as a coordinated
// member, and the entry it was invoked through. See
// api/v1alpha1.Provenance.
type Provenance struct {
	Parent Ref
	Entry  string
}

// OriginReader names the reader who started a conversation, in the key space
// of the channel they started it from. See api/v1alpha1.OriginReader.
type OriginReader struct {
	Channel string
	Key     string
}

// SignalProvenance is the originating signal, kept for attribution. See
// api/v1alpha1.SignalProvenance.
type SignalProvenance struct {
	SourceRef Ref
	Labels    map[string]string
	Severity  string
}

// InputOrigin records where one queued input came from. See
// api/v1alpha1.InputOrigin.
type InputOrigin struct {
	Kind       string
	Name       string
	Entry      string
	SignalKind string
	Sender     string
}

// InputItem is one queued work unit. Payload carries the text inline, or
// PayloadRef names the blob that carries it out-of-line — never both. See
// api/v1alpha1.InputItem.
//
// Agent is the deprecated, no-longer-written override
// api/v1alpha1.InputItem.Agent: dispatch still reads it for one release
// (a queued item written before that release carries it), so a record
// carries it through untouched. Nothing in this contract writes it, and it
// is removed with the dual-read.
type InputItem struct {
	ID         string
	Type       string
	Agent      string
	Payload    string
	PayloadRef Key
	Origin     *InputOrigin
	ReceivedAt time.Time
}

// RecordedInput is one message a run consumed, kept on the run exactly as
// written (the inline cap api/v1alpha1.BoundConversationTitle-style helpers
// apply is a manager-side concern, not this contract's). See
// api/v1alpha1.RecordedInput.
type RecordedInput struct {
	ID         string
	Type       string
	Text       string
	Truncated  bool
	PayloadRef Key
	Surface    string
	Sender     string
	Origin     string
	ReceivedAt *time.Time
}

// Run records one completed agent run. Runs are append-only sub-records of a
// ConversationRecord (design D-5): added through Store.AppendRun, read
// through Store.ListRuns, never part of Get/Update's own record. Three
// delivery markers on an existing run (DeliveryTracked, Delivered,
// RoutedToParent) only ever grow, and change through Store.UpdateRun (design
// D-12). See api/v1alpha1.RunStatus.
type Run struct {
	RunID           string
	JobKind         string
	Status          string
	ExitCode        *int32
	Result          string
	StartedAt       *time.Time
	FinishedAt      *time.Time
	InputIDs        []string
	DeliveryTracked bool
	Delivered       []string
	RoutedToParent  bool
	Inputs          []RecordedInput
}

// RunUpdate is the one mutation an existing run accepts (design D-12). Every
// field only ever grows — a flag goes false to true, a set gains members,
// nothing is removed or overwritten — so an update is idempotent and commutes
// with another UpdateRun and with AppendRun. A zero RunUpdate changes nothing.
type RunUpdate struct {
	// MarkDeliveryTracked sets Run.DeliveryTracked true.
	MarkDeliveryTracked bool
	// MarkDelivered adds each channel to Run.Delivered unless it is already
	// there, keeping first-appearance order.
	MarkDelivered []string
	// MarkRoutedToParent sets Run.RoutedToParent true.
	MarkRoutedToParent bool
}

// ThreadBinding pins one bound channel to its conversation thread. See
// api/v1alpha1.ThreadBinding.
type ThreadBinding struct {
	Channel          string
	ThreadID         string
	ReadAt           *string
	ReadTracked      bool
	Readers          []ReaderMark
	UndeliveredReply string
	Claim            *OpClaim
}

// ReaderMark is one identity's watermark on a thread. See
// api/v1alpha1.ReaderMark.
type ReaderMark struct {
	Key    string
	ReadAt *string
}

// OpClaim records which replica is dispatching a thread's pending op, and
// since when. See api/v1alpha1.OpClaim.
type OpClaim struct {
	Holder    string
	ClaimedAt time.Time
}

// ConversationBudget is a Coordinator-rooted conversation's own resource
// ceiling and the running counts against it. See
// api/v1alpha1.ConversationBudget.
type ConversationBudget struct {
	MaxAgents     int32
	MaxTurns      int32
	Deadline      *time.Time
	AgentsInvoked int32
	Turns         int32
}

// InflightRun tracks the unit currently dispatched to the runtime pod. See
// api/v1alpha1.InflightRun.
type InflightRun struct {
	RunID        string
	InputIDs     []string
	DispatchedAt time.Time
	Moved        bool
}

// ContextCheckpoint is one durable copy of a conversation's context. See
// api/v1alpha1.ContextCheckpoint.
type ContextCheckpoint struct {
	At         time.Time
	Generation string
	Quiesced   bool
	Bytes      int64
}

// Condition is one observed-state condition, the contract analog of
// metav1.Condition — carried so a conversation's conditions round-trip
// through a backend that is not the apiserver.
type Condition struct {
	Type               string
	Status             string
	Reason             string
	Message            string
	ObservedGeneration int64
	LastTransitionTime time.Time
}

// ConversationRecord is the conversation control record — every field of
// api/v1alpha1.ConversationSpec and api/v1alpha1.ConversationStatus except
// Runs (an append-only sub-record, see Run/Store.AppendRun) and the inline
// text a large InputItem.Payload defers to a blob (see Store.PutBlob).
//
// No Kubernetes type appears here (design D-1): a backend that is not the
// apiserver has no ObjectMeta, no resourceVersion, no finalizer to fake. The
// `kube` backend alone maps this to and from the CRD it keeps.
type ConversationRecord struct {
	Name string

	// ID, CreatedAt and DeletionRequestedAt are minted by the backend and
	// read-only: Create and Update ignore whatever a caller puts here, and a
	// record read back always carries them (design D-11).
	//
	// ID is the record's identity, opaque and stable for its whole life. A
	// record deleted and created again under the same Name has a new ID, so
	// anything derived from the ID (a topic-creation op id, an owner link on
	// a workload) never confuses the two lives.
	//
	// CreatedAt is when the backend first stored the record. It orders FIFO
	// admission and is the idle clock's floor for a record with no activity.
	//
	// DeletionRequestedAt is non-nil from the first Delete of a record that
	// still holds a finalizer, until the record is gone.
	ID                  string
	CreatedAt           time.Time
	DeletionRequestedAt *time.Time

	// Finalizers is the set of holds the manager has placed on the record:
	// while it is non-empty, a Delete only marks the record
	// (DeletionRequestedAt) and the record stays readable. Caller-owned,
	// written through Update, and on a record being deleted it can only
	// shrink (design D-11).
	Finalizers []string

	// NameBase is the word-chain a record's name was allocated from,
	// "<kind>-<words>" with no numeric collision suffix. Set at Create and
	// never changed. It is the key of the ByNameBase index.
	NameBase string

	CausedBy              *Provenance
	CoordinatorRef        Ref
	EscalationChannelRefs []Ref
	ChannelRefs           []Ref
	ProfileRef            Ref
	PipelineRef           Ref
	Signal                *SignalProvenance
	Title                 string
	ConversationID        string
	OriginReader          *OriginReader
	Toolsets              *ToolsetBinding
	MCPConfigs            *ToolingBinding
	RuntimeRef            Ref
	ServiceAccountName    string
	ContextClaimName      string
	WorkspaceClaimName    string
	Inputs                []InputItem

	Phase                   string
	Threads                 []ThreadBinding
	RuntimeContextID        string
	SessionID               string
	RuntimePod              string
	Inflight                *InflightRun
	ProcessedInputIDs       []string
	LastActivity            *time.Time
	ClosedAt                *time.Time
	Budget                  *ConversationBudget
	EscalatedAt             *time.Time
	EscalationMessage       string
	CloseReason             string
	Brief                   string
	ThreadsArchived         []string
	Reopens                 int32
	ContextCheckpoint       *ContextCheckpoint
	RuntimeStartFailures    int32
	LastRuntimeStartFailure *time.Time
	Conditions              []Condition

	// ExpiresAt marks a record invisible to Get/List and delivered as a
	// delete Event on Watch once passed (task 1.1, 2.6). No conversation
	// written by this change's manager sets it — it exists for
	// signal-persistence's terminal records to join the same contract.
	ExpiresAt *time.Time
}

// Blob is the out-of-line body of a queued InputItem whose Payload a caller
// chose to keep off the control record — the contract analog of
// api/v1alpha1.ConversationInputSpec, minus ConsumedAt/Consumed (dead fields:
// nothing in the manager reads them; a consumed input is pruned from
// ConversationRecord.Inputs instead).
type Blob struct {
	ConversationName string
	Type             string
	Payload          string
	Labels           map[string]string
	Severity         string
}
