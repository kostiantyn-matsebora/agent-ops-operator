package v1alpha1

import "testing"

// TestDeepCopyOnNilReceiverReturnsNilForEveryType closes the nil-receiver
// branch of every generated DeepCopy() -- `if in == nil { return nil }` --
// which TestDeepCopyRoundTripsFuzzed can never reach: NilChance(0) forces
// every field non-nil precisely so the copy machinery itself is exercised,
// leaving the RECEIVER nil check as the one branch fuzzing cannot produce.
// Table-driven over a map of closures, not one `if` per type, so the number
// of types checked never affects this function's own cognitive complexity.
func TestDeepCopyOnNilReceiverReturnsNilForEveryType(t *testing.T) {
	checks := map[string]func() bool{
		"AdapterRef":              func() bool { return (*AdapterRef)(nil).DeepCopy() == nil },
		"AgentProfile":            func() bool { return (*AgentProfile)(nil).DeepCopy() == nil },
		"AgentProfileList":        func() bool { return (*AgentProfileList)(nil).DeepCopy() == nil },
		"AgentProfileSpec":        func() bool { return (*AgentProfileSpec)(nil).DeepCopy() == nil },
		"AgentProfileStatus":      func() bool { return (*AgentProfileStatus)(nil).DeepCopy() == nil },
		"AgentRuntime":            func() bool { return (*AgentRuntime)(nil).DeepCopy() == nil },
		"AgentRuntimeList":        func() bool { return (*AgentRuntimeList)(nil).DeepCopy() == nil },
		"AgentRuntimeSpec":        func() bool { return (*AgentRuntimeSpec)(nil).DeepCopy() == nil },
		"AgentRuntimeStatus":      func() bool { return (*AgentRuntimeStatus)(nil).DeepCopy() == nil },
		"Channel":                 func() bool { return (*Channel)(nil).DeepCopy() == nil },
		"ChannelAdapter":          func() bool { return (*ChannelAdapter)(nil).DeepCopy() == nil },
		"ChannelAdapterList":      func() bool { return (*ChannelAdapterList)(nil).DeepCopy() == nil },
		"ChannelAdapterSpec":      func() bool { return (*ChannelAdapterSpec)(nil).DeepCopy() == nil },
		"ChannelAdapterStatus":    func() bool { return (*ChannelAdapterStatus)(nil).DeepCopy() == nil },
		"ChannelList":             func() bool { return (*ChannelList)(nil).DeepCopy() == nil },
		"ChannelSpec":             func() bool { return (*ChannelSpec)(nil).DeepCopy() == nil },
		"ChannelStatus":           func() bool { return (*ChannelStatus)(nil).DeepCopy() == nil },
		"ContextCheckpoint":       func() bool { return (*ContextCheckpoint)(nil).DeepCopy() == nil },
		"ContextSync":             func() bool { return (*ContextSync)(nil).DeepCopy() == nil },
		"Conversation":            func() bool { return (*Conversation)(nil).DeepCopy() == nil },
		"ConversationInput":       func() bool { return (*ConversationInput)(nil).DeepCopy() == nil },
		"ConversationInputList":   func() bool { return (*ConversationInputList)(nil).DeepCopy() == nil },
		"ConversationInputSpec":   func() bool { return (*ConversationInputSpec)(nil).DeepCopy() == nil },
		"ConversationInputStatus": func() bool { return (*ConversationInputStatus)(nil).DeepCopy() == nil },
		"ConversationList":        func() bool { return (*ConversationList)(nil).DeepCopy() == nil },
		"ConversationSpec":        func() bool { return (*ConversationSpec)(nil).DeepCopy() == nil },
		"ConversationStatus":      func() bool { return (*ConversationStatus)(nil).DeepCopy() == nil },
		"CooldownEntry":           func() bool { return (*CooldownEntry)(nil).DeepCopy() == nil },
		"CredentialKeyDoc":        func() bool { return (*CredentialKeyDoc)(nil).DeepCopy() == nil },
		"EgressMediation":         func() bool { return (*EgressMediation)(nil).DeepCopy() == nil },
		"GroupingSpec":            func() bool { return (*GroupingSpec)(nil).DeepCopy() == nil },
		"InflightRun":             func() bool { return (*InflightRun)(nil).DeepCopy() == nil },
		"InputItem":               func() bool { return (*InputItem)(nil).DeepCopy() == nil },
		"InputOrigin":             func() bool { return (*InputOrigin)(nil).DeepCopy() == nil },
		"MCPConfig":               func() bool { return (*MCPConfig)(nil).DeepCopy() == nil },
		"MCPConfigList":           func() bool { return (*MCPConfigList)(nil).DeepCopy() == nil },
		"MCPConfigSpec":           func() bool { return (*MCPConfigSpec)(nil).DeepCopy() == nil },
		"MCPConfigStatus":         func() bool { return (*MCPConfigStatus)(nil).DeepCopy() == nil },
		"MCPServer":               func() bool { return (*MCPServer)(nil).DeepCopy() == nil },
		"MCPToolset":              func() bool { return (*MCPToolset)(nil).DeepCopy() == nil },
		"MCPToolsetList":          func() bool { return (*MCPToolsetList)(nil).DeepCopy() == nil },
		"MCPToolsetSpec":          func() bool { return (*MCPToolsetSpec)(nil).DeepCopy() == nil },
		"NamedValue":              func() bool { return (*NamedValue)(nil).DeepCopy() == nil },
		"ObjectRef":               func() bool { return (*ObjectRef)(nil).DeepCopy() == nil },
		"OriginReader":            func() bool { return (*OriginReader)(nil).DeepCopy() == nil },
		"PersistenceBinding":      func() bool { return (*PersistenceBinding)(nil).DeepCopy() == nil },
		"Pipeline":                func() bool { return (*Pipeline)(nil).DeepCopy() == nil },
		"PipelineList":            func() bool { return (*PipelineList)(nil).DeepCopy() == nil },
		"PipelinePersistence":     func() bool { return (*PipelinePersistence)(nil).DeepCopy() == nil },
		"PipelineSpec":            func() bool { return (*PipelineSpec)(nil).DeepCopy() == nil },
		"PipelineStatus":          func() bool { return (*PipelineStatus)(nil).DeepCopy() == nil },
		"ReaderMark":              func() bool { return (*ReaderMark)(nil).DeepCopy() == nil },
		"RecordedInput":           func() bool { return (*RecordedInput)(nil).DeepCopy() == nil },
		"RepoAuth":                func() bool { return (*RepoAuth)(nil).DeepCopy() == nil },
		"RepositorySpec":          func() bool { return (*RepositorySpec)(nil).DeepCopy() == nil },
		"RunStatus":               func() bool { return (*RunStatus)(nil).DeepCopy() == nil },
		"SignalAdapter":           func() bool { return (*SignalAdapter)(nil).DeepCopy() == nil },
		"SignalAdapterList":       func() bool { return (*SignalAdapterList)(nil).DeepCopy() == nil },
		"SignalAdapterSpec":       func() bool { return (*SignalAdapterSpec)(nil).DeepCopy() == nil },
		"SignalAdapterStatus":     func() bool { return (*SignalAdapterStatus)(nil).DeepCopy() == nil },
		"SignalProvenance":        func() bool { return (*SignalProvenance)(nil).DeepCopy() == nil },
		"SignalSource":            func() bool { return (*SignalSource)(nil).DeepCopy() == nil },
		"SignalSourceList":        func() bool { return (*SignalSourceList)(nil).DeepCopy() == nil },
		"SignalSourceSpec":        func() bool { return (*SignalSourceSpec)(nil).DeepCopy() == nil },
		"SignalSourceStatus":      func() bool { return (*SignalSourceStatus)(nil).DeepCopy() == nil },
		"ThreadBinding":           func() bool { return (*ThreadBinding)(nil).DeepCopy() == nil },
		"ToolingBinding":          func() bool { return (*ToolingBinding)(nil).DeepCopy() == nil },
		"ToolsetBinding":          func() bool { return (*ToolsetBinding)(nil).DeepCopy() == nil },
	}
	for name, ok := range checks {
		if !ok() {
			t.Errorf("%s.DeepCopy() on a nil receiver must return nil", name)
		}
	}
}

// TestDeepCopyObjectOnNilReceiverReturnsNilForEveryRootType is the same gap
// one layer up, for every CRD root/list type's DeepCopyObject().
func TestDeepCopyObjectOnNilReceiverReturnsNilForEveryRootType(t *testing.T) {
	checks := map[string]func() bool{
		"AgentProfile":          func() bool { return (*AgentProfile)(nil).DeepCopyObject() == nil },
		"AgentProfileList":      func() bool { return (*AgentProfileList)(nil).DeepCopyObject() == nil },
		"AgentRuntime":          func() bool { return (*AgentRuntime)(nil).DeepCopyObject() == nil },
		"AgentRuntimeList":      func() bool { return (*AgentRuntimeList)(nil).DeepCopyObject() == nil },
		"Channel":               func() bool { return (*Channel)(nil).DeepCopyObject() == nil },
		"ChannelAdapter":        func() bool { return (*ChannelAdapter)(nil).DeepCopyObject() == nil },
		"ChannelAdapterList":    func() bool { return (*ChannelAdapterList)(nil).DeepCopyObject() == nil },
		"ChannelList":           func() bool { return (*ChannelList)(nil).DeepCopyObject() == nil },
		"Conversation":          func() bool { return (*Conversation)(nil).DeepCopyObject() == nil },
		"ConversationInput":     func() bool { return (*ConversationInput)(nil).DeepCopyObject() == nil },
		"ConversationInputList": func() bool { return (*ConversationInputList)(nil).DeepCopyObject() == nil },
		"ConversationList":      func() bool { return (*ConversationList)(nil).DeepCopyObject() == nil },
		"MCPConfig":             func() bool { return (*MCPConfig)(nil).DeepCopyObject() == nil },
		"MCPConfigList":         func() bool { return (*MCPConfigList)(nil).DeepCopyObject() == nil },
		"MCPToolset":            func() bool { return (*MCPToolset)(nil).DeepCopyObject() == nil },
		"MCPToolsetList":        func() bool { return (*MCPToolsetList)(nil).DeepCopyObject() == nil },
		"Pipeline":              func() bool { return (*Pipeline)(nil).DeepCopyObject() == nil },
		"PipelineList":          func() bool { return (*PipelineList)(nil).DeepCopyObject() == nil },
		"SignalAdapter":         func() bool { return (*SignalAdapter)(nil).DeepCopyObject() == nil },
		"SignalAdapterList":     func() bool { return (*SignalAdapterList)(nil).DeepCopyObject() == nil },
		"SignalSource":          func() bool { return (*SignalSource)(nil).DeepCopyObject() == nil },
		"SignalSourceList":      func() bool { return (*SignalSourceList)(nil).DeepCopyObject() == nil },
	}
	for name, ok := range checks {
		if !ok() {
			t.Errorf("%s.DeepCopyObject() on a nil receiver must return a nil interface", name)
		}
	}
}
