package kube

import (
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The state package names phases, input types and origin kinds as plain
// strings so it imports nothing from Kubernetes. This backend maps them to the
// API's typed constants byte for byte, so a value changed on one side only is a
// stored object that no longer reads back as what was written — pinned here.
func TestStateVocabularyEqualsTheStoredVocabulary(t *testing.T) {
	for _, tc := range []struct{ name, state, api string }{
		{"PhasePending", state.PhasePending, string(agentopsv1alpha1.ConversationPending)},
		{"PhaseIdle", state.PhaseIdle, string(agentopsv1alpha1.ConversationIdle)},
		{"PhaseQueued", state.PhaseQueued, string(agentopsv1alpha1.ConversationQueued)},
		{"PhaseWorking", state.PhaseWorking, string(agentopsv1alpha1.ConversationWorking)},
		{"PhaseClosed", state.PhaseClosed, string(agentopsv1alpha1.ConversationClosed)},
		{"InputTask", state.InputTask, string(agentopsv1alpha1.InputTask)},
		{"InputAlert", state.InputAlert, string(agentopsv1alpha1.InputAlert)},
		{"InputReply", state.InputReply, string(agentopsv1alpha1.InputReply)},
		{"InputRecurrence", state.InputRecurrence, string(agentopsv1alpha1.InputRecurrence)},
		{"InputJob", state.InputJob, string(agentopsv1alpha1.InputJob)},
		{"OriginSignal", state.OriginSignal, string(agentopsv1alpha1.OriginSignal)},
		{"OriginChannel", state.OriginChannel, string(agentopsv1alpha1.OriginChannel)},
		{"OriginMember", state.OriginMember, string(agentopsv1alpha1.OriginMember)},
		{"OriginManager", state.OriginManager, string(agentopsv1alpha1.OriginManager)},
		{"SignalKindChat", state.SignalKindChat, agentopsv1alpha1.SignalKindChat},
		{"LabelChatChannel", state.LabelChatChannel, agentopsv1alpha1.LabelChatChannel},
		{"LabelChatSender", state.LabelChatSender, agentopsv1alpha1.LabelChatSender},
	} {
		if tc.state != tc.api {
			t.Errorf("%s: state says %q, the stored vocabulary says %q", tc.name, tc.state, tc.api)
		}
	}
	if state.MaxRecordedInputText != agentopsv1alpha1.MaxRecordedInputText {
		t.Errorf("MaxRecordedInputText: state %d, api %d", state.MaxRecordedInputText, agentopsv1alpha1.MaxRecordedInputText)
	}
}
