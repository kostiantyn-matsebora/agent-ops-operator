package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

// A CONVERSATION KEEPS ITS QUESTIONS, NOT ONLY ITS ANSWERS.
//
// status.runs[] was durable and spec.inputs[] was a queue, so pruning the queue
// destroyed the only copy of what a person said and a viewer could rebuild half
// a thread. These tests pin the two halves of the fix: the record is written
// with the run that consumed the input, and delivery is decided per DESTINATION.

// mkQueuedConv creates a chat-less conversation holding one queued input, so a
// work unit can be dispatched with no thread to bind first.
func mkQueuedConv(t *testing.T, name, profile string, item agentopsv1alpha1.InputItem) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{item}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, name) })
	return conv
}

// runOne dispatches the pending unit and reports it done, which is where the
// consumed inputs are recorded.
func runOne(t *testing.T, srv *httpapi.Server, conv string, result string) {
	t.Helper()
	rec := adapterReq(srv, "GET", "/work?convo="+conv+"&wait=0", nil, "")
	if rec.Code != 200 {
		t.Fatalf("dispatch: %d %s", rec.Code, rec.Body.String())
	}
	var unit struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &unit); err != nil {
		t.Fatal(err)
	}
	rec = adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": conv, "runId": unit.RunID, "status": "succeeded", "result": result,
	}, "")
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
}

// The queue is pruned, as it always was. What it may no longer take with it is
// the only copy of the message.
func TestConsumedInputSurvivesPruning(t *testing.T) {
	mkProfile(t, "prof-record")
	srv := apiServer()
	mkQueuedConv(t, "record-conv", "prof-record", agentopsv1alpha1.InputItem{
		ID: "in-1", Type: agentopsv1alpha1.InputTask, Payload: "why is api down?",
		Origin: &agentopsv1alpha1.InputOrigin{
			Kind: agentopsv1alpha1.OriginChannel, Name: "console", Sender: "kostya@example.com",
		},
	})
	runOne(t, srv, "record-conv", "it was OOMKilled")

	// The record and the processed marker are ONE write: at this point nothing
	// has reconciled, so a crash here is exactly the window the ordering
	// guarantee covers — and the message is already durable.
	conv := getConv(t, "record-conv")
	if len(conv.Status.ProcessedInputIDs) != 1 || conv.Status.ProcessedInputIDs[0] != "in-1" {
		t.Fatalf("the input must be marked processed: %+v", conv.Status.ProcessedInputIDs)
	}
	if len(conv.Status.Runs) != 1 || len(conv.Status.Runs[0].Inputs) != 1 {
		t.Fatalf("the run must record what it consumed: %+v", conv.Status.Runs)
	}
	rec := conv.Status.Runs[0].Inputs[0]
	if rec.ID != "in-1" || rec.Text != "why is api down?" {
		t.Fatalf("the record must hold the message: %+v", rec)
	}
	if rec.Surface != "console" || rec.Sender != "kostya@example.com" {
		t.Fatalf("the record must say where it entered and who typed it: %+v", rec)
	}
	if rec.Truncated {
		t.Fatalf("an ordinary message is kept whole: %+v", rec)
	}

	// now prune, exactly as before — and read the message back afterwards
	reconcileWithOps(t, srv, "record-conv")
	conv = getConv(t, "record-conv")
	if len(conv.Spec.Inputs) != 0 {
		t.Fatalf("pruning must still empty the queue: %+v", conv.Spec.Inputs)
	}
	if len(conv.Status.Runs[0].Inputs) != 1 ||
		conv.Status.Runs[0].Inputs[0].Text != "why is api down?" {
		t.Fatalf("the message must outlive its queue entry: %+v", conv.Status.Runs[0].Inputs)
	}
}

// A large payload lives out of line so the Conversation stays small. The record
// must not undo that by copying it in.
func TestOversizedPayloadIsBoundedInTheRecord(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-bigrec")
	srv := apiServer()

	big := strings.Repeat("x", agentopsv1alpha1.MaxRecordedInputText+500)
	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Name, ci.Namespace = "bigrec-payload", ns
	ci.Spec = agentopsv1alpha1.ConversationInputSpec{
		ConversationRef: agentopsv1alpha1.ObjectRef{Name: "bigrec-conv"},
		Type:            agentopsv1alpha1.InputAlert,
		Payload:         big,
		Labels:          map[string]string{"alertname": "Huge"},
	}
	if err := k8sClient.Create(ctx, ci); err != nil {
		t.Fatal(err)
	}
	mkQueuedConv(t, "bigrec-conv", "prof-bigrec", agentopsv1alpha1.InputItem{
		ID: "in-big", Type: agentopsv1alpha1.InputAlert,
		PayloadRef: &agentopsv1alpha1.ObjectRef{Name: "bigrec-payload"},
		Origin:     &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "src", SignalKind: "alert"},
	})
	runOne(t, srv, "bigrec-conv", "looked at it")

	rec := getConv(t, "bigrec-conv").Status.Runs[0].Inputs[0]
	if !rec.Truncated {
		t.Fatalf("an oversized payload must be marked as a fragment: %+v", rec.Truncated)
	}
	if len([]rune(rec.Text)) != agentopsv1alpha1.MaxRecordedInputText {
		t.Fatalf("the record must inline exactly the cap, got %d runes", len([]rune(rec.Text)))
	}
	if rec.PayloadRef == nil || rec.PayloadRef.Name != "bigrec-payload" {
		t.Fatalf("the record must cite where the full payload lived: %+v", rec.PayloadRef)
	}
}

// A run written before the record existed has none, and nothing invents one.
func TestRunPredatingTheRecordHasNoInputs(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-oldrun")
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "oldrun-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-oldrun"}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "r-old", Status: "succeeded", Result: "an answer from before",
		InputIDs: []string{"in-gone"}, DeliveryTracked: true,
	}}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}
	got := getConv(t, "oldrun-conv")
	if len(got.Status.Runs) != 1 || got.Status.Runs[0].Inputs != nil {
		t.Fatalf("an older run carries no inputs and gains none: %+v", got.Status.Runs)
	}
	if got.Status.Runs[0].Result != "an answer from before" {
		t.Fatalf("what it does hold stays readable: %+v", got.Status.Runs[0])
	}
}

// Two surfaces served by ONE adapter: the decision is per SURFACE, not per
// transport, so the sibling receives the message and the origin does not.
func TestDeliveryIsDecidedPerSurfaceNotPerTransport(t *testing.T) {
	mkProfile(t, "prof-twosurf")
	mkAdapter(t, "tg-twosurf")
	mkTypedChannel(t, "surf-one", "tg-twosurf", "")
	mkTypedChannel(t, "surf-two", "tg-twosurf", "")
	mkChatSource(t, "src-twosurf", "surf-one")
	mkPipeline(t, "twosurf-pipe", []string{"src-twosurf"}, []string{"surf-one", "surf-two"}, "prof-twosurf")
	reconcilePipeline(t, "twosurf-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "src-twosurf", "surf-one", "anyone home?"); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := convForProfile(t, "prof-twosurf")
	if conv == nil {
		t.Fatal("conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	reconcileWithOps(t, srv, conv.Name)
	// one adapter, two surfaces: both ensure-topics come back on the same poll
	bindThread(t, srv, drainOps(t, srv, "tg-twosurf"), "t-shared")
	for i := 0; i < 3; i++ {
		reconcileWithOps(t, srv, conv.Name)
	}
	var relays []chat.Op
	for _, op := range drainOps(t, srv, "tg-twosurf") {
		if op.Message != nil && op.Message.Kind == chat.MsgRelay {
			relays = append(relays, op)
		}
	}
	if len(relays) != 1 {
		t.Fatalf("exactly the surface that did not display it is owed the message, got %d", len(relays))
	}
	if relays[0].Channel != "surf-two" || relays[0].Message.Origin != "surf-one" {
		t.Fatalf("the sibling surface receives it, attributed to the origin: %+v", relays[0])
	}
}

// An alert entered on no surface at all, so every bound channel is owed it.
func TestAlertReachesEveryChannel(t *testing.T) {
	mkProfile(t, "prof-allchan")
	mkChannel(t, "allchan-a", "tg-allchan-a")
	mkChannel(t, "allchan-b", "tg-allchan-b")
	mkSignalSource(t, "src-allchan", "am-allchan", "")
	mkPipeline(t, "allchan-pipe", []string{"src-allchan"}, []string{"allchan-a", "allchan-b"}, "prof-allchan")
	reconcilePipeline(t, "allchan-pipe")
	srv := apiServer()

	rec := postSignal(t, srv.Handler(), testMasterToken, "src-allchan", []map[string]any{{
		"fingerprint": "allchan-1", "labels": map[string]string{"alertname": "NobodyReadsMe"},
		"payload":     "disk at 99%",
	}})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := convForProfile(t, "prof-allchan")
	if conv == nil {
		t.Fatal("conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	reconcileWithOps(t, srv, conv.Name)
	for _, adapter := range []string{"tg-allchan-a", "tg-allchan-b"} {
		bindThread(t, srv, drainOps(t, srv, adapter), "th-"+adapter)
	}
	reconcileWithOps(t, srv, conv.Name)
	for _, adapter := range []string{"tg-allchan-a", "tg-allchan-b"} {
		cards := cardsFor(drainOps(t, srv, adapter))
		if len(cards) != 1 || !strings.Contains(cards[0].Message.Body, "disk at 99%") {
			t.Fatalf("%s: every channel is owed an event no surface displayed: %+v", adapter, cards)
		}
	}
}

// A viewer renders only what it is sent, so it receives its own users' messages
// — the case the origin-kind rule got wrong, and the reason a console
// transcript began at the agent's answer.
func TestSurfaceThatDoesNotEchoReceivesItsOwnUsersMessages(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-viewer")
	viewer := &agentopsv1alpha1.ChannelAdapter{}
	viewer.Name, viewer.Namespace = "viewer-adapter", ns
	viewer.Spec.Image = "example/console:1"
	no := false
	viewer.Spec.EchoesOwnMessages = &no
	if err := k8sClient.Create(ctx, viewer); err != nil {
		t.Fatal(err)
	}
	mkTypedChannel(t, "viewer-chan", "viewer-adapter", "")
	mkChatSource(t, "src-viewer", "viewer-chan")
	mkPipeline(t, "viewer-pipe", []string{"src-viewer"}, []string{"viewer-chan"}, "prof-viewer")
	reconcilePipeline(t, "viewer-pipe")
	srv := apiServer()

	if rec := chatSignal(t, srv, "src-viewer", "viewer-chan", "start here"); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := convForProfile(t, "prof-viewer")
	if conv == nil {
		t.Fatal("conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	reconcileWithOps(t, srv, conv.Name)
	bindThread(t, srv, drainOps(t, srv, "viewer-adapter"), "v-1")
	for i := 0; i < 3; i++ {
		reconcileWithOps(t, srv, conv.Name)
	}
	var relays []chat.Op
	for _, op := range drainOps(t, srv, "viewer-adapter") {
		if op.Message != nil && op.Message.Kind == chat.MsgRelay {
			relays = append(relays, op)
		}
	}
	if len(relays) != 1 || !strings.Contains(relays[0].Message.Body, "start here") {
		t.Fatalf("a surface that displays nothing itself must be sent the message: %+v", relays)
	}
	// and exactly once, however many times it is re-derived
	for i := 0; i < 3; i++ {
		reconcileWithOps(t, srv, conv.Name)
	}
	for _, op := range drainOps(t, srv, "viewer-adapter") {
		if op.Message != nil && op.Message.Kind == chat.MsgRelay {
			t.Fatalf("re-derivation must dedup on the stable op id: %+v", op)
		}
	}
}
