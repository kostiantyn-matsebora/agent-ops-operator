package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// POST /work/context — the context-sync sidecar reports one operation.
//
// It rides the WORK contract rather than the adapter contract, because the
// sidecar is part of a runtime pod: it is reached the same way, lives the same
// life, and authenticating it as an adapter would give a pod-local process an
// adapter's scope for no reason.
//
// The split between what is recorded WHERE is the same split the rest of the
// manager already makes:
//
//   - The STREAM of operations is declared-lossy telemetry, so it goes to the
//     activity log, which yields the console's per-conversation view and the
//     Prometheus registry from one instrumentation pass.
//   - The FACT of the latest successful checkpoint is durable state, because
//     whether a conversation has a usable context after a crash decides whether
//     it can continue at all — and that cannot depend on a ring buffer entry
//     that may have been evicted. It goes on the Conversation.

// contextReport is what the sidecar posts.
type contextReport struct {
	Kind         string `json:"kind"`
	Conversation string `json:"conversation"`
	Reason       string `json:"reason,omitempty"`
	Bytes        int64  `json:"bytes,omitempty"`
	Files        int    `json:"files,omitempty"`
	Quiesced     bool   `json:"quiesced,omitempty"`
	Found        bool   `json:"found,omitempty"`
	DurationMs   int64  `json:"durationMs,omitempty"`
	Error        string `json:"error,omitempty"`
}

// contextKinds maps the sidecar's vocabulary to activity kinds. A closed set:
// an unknown kind is refused rather than passed through, so a future sidecar
// cannot invent event kinds the metrics layer has never seen.
var contextKinds = map[string]string{
	"context.restore":    activity.KindContextRestored,
	"context.checkpoint": activity.KindContextCheckpoint,
	"context.skip":       activity.KindContextSkipped,
	"context.failed":     activity.KindContextFailed,
}

// contextCodes bounds the reason label. Anything else becomes empty rather than
// being forwarded, because Code is a metric label and an unbounded one grows
// series without limit.
var contextCodes = map[string]string{
	activity.CodeContextWorkBoundary: activity.CodeContextWorkBoundary,
	activity.CodeContextInterval:     activity.CodeContextInterval,
	activity.CodeContextShutdown:     activity.CodeContextShutdown,
	activity.CodeContextStart:        activity.CodeContextStart,
}

func (s *Server) handleContextReport(w http.ResponseWriter, r *http.Request) {
	var rep contextReport
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&rep); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	kind, ok := contextKinds[rep.Kind]
	if !ok {
		http.Error(w, "unknown context event kind", http.StatusBadRequest)
		return
	}
	if rep.Conversation == "" {
		http.Error(w, "conversation is required", http.StatusBadRequest)
		return
	}

	status := activity.StatusOK
	if rep.Error != "" {
		status = activity.StatusError
	}
	s.Activity.Emit(activity.Event{
		Kind:         kind,
		Status:       status,
		Conversation: rep.Conversation,
		From:         &activity.NodeRef{Kind: activity.NodeRuntime, Name: rep.Conversation},
		To:           &activity.NodeRef{Kind: activity.NodeConversation, Name: rep.Conversation},
		LatencyMs:    rep.DurationMs,
		Code:         contextCodes[rep.Reason],
		Detail:       contextDetail(rep),
	})

	// ONLY a checkpoint that actually transferred data updates the CR. A skip
	// writes nothing: recording every skip would patch every conversation on
	// every interval forever, which is exactly the write amplification that
	// suppressed signals already avoid.
	if kind == activity.KindContextCheckpoint && rep.Error == "" {
		if err := s.recordCheckpoint(r.Context(), rep); err != nil {
			// The checkpoint itself already succeeded on disk. Failing the
			// report would make the sidecar retry a copy that is already
			// durable, so this is logged through the response and no further.
			http.Error(w, "recorded telemetry but failed to patch status: "+err.Error(),
				http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func contextDetail(rep contextReport) string {
	if rep.Error != "" {
		return rep.Error
	}
	b, _ := json.Marshal(map[string]any{
		"bytes": rep.Bytes, "files": rep.Files, "quiesced": rep.Quiesced,
	})
	return string(b)
}

// recordCheckpoint stamps the durable half onto the Conversation.
func (s *Server) recordCheckpoint(ctx context.Context, rep contextReport) error {
	var conv agentopsv1alpha1.Conversation
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: rep.Conversation}, &conv); err != nil {
		return client.IgnoreNotFound(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.ContextCheckpoint = &agentopsv1alpha1.ContextCheckpoint{
		At:       metav1.Now(),
		Quiesced: rep.Quiesced,
		Bytes:    rep.Bytes,
	}
	// A best-effort copy is worth reporting on the conversation too: a reader
	// deciding whether continuity is safe needs to know the newest copy may
	// hold a torn file.
	if !rep.Quiesced {
		apimeta.SetStatusCondition(&conv.Status.Conditions, metav1.Condition{
			Type:               "ContextCheckpointQuiesced",
			Status:             metav1.ConditionFalse,
			Reason:             "TakenMidRun",
			Message:            "the newest durable copy was taken while a run was inflight and may hold a partially written file",
			LastTransitionTime: metav1.Now(),
		})
	}
	return s.Client.Status().Patch(ctx, &conv, patch)
}
