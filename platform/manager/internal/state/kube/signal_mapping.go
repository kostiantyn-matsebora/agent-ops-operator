package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// signalSourceKind is the kind a Signal's owner reference names.
const signalSourceKind = "SignalSource"

// toSignalRecord maps a Signal to the record it carries (api -> state), used
// by Get, List and Watch.
func toSignalRecord(sig *agentopsv1alpha1.Signal) state.SignalRecord {
	sp, st := sig.Spec, sig.Status
	rec := state.SignalRecord{
		Name:      sig.Name,
		ID:        string(sig.UID),
		CreatedAt: sig.CreationTimestamp.Time.UTC(),
		SourceID:  ownerSourceID(sig),

		SourceRef:    sp.SourceRef.Name,
		Adapter:      sp.Adapter,
		InitiationID: sp.InitiationID,
		SignalID:     sp.SignalID,
		Kind:         sp.Kind,
		Title:        sp.Title,
		Severity:     string(sp.Severity),
		Labels:       sp.Labels,
		Reader:       sp.Reader,
		Payload:      sp.Payload,

		Phase:          string(st.Phase),
		Reason:         string(st.Reason),
		Attempts:       st.Attempts,
		FirstAttemptAt: metaTimeToTimePtr(st.FirstAttemptAt),
		CompletedAt:    metaTimeToTimePtr(st.CompletedAt),
		ConsumedBy:     st.ConsumedBy,
		ConsumedAt:     metaTimeToTimePtr(st.ConsumedAt),

		ExpiresAt: expiresAtFromAnnotations(sig.Annotations),
	}
	for _, c := range st.Claimants {
		rec.Claimants = append(rec.Claimants, state.SignalClaimant{
			Kind: string(c.Ref.Kind), Name: c.Ref.Name, State: string(c.State),
			ConversationRef: objectRefToRef(c.ConversationRef), Reason: c.Reason,
		})
	}
	return rec
}

// ownerSourceID reads the opaque source identity back off the controller
// owner reference Create set.
func ownerSourceID(sig *agentopsv1alpha1.Signal) string {
	for _, o := range sig.OwnerReferences {
		if o.Kind == signalSourceKind && o.Controller != nil && *o.Controller {
			return string(o.UID)
		}
	}
	return ""
}

// applySignalSpec writes rec's publication fields and owner link onto sig. It
// is used by Create only: a stored Signal's spec is never rewritten.
func applySignalSpec(sig *agentopsv1alpha1.Signal, rec state.SignalRecord) {
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef:    agentopsv1alpha1.ObjectRef{Name: rec.SourceRef},
		Adapter:      rec.Adapter,
		InitiationID: rec.InitiationID,
		SignalID:     rec.SignalID,
		Kind:         rec.Kind,
		Title:        rec.Title,
		Severity:     agentopsv1alpha1.Severity(rec.Severity),
		Labels:       rec.Labels,
		Reader:       rec.Reader,
		Payload:      rec.Payload,
	}
	if rec.SourceID != "" {
		yes := true
		// The same reference controllerutil.SetControllerReference writes, so
		// the stored object is unchanged.
		sig.OwnerReferences = []metav1.OwnerReference{{
			APIVersion:         agentopsv1alpha1.GroupVersion.String(),
			Kind:               signalSourceKind,
			Name:               rec.SourceRef,
			UID:                types.UID(rec.SourceID),
			Controller:         &yes,
			BlockOwnerDeletion: &yes,
		}}
	}
	setExpiresAt(&sig.ObjectMeta, rec.ExpiresAt)
}

// applySignalStatus writes rec's status fields onto sig.
func applySignalStatus(sig *agentopsv1alpha1.Signal, rec state.SignalRecord) {
	st := agentopsv1alpha1.SignalStatus{
		Phase:          agentopsv1alpha1.SignalPhase(rec.Phase),
		Reason:         agentopsv1alpha1.SignalDropReason(rec.Reason),
		Attempts:       rec.Attempts,
		FirstAttemptAt: timeToMetaTimePtr(rec.FirstAttemptAt),
		CompletedAt:    timeToMetaTimePtr(rec.CompletedAt),
		ConsumedBy:     rec.ConsumedBy,
		ConsumedAt:     timeToMetaTimePtr(rec.ConsumedAt),
	}
	for _, c := range rec.Claimants {
		st.Claimants = append(st.Claimants, agentopsv1alpha1.SignalClaimant{
			Ref:             agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKind(c.Kind), Name: c.Name},
			State:           agentopsv1alpha1.ClaimantState(c.State),
			ConversationRef: refToObjectRef(c.ConversationRef),
			Reason:          c.Reason,
		})
	}
	sig.Status = st
}

// toCooldownEntries maps a source's recorded window to the contract's.
func toCooldownEntries(in []agentopsv1alpha1.CooldownEntry) []state.CooldownEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]state.CooldownEntry, len(in))
	for i, e := range in {
		out[i] = state.CooldownEntry{InitiationID: e.InitiationID, At: e.At.Time.UTC()}
	}
	return out
}

func toAPICooldownEntries(in []state.CooldownEntry) []agentopsv1alpha1.CooldownEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentopsv1alpha1.CooldownEntry, len(in))
	for i, e := range in {
		out[i] = agentopsv1alpha1.CooldownEntry{InitiationID: e.InitiationID, At: metav1.NewTime(e.At)}
	}
	return out
}
