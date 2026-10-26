package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
)

// memberWorkReportServer is a Coordinator-rooted MEMBER conversation
// (spec.causedBy set) with one inflight run, wired with a Router so
// handleWorkDone can route its result to the parent.
func memberWorkReportServer(t *testing.T, parent *agentopsv1alpha1.Conversation, budget *agentopsv1alpha1.ConversationBudget) (*Server, *agentopsv1alpha1.Conversation) {
	t.Helper()
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", "agent-ops"
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent.Name, Entry: "worker"}
	member.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1", DispatchedAt: metav1.Now()}
	member.Status.Budget = budget

	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithObjects(member, parent).
		WithStatusSubresource(member, parent).Build()
	q := &chat.OpQueue{Client: c, Namespace: "agent-ops", Registry: chat.NewRegistry()}
	router := &chat.Router{Client: c, Reader: c, Namespace: "agent-ops", Ops: q}
	return &Server{Reader: c, Client: c, Namespace: "agent-ops", Activity: activity.New(64), Router: router}, member
}

func TestHandleWorkDoneRoutesAMemberResultToItsParent(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	s, member := memberWorkReportServer(t, parent, nil)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "found the leak",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var gotParent agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &gotParent); err != nil {
		t.Fatal(err)
	}
	if len(gotParent.Spec.Inputs) != 1 || gotParent.Spec.Inputs[0].Payload != "found the leak" {
		t.Fatalf("the member's result must land as an input on the parent, got %+v", gotParent.Spec.Inputs)
	}
	if gotParent.Spec.Inputs[0].Origin == nil || gotParent.Spec.Inputs[0].Origin.Kind != agentopsv1alpha1.OriginMember ||
		gotParent.Spec.Inputs[0].Origin.Name != member.Name || gotParent.Spec.Inputs[0].Origin.Entry != "worker" {
		t.Fatalf("the input's origin must cross the causedBy edge, got %+v", gotParent.Spec.Inputs[0].Origin)
	}

	var gotMember agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: member.Name}, &gotMember); err != nil {
		t.Fatal(err)
	}
	if len(gotMember.Status.Runs) != 1 || !gotMember.Status.Runs[0].RoutedToParent {
		t.Fatalf("the run must be marked routed once the append succeeds, got %+v", gotMember.Status.Runs)
	}
}

func TestHandleWorkDoneNeverAppendsToAClosedParent(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	parent.Status.Phase = agentopsv1alpha1.ConversationClosed
	s, member := memberWorkReportServer(t, parent, nil)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "too late",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var gotParent agentopsv1alpha1.Conversation
	s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &gotParent)
	if len(gotParent.Spec.Inputs) != 0 {
		t.Fatalf("a closed parent gets nothing appended, got %+v", gotParent.Spec.Inputs)
	}
}

func TestHandleWorkDoneIncrementsTurnsOnItsOwnBudgetOnly(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	budget := &agentopsv1alpha1.ConversationBudget{MaxTurns: 5, Turns: 1}
	s, member := memberWorkReportServer(t, parent, budget)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "ok",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: member.Name}, &got)
	if got.Status.Budget == nil || got.Status.Budget.Turns != 2 {
		t.Fatalf("turns must increment on the conversation's OWN budget, got %+v", got.Status.Budget)
	}
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("turns below maxTurns must not close anything")
	}
}

func TestHandleWorkDoneClosesOnMaxTurnsReached(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	budget := &agentopsv1alpha1.ConversationBudget{MaxTurns: 2, Turns: 1}
	s, member := memberWorkReportServer(t, parent, budget)

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "ok",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: member.Name}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("reaching maxTurns must close this conversation via budget-exceeded, phase=%s", got.Status.Phase)
	}
}

// ---- the root turn's judgement (coordination-loop: "Every root turn ends in
// a move", design D-B/C/D) ---------------------------------------------------

// rootWorkReportServer is a Coordinator ROOT conversation (spec.coordinatorRef
// set) with one inflight run consuming the given inputs, beside its
// Coordinator with a two-entry roster.
func rootWorkReportServer(t *testing.T, root *agentopsv1alpha1.Conversation) *Server {
	t.Helper()
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-a", "agent-ops"
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{Name: "triage"}, {Name: "fixer"}}
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithObjects(root, co).
		WithStatusSubresource(root).Build()
	q := &chat.OpQueue{Client: c, Namespace: "agent-ops", Registry: chat.NewRegistry()}
	router := &chat.Router{Client: c, Reader: c, Namespace: "agent-ops", Ops: q}
	return &Server{Reader: c, Client: c, Namespace: "agent-ops", Activity: activity.New(64), Router: router}
}

// judgedRoot is a root whose run r1 consumed the given inputs.
func judgedRoot(moved bool, inputs ...agentopsv1alpha1.InputItem) *agentopsv1alpha1.Conversation {
	root := &agentopsv1alpha1.Conversation{}
	root.Name, root.Namespace = "root-1", "agent-ops"
	root.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}
	root.Spec.Inputs = inputs
	ids := make([]string, 0, len(inputs))
	for _, in := range inputs {
		ids = append(ids, in.ID)
	}
	root.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1", InputIDs: ids, DispatchedAt: metav1.Now(), Moved: moved}
	return root
}

func inputOf(id string, kind agentopsv1alpha1.OriginKind) agentopsv1alpha1.InputItem {
	return agentopsv1alpha1.InputItem{
		ID: id, Type: agentopsv1alpha1.InputTask, Payload: "something happened", ReceivedAt: metav1.Now(),
		Origin: &agentopsv1alpha1.InputOrigin{Kind: kind, Name: "origin-of-" + id},
	}
}

func correctiveInput() agentopsv1alpha1.InputItem {
	in := inputOf("m1", agentopsv1alpha1.OriginManager)
	in.Origin.Name = chat.MoveRuleName
	return in
}

// managerInputs returns the manager-origin inputs on a conversation.
func managerInputs(conv *agentopsv1alpha1.Conversation) []agentopsv1alpha1.InputItem {
	var out []agentopsv1alpha1.InputItem
	for _, in := range conv.Spec.Inputs {
		if in.Origin != nil && in.Origin.Kind == agentopsv1alpha1.OriginManager {
			out = append(out, in)
		}
	}
	return out
}

func getRoot(t *testing.T, s *Server) *agentopsv1alpha1.Conversation {
	t.Helper()
	var got agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &got); err != nil {
		t.Fatal(err)
	}
	return &got
}

type rootTurnCase struct {
	name          string
	root          *agentopsv1alpha1.Conversation
	status        string
	wantManager   int  // manager-origin inputs on the root afterwards
	wantEscalated bool // escalatedAt stamped afterwards
}

func rootTurnCases() []rootTurnCase {
	escalated := judgedRoot(false, correctiveInput())
	escalated.Status.EscalatedAt = &metav1.Time{Time: metav1.Now().Add(-time.Minute)}
	escalated.Status.EscalationMessage = "an earlier digest"
	closed := judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginMember))
	closed.Status.Phase = agentopsv1alpha1.ConversationClosed
	return []rootTurnCase{
		{name: "a verdict with no move is handed back",
			root: judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginMember)), status: "succeeded", wantManager: 1},
		{name: "a second miss in a row escalates and appends nothing more",
			root: judgedRoot(false, correctiveInput()), status: "succeeded", wantManager: 1, wantEscalated: true},
		{name: "a moved run appends nothing",
			root: judgedRoot(true, inputOf("i1", agentopsv1alpha1.OriginMember)), status: "succeeded"},
		{name: "answering a person is a move",
			root: judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginChannel)), status: "succeeded"},
		{name: "a person's input among others still counts as answered",
			root: judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginMember), inputOf("i2", agentopsv1alpha1.OriginChannel)), status: "succeeded"},
		{name: "a failed run is not judged",
			root: judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginMember)), status: "failed"},
		{name: "a closed root appends nothing",
			root: closed, status: "succeeded"},
		{name: "a corrective input beside another input is a first miss, not a second",
			root: judgedRoot(false, correctiveInput(), inputOf("i2", agentopsv1alpha1.OriginSignal)), status: "succeeded", wantManager: 2},
		{name: "a second miss on an already escalated root is logged and changes nothing",
			root: escalated, status: "succeeded", wantManager: 1, wantEscalated: true},
	}
}

func TestHandleWorkDoneJudgesARootTurn(t *testing.T) {
	for _, tc := range rootTurnCases() {
		t.Run(tc.name, func(t *testing.T) { runRootTurnCase(t, tc) })
	}
}

func runRootTurnCase(t *testing.T, tc rootTurnCase) {
	s := rootWorkReportServer(t, tc.root)
	rec := postWorkDone(t, s, map[string]any{
		"convo": "root-1", "runId": "r1", "status": tc.status, "result": "I conclude the leak is in api",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := getRoot(t, s)
	if n := len(managerInputs(got)); n != tc.wantManager {
		t.Fatalf("want %d manager-origin input(s), got %d: %+v", tc.wantManager, n, got.Spec.Inputs)
	}
	if escalated := got.Status.EscalatedAt != nil; escalated != tc.wantEscalated {
		t.Fatalf("escalated = %v, want %v (message %q)", escalated, tc.wantEscalated, got.Status.EscalationMessage)
	}
	if got.Status.Inflight != nil {
		t.Fatalf("the inflight entry, stamp included, must be consumed with the run, got %+v", got.Status.Inflight)
	}
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed && tc.root.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatal("a miss must never close the root")
	}
}

// The first miss in detail: the corrective input is ONE ordinary task of origin
// {manager, move-per-turn}, naming the rule and the roster, and it leaves the
// root Queued so the next dispatch is the ordinary one.
func TestHandleWorkDoneFirstMissAppendsTheCorrectiveInput(t *testing.T) {
	s := rootWorkReportServer(t, judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginMember)))
	rec := postWorkDone(t, s, map[string]any{
		"convo": "root-1", "runId": "r1", "status": "succeeded", "result": "the leak is in api",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := getRoot(t, s)
	appended := managerInputs(got)
	if len(appended) != 1 {
		t.Fatalf("want exactly one corrective input, got %+v", got.Spec.Inputs)
	}
	in := appended[0]
	if in.Type != agentopsv1alpha1.InputTask || in.Origin.Name != chat.MoveRuleName || in.ID != chat.MoveInputID("root-1", "r1") {
		t.Fatalf("the corrective input is an ordinary task of origin {manager, move-per-turn}, idempotent per run: %+v", in)
	}
	for _, want := range []string{"without a move", "invoke", "escalate", "close", "triage, fixer"} {
		if !strings.Contains(in.Payload, want) {
			t.Fatalf("the corrective text must name the rule and the roster, missing %q in %q", want, in.Payload)
		}
	}
	if got.Status.Phase != agentopsv1alpha1.ConversationQueued {
		t.Fatalf("a pending corrective input must land the phase Queued, got %s", got.Status.Phase)
	}
	if got.Status.EscalatedAt != nil {
		t.Fatal("a first miss never escalates")
	}
	if len(got.Status.Runs) != 1 || len(got.Status.Runs[0].Inputs) != 1 || got.Status.Runs[0].Inputs[0].Origin != agentopsv1alpha1.OriginMember {
		t.Fatalf("the run's own record is untouched by the judgement, got %+v", got.Status.Runs)
	}
}

// The second miss in detail: the digest says two turns ended without a move,
// names the roster and quotes the last result, and the root stays open.
func TestHandleWorkDoneSecondMissEscalatesWithTheDigest(t *testing.T) {
	s := rootWorkReportServer(t, judgedRoot(false, correctiveInput()))
	rec := postWorkDone(t, s, map[string]any{
		"convo": "root-1", "runId": "r1", "status": "succeeded", "result": "still nothing to do here",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := getRoot(t, s)
	if got.Status.EscalatedAt == nil {
		t.Fatal("a second miss in a row must escalate")
	}
	for _, want := range []string{"two turns", "without invoking, escalating or closing", "triage, fixer", "still nothing to do here"} {
		if !strings.Contains(got.Status.EscalationMessage, want) {
			t.Fatalf("the digest must say what happened, missing %q in %q", want, got.Status.EscalationMessage)
		}
	}
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("nothing was exceeded: the root stays open for the person")
	}
	if n := len(managerInputs(got)); n != 1 {
		t.Fatalf("no further corrective input after the second miss, got %d", n)
	}
}

// A person's reply resets the count: first miss, then a channel-origin run
// (answered), then a miss again — a FIRST miss, handed back rather than
// escalated.
func TestHandleWorkDoneAPersonsReplyResetsTheMissCount(t *testing.T) {
	s := rootWorkReportServer(t, judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginMember)))
	if rec := postWorkDone(t, s, map[string]any{"convo": "root-1", "runId": "r1", "status": "succeeded", "result": "hm"}); rec.Code != 200 {
		t.Fatalf("first run: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(managerInputs(getRoot(t, s))); n != 1 {
		t.Fatalf("the first miss hands the turn back once, got %d", n)
	}

	// The person replies before the corrective turn runs; the next run
	// consumes BOTH the corrective input and the reply.
	dispatchNextRun(t, s, "r2", inputOf("c1", agentopsv1alpha1.OriginChannel))
	if rec := postWorkDone(t, s, map[string]any{"convo": "root-1", "runId": "r2", "status": "succeeded", "result": "here is my answer"}); rec.Code != 200 {
		t.Fatalf("second run: %d %s", rec.Code, rec.Body.String())
	}
	got := getRoot(t, s)
	if got.Status.EscalatedAt != nil {
		t.Fatal("a run answering a person is never a miss")
	}
	if n := len(managerInputs(got)); n != 1 {
		t.Fatalf("an answered turn appends nothing, got %d", n)
	}

	// A member's result arrives, the root concludes again without a move: a
	// first miss, since the person's reply sat between the two.
	dispatchNextRun(t, s, "r3", inputOf("i3", agentopsv1alpha1.OriginMember))
	if rec := postWorkDone(t, s, map[string]any{"convo": "root-1", "runId": "r3", "status": "succeeded", "result": "hm again"}); rec.Code != 200 {
		t.Fatalf("third run: %d %s", rec.Code, rec.Body.String())
	}
	got = getRoot(t, s)
	if got.Status.EscalatedAt != nil {
		t.Fatal("the miss after a person's reply is a FIRST miss, never escalated")
	}
	if n := len(managerInputs(got)); n != 2 {
		t.Fatalf("the first miss after a reset hands the turn back again, got %d manager inputs", n)
	}
}

// dispatchNextRun appends the given inputs and marks a new run inflight
// consuming every input still pending — what the reconciler and dispatch do
// between two reports, without a pod.
func dispatchNextRun(t *testing.T, s *Server, runID string, extra ...agentopsv1alpha1.InputItem) {
	t.Helper()
	ctx := context.Background()
	root := getRoot(t, s)
	specPatch := client.MergeFrom(root.DeepCopy())
	root.Spec.Inputs = append(root.Spec.Inputs, extra...)
	if err := s.Client.Patch(ctx, root, specPatch); err != nil {
		t.Fatal(err)
	}
	root = getRoot(t, s)
	statusPatch := client.MergeFrom(root.DeepCopy())
	var ids []string
	for _, in := range dispatch.PendingInputs(root) {
		ids = append(ids, in.ID)
	}
	root.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: runID, InputIDs: ids, DispatchedAt: metav1.Now()}
	if err := s.Client.Status().Patch(ctx, root, statusPatch); err != nil {
		t.Fatal(err)
	}
}

// A member (causedBy only) is never judged: its result IS its move and goes to
// its parent, and nothing is appended to the member itself.
func TestHandleWorkDoneNeverJudgesAMember(t *testing.T) {
	parent := &agentopsv1alpha1.Conversation{}
	parent.Name, parent.Namespace = "root-1", "agent-ops"
	s, member := memberWorkReportServer(t, parent, nil)
	member.Status.Inflight.InputIDs = []string{"i1"}

	rec := postWorkDone(t, s, map[string]any{
		"convo": member.Name, "runId": "r1", "status": "succeeded", "result": "found the leak",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: member.Name}, &got); err != nil {
		t.Fatal(err)
	}
	if n := len(managerInputs(&got)); n != 0 {
		t.Fatalf("a member is never handed a corrective input, got %d", n)
	}
	if got.Status.EscalatedAt != nil {
		t.Fatal("a member is never escalated by the judgement")
	}
}

// A report naming no inflight run is not a turn: nothing was dispatched, so
// nothing is judged, whatever the status says.
func TestHandleWorkDoneJudgesOnlyTheDispatchedRun(t *testing.T) {
	s := rootWorkReportServer(t, judgedRoot(false, inputOf("i1", agentopsv1alpha1.OriginMember)))
	rec := postWorkDone(t, s, map[string]any{
		"convo": "root-1", "runId": "r-unknown", "status": "succeeded", "result": "a stray report",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if n := len(managerInputs(getRoot(t, s))); n != 0 {
		t.Fatalf("a report for a run never dispatched must not be judged, got %d corrective inputs", n)
	}
}

func TestCorrectiveInputTextNamesTheRuleAndTheRoster(t *testing.T) {
	text := correctiveInputText([]string{"triage", "fixer"})
	for _, want := range []string{"ended without a move", "Every turn ends in invoke, escalate, close", "invoke one of triage, fixer", "escalate to a person", "close with a reason"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if bare := correctiveInputText(nil); !strings.Contains(bare, "invoke one of the roster") {
		t.Fatalf("an unreadable Coordinator leaves the roster unnamed, got %q", bare)
	}
}

func TestSecondMissDigestIsBounded(t *testing.T) {
	long := strings.Repeat("é", 800)
	digest := secondMissDigest([]string{"triage"}, long)
	if !strings.Contains(digest, "two turns in a row") || !strings.Contains(digest, "Roster: triage.") {
		t.Fatalf("the digest must say what happened and who was available, got %q", digest)
	}
	if got := len([]rune(digest)); got > 700 {
		t.Fatalf("the quoted result must be bounded, digest is %d runes", got)
	}
	if !strings.HasSuffix(digest, "…") {
		t.Fatalf("a cut result must read as a fragment, got %q", digest[len(digest)-20:])
	}
	if plain := secondMissDigest(nil, ""); strings.Contains(plain, "last result") || !strings.Contains(plain, "the roster") {
		t.Fatalf("no result quotes nothing, no roster says so, got %q", plain)
	}
}

// A chat signal records origin `signal` and the surface the person typed on.
// An alert records no surface. The surface is what separates the two.
func TestAnsweredAPersonCountsAChatSignalByItsSurface(t *testing.T) {
	sig := agentopsv1alpha1.OriginSignal
	cases := []struct {
		name     string
		recorded []agentopsv1alpha1.RecordedInput
		want     bool
	}{
		{"a channel input", []agentopsv1alpha1.RecordedInput{{Origin: agentopsv1alpha1.OriginChannel}}, true},
		{"a chat signal that records its surface", []agentopsv1alpha1.RecordedInput{{Origin: sig, Surface: "home-telegram"}}, true},
		{"an alert with no surface", []agentopsv1alpha1.RecordedInput{{Origin: sig}}, false},
		{"a member result", []agentopsv1alpha1.RecordedInput{{Origin: agentopsv1alpha1.OriginMember}}, false},
		{"the corrective input", []agentopsv1alpha1.RecordedInput{{Origin: agentopsv1alpha1.OriginManager}}, false},
		{"none", nil, false},
	}
	for _, tc := range cases {
		if got := answeredAPerson(tc.recorded); got != tc.want {
			t.Errorf("%s: answeredAPerson = %v, want %v", tc.name, got, tc.want)
		}
	}
}
