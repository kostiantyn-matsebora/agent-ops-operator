package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// sigTestNamespace is the namespace every fixture in this file shares.
const sigTestNamespace = "agent-ops"

// sigServer builds a Server wired for /signal/inbound: a fake client seeded
// with the given objects. Publication only creates Signals, so the Signal
// status subresource is all it needs; the Router is the real one over the same
// client, for the chat commands that answer in place.
func sigServer(t *testing.T, objs ...client.Object) (*Server, client.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}, &agentopsv1alpha1.Signal{}).
		WithIndex(&agentopsv1alpha1.Conversation{}, agentopsv1alpha1.IndexConversationID, controller.ConversationIDIndexer).
		WithObjects(objs...).Build()
	acts := activity.New(64)
	q := &chat.OpQueue{Client: c, Namespace: sigTestNamespace, Registry: chat.NewRegistry(), Activity: acts}
	router := &chat.Router{Client: c, Reader: c, Namespace: sigTestNamespace, Ops: q, Activity: acts}
	return &Server{Reader: c, Client: c, Namespace: sigTestNamespace, AdapterToken: "sig-test-master",
		Ops: q, Router: router, Activity: acts}, c
}

// sigTestSource is a minimal SignalSource fixture.
func sigTestSource(name, adapter string) *agentopsv1alpha1.SignalSource {
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, sigTestNamespace
	src.Spec.Adapter = adapter
	return src
}

// falsePtr is a *bool pointing at false, for setting SignalSource.Spec.LogFiltered.
func falsePtr() *bool { b := false; return &b }

// sigTestPipeline is a Ready Pipeline claiming the given source, its
// capability inlined — no AgentProfile/AgentRuntime object is needed, since
// ResolveCapability/SnapshotFor both degrade gracefully to the bootstrap
// default when neither resolves.
func sigTestPipeline(name, source string) *agentopsv1alpha1.Pipeline {
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = name, sigTestNamespace
	p.Spec.SignalSourceRefs = []agentopsv1alpha1.ObjectRef{{Name: source}}
	p.Spec.AgentCapabilitySpec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "prof-" + name}
	p.Status.Conditions = []metav1.Condition{{
		Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "ready",
		LastTransitionTime: metav1.Now(),
	}}
	return p
}

func postSig(s *Server, source string, signals []map[string]any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"source": source, "signals": signals})
	req := httptest.NewRequest("POST", "/signal/inbound", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+s.AdapterToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// signal-severity: "An unknown value is refused" — the 400 must name all
// four accepted values, and the loop runs ahead of s.signalSource (no Client
// list ever happens), so a bare zero-value Server already exercises it —
// same style as TestMalformedBodyIsRefusedWithoutReadingAnything.
func TestSignalInboundRefusesUnknownSeverityNamingAcceptedValues(t *testing.T) {
	s := &Server{}
	reqBody, _ := json.Marshal(map[string]any{"source": "any-source", "signals": []map[string]any{
		{"initiationId": "sev-1", "payload": "boom", "severity": "fatal"},
	}})
	req := httptest.NewRequest("POST", "/signal/inbound", strings.NewReader(string(reqBody)))
	rec := httptest.NewRecorder()
	// Called directly, bypassing signalAuth — the validation loop runs before
	// any Client call, same reasoning TestMalformedBodyIsRefusedWithoutReadingAnything
	// gives for calling a handler on a bare Server{} directly.
	s.handleSignalInbound(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	respBody := rec.Body.String()
	for _, want := range []string{"critical", "error", "warning", "info"} {
		if !strings.Contains(respBody, want) {
			t.Fatalf("body = %q, must name the accepted value %q", respBody, want)
		}
	}
}

// signal-severity: "One bad signal refuses the whole batch" — a batch with
// one valid signal and one carrying an unmapped severity is refused whole,
// and NOTHING is created, mirrored on the same "list conversations before
// and after" pattern the correlation/batch tests in internal/integration use
// (here via the fake client directly, since this package builds its own
// Server rather than reaching for envtest).
func TestSignalInboundOneBadSeverityRefusesWholeBatch(t *testing.T) {
	source := sigTestSource("sev-batch-src", "sev-batch-t")
	pipeline := sigTestPipeline("sev-batch-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "sev-batch-ok", "payload": "fine", "severity": "critical"},
		{"initiationId": "sev-batch-bad", "payload": "boom", "severity": "fatal"},
	})
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 0 {
		t.Fatalf("one bad signal must refuse the WHOLE batch — want 0 Signals, got %d", len(convs.Items))
	}
	var inputs agentopsv1alpha1.ConversationInputList
	if err := c.List(context.Background(), &inputs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(inputs.Items) != 0 {
		t.Fatalf("one bad signal must refuse the WHOLE batch — want 0 inputs, got %d", len(inputs.Items))
	}
}

// signal-severity: "Absent severity behaves as today" — no severity field at
// all is accepted and routes exactly as before the field existed: a
// conversation is created, and its provenance/opening input both omit
// Severity.
func TestSignalInboundAbsentSeverityBehavesAsToday(t *testing.T) {
	source := sigTestSource("sev-absent-src", "sev-absent-t")
	s, c := sigServer(t, source)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "sev-absent-1", "payload": "hello"},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	sigs := listSignals(t, c)
	if len(sigs) != 1 {
		t.Fatalf("want 1 Signal, got %d", len(sigs))
	}
	if sigs[0].Spec.Severity != "" {
		t.Fatalf("absent severity must leave the Signal's severity omitted, got %q", sigs[0].Spec.Severity)
	}
}

// signal-severity: "Each vocabulary value is accepted" — all four, stored on
// the Signal verbatim. The consumer threads it onto the conversation's
// provenance and the input (internal/signalroute's tests).
func TestSignalInboundValidSeverityIsStoredOnTheSignal(t *testing.T) {
	for _, sev := range []agentopsv1alpha1.Severity{
		agentopsv1alpha1.SeverityCritical, agentopsv1alpha1.SeverityError,
		agentopsv1alpha1.SeverityWarning, agentopsv1alpha1.SeverityInfo,
	} {
		t.Run(string(sev), func(t *testing.T) {
			source := sigTestSource("sev-valid-src-"+string(sev), "sev-valid-t")
			s, c := sigServer(t, source)
			rec := postSig(s, source.Name, []map[string]any{
				{"initiationId": "sev-valid-1", "payload": "hello", "severity": string(sev)},
			})
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			sigs := listSignals(t, c)
			if len(sigs) != 1 || sigs[0].Spec.Severity != sev {
				t.Fatalf("severity %q not stored on the Signal: %+v", sev, sigs)
			}
		})
	}
}

// listSignals returns every Signal in the test namespace.
func listSignals(t *testing.T, c client.Client) []agentopsv1alpha1.Signal {
	t.Helper()
	var list agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &list, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	return list.Items
}

// signalsource-attribute-filter: "A signal matching every requirement routes
// normally" — a filtered source still admits a signal that satisfies its
// attributeFilters, identically to no filter declared.
func TestSignalInboundAttributeFilterAdmitsMatchingSignal(t *testing.T) {
	source := sigTestSource("attrfilter-match-src", "attrfilter-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: agentopsv1alpha1.AttributeFilterIn, Values: []string{"payments"}},
	}
	pipeline := sigTestPipeline("attrfilter-match-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "attrfilter-match-1", "payload": "hello", "labels": map[string]string{"team": "payments"}},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 {
		t.Fatalf("a matching signal must route normally — want 1 Signal, got %d", len(convs.Items))
	}
}

// signalsource-attribute-filter: "A signal failing one requirement is
// dropped before cooldown" — the signal never reaches a serving pipeline,
// never consumes cooldown state, and never bumps receivedTotal.
func TestSignalInboundAttributeFilterDropsFailingSignalBeforeCooldown(t *testing.T) {
	source := sigTestSource("attrfilter-drop-src", "attrfilter-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: agentopsv1alpha1.AttributeFilterIn, Values: []string{"payments"}},
	}
	pipeline := sigTestPipeline("attrfilter-drop-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "attrfilter-drop-1", "payload": "hello", "labels": map[string]string{"team": "search"}},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["reason"] != ReasonFiltered {
		t.Fatalf("reason = %v, want %q", resp["reason"], ReasonFiltered)
	}

	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 0 {
		t.Fatalf("a filtered signal must never reach a serving pipeline — want 0 Signals, got %d", len(convs.Items))
	}

	var refreshed agentopsv1alpha1.SignalSource
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigTestNamespace, Name: source.Name}, &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Status.ReceivedTotal != 0 {
		t.Fatalf("a filtered signal must never bump receivedTotal, got %d", refreshed.Status.ReceivedTotal)
	}
	if len(refreshed.Status.Cooldown) != 0 {
		t.Fatalf("a filtered signal must never consume cooldown state, got %+v", refreshed.Status.Cooldown)
	}

	events, _ := s.Activity.Since("", 0)
	found := false
	for _, e := range events {
		if e.Kind == activity.KindSignalDropped && e.Code == activity.CodeFiltered {
			found = true
		}
	}
	if !found {
		t.Fatal("a filtered drop must be recorded as a signal.dropped event by default (logFiltered unset)")
	}
}

// signalsource-attribute-filter: "Empty or absent attributeFilters admits
// everything" — a source with the field explicitly set to an empty list, and
// one that never declared it at all (nil), both behave identically to no
// filter existing.
func TestSignalInboundNilOrEmptyAttributeFiltersAdmitsEverything(t *testing.T) {
	cases := []struct {
		name    string
		filters []agentopsv1alpha1.AttributeFilterRequirement
	}{
		{name: "nil", filters: nil},
		{name: "empty", filters: []agentopsv1alpha1.AttributeFilterRequirement{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := sigTestSource("attrfilter-"+tc.name+"-src", "attrfilter-t")
			source.Spec.AttributeFilters = tc.filters
			pipeline := sigTestPipeline("attrfilter-"+tc.name+"-pipe", source.Name)
			s, c := sigServer(t, source, pipeline)

			rec := postSig(s, source.Name, []map[string]any{
				{"initiationId": "attrfilter-" + tc.name + "-1", "payload": "hello"},
			})
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			var convs agentopsv1alpha1.SignalList
			if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
				t.Fatal(err)
			}
			if len(convs.Items) != 1 {
				t.Fatalf("%s filters must admit every signal — want 1 Signal, got %d", tc.name, len(convs.Items))
			}
		})
	}
}

// signalsource-attribute-filter: "A value-count-malformed requirement fails
// closed" — an In requirement with no Values passes the CRD's operator enum
// but is evaluated as never-satisfied, so the signal it gates is dropped
// rather than admitted.
func TestSignalInboundAttributeFilterFailsClosedOnMalformedRequirement(t *testing.T) {
	source := sigTestSource("attrfilter-malformed-src", "attrfilter-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: agentopsv1alpha1.AttributeFilterIn, Values: nil},
	}
	pipeline := sigTestPipeline("attrfilter-malformed-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "attrfilter-malformed-1", "payload": "hello", "labels": map[string]string{"team": "payments"}},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 0 {
		t.Fatalf("a malformed requirement must fail closed — want 0 Signals, got %d", len(convs.Items))
	}
}

// signalsource-attribute-filter: the batch response's "reason" is set
// whenever any signal in the batch was filtered out — even a PARTIALLY
// filtered batch, where the admitted remainder still routes — and left
// unset on a batch nothing was filtered from, matching the convention
// ReasonAtCapacity already follows.
func TestSignalInboundAttributeFilterReasonSetOnPartialBatchUnsetOnFullAdmit(t *testing.T) {
	source := sigTestSource("attrfilter-partial-src", "attrfilter-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: agentopsv1alpha1.AttributeFilterIn, Values: []string{"payments"}},
	}
	pipeline := sigTestPipeline("attrfilter-partial-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "attrfilter-partial-ok", "payload": "fine", "labels": map[string]string{"team": "payments"}},
		{"initiationId": "attrfilter-partial-bad", "payload": "boom", "labels": map[string]string{"team": "search"}},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["reason"] != ReasonFiltered {
		t.Fatalf("a partially-filtered batch must report reason=%q, got %v", ReasonFiltered, resp["reason"])
	}
	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 {
		t.Fatalf("the admitted remainder must still route — want 1 Signal, got %d", len(convs.Items))
	}

	// A fully-admitted batch on a DIFFERENT source — same filter, nothing
	// fails it — must leave "reason" unset entirely.
	cleanSource := sigTestSource("attrfilter-clean-src", "attrfilter-t")
	cleanSource.Spec.AttributeFilters = source.Spec.AttributeFilters
	cleanPipeline := sigTestPipeline("attrfilter-clean-pipe", cleanSource.Name)
	s2, _ := sigServer(t, cleanSource, cleanPipeline)
	rec2 := postSig(s2, cleanSource.Name, []map[string]any{
		{"initiationId": "attrfilter-clean-1", "payload": "fine", "labels": map[string]string{"team": "payments"}},
	})
	if rec2.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec2.Code, rec2.Body.String())
	}
	var resp2 map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp2["reason"]; ok {
		t.Fatalf("a fully-admitted batch must leave reason unset, got %v", resp2["reason"])
	}
}

// signalsource-attribute-filter: "logFiltered false suppresses only the
// filtered-drop record" — the signal is still dropped, but no signal.dropped
// event is recorded for it.
func TestSignalInboundLogFilteredFalseSuppressesFilteredDropEvent(t *testing.T) {
	source := sigTestSource("attrfilter-silent-src", "attrfilter-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: agentopsv1alpha1.AttributeFilterIn, Values: []string{"payments"}},
	}
	source.Spec.LogFiltered = falsePtr()
	pipeline := sigTestPipeline("attrfilter-silent-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "attrfilter-silent-1", "payload": "hello", "labels": map[string]string{"team": "search"}},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 0 {
		t.Fatalf("the signal must still be dropped — want 0 Signals, got %d", len(convs.Items))
	}

	events, _ := s.Activity.Since("", 0)
	for _, e := range events {
		if e.Kind == activity.KindSignalDropped && e.Code == activity.CodeFiltered {
			t.Fatalf("logFiltered:false must suppress the filtered-drop event, got %+v", e)
		}
	}
}

// signalsource-attribute-filter: "Other drop reasons still log regardless of
// logFiltered" — logFiltered:false gates only CodeFiltered, so the duplicate
// record of a re-sent message is still emitted.
func TestSignalInboundLogFilteredFalseDoesNotSuppressOtherDropCodes(t *testing.T) {
	source := sigTestSource("attrfilter-dup-src", "attrfilter-t")
	source.Spec.LogFiltered = falsePtr()
	s, _ := sigServer(t, source)

	for i := 0; i < 2; i++ {
		rec := postSig(s, source.Name, []map[string]any{
			{"initiationId": "attrfilter-dup-1", "signalId": "m-1", "payload": "hello"},
		})
		if rec.Code != 200 {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
	}

	events, _ := s.Activity.Since("", 0)
	found := false
	for _, e := range events {
		if e.Kind == activity.KindSignalDropped && e.Code == activity.CodeDuplicate {
			found = true
		}
	}
	if !found {
		t.Fatal("logFiltered must not suppress the duplicate event")
	}
}

// signalsource-attribute-filter: "A signal matching every requirement routes
// normally" / "A signal failing one requirement is dropped" — exercised here
// through NotIn/Exists/DoesNotExist, not just In (the only operator the
// other httpapi tests above use). ingest.Admits already pins each operator's
// semantics at the unit level; this confirms the handler actually wires
// AttributeFilters through for every operator, not just the one it happens
// to be tested with elsewhere.
func TestSignalInboundAttributeFilterOperatorsThroughHTTPAPI(t *testing.T) {
	cases := []struct {
		name        string
		requirement agentopsv1alpha1.AttributeFilterRequirement
		admitLabels map[string]string
		dropLabels  map[string]string
	}{
		{
			name: "NotIn",
			requirement: agentopsv1alpha1.AttributeFilterRequirement{
				Key: "team", Operator: agentopsv1alpha1.AttributeFilterNotIn, Values: []string{"blocked"},
			},
			admitLabels: map[string]string{"team": "payments"},
			dropLabels:  map[string]string{"team": "blocked"},
		},
		{
			name: "Exists",
			requirement: agentopsv1alpha1.AttributeFilterRequirement{
				Key: "team", Operator: agentopsv1alpha1.AttributeFilterExists,
			},
			admitLabels: map[string]string{"team": "payments"},
			dropLabels:  map[string]string{"other": "x"},
		},
		{
			name: "DoesNotExist",
			requirement: agentopsv1alpha1.AttributeFilterRequirement{
				Key: "team", Operator: agentopsv1alpha1.AttributeFilterDoesNotExist,
			},
			admitLabels: map[string]string{"other": "x"},
			dropLabels:  map[string]string{"team": "payments"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkAttributeFilterOperator(t, tc.name, tc.requirement, tc.admitLabels, tc.dropLabels)
		})
	}
}

func checkAttributeFilterOperator(t *testing.T, name string, requirement agentopsv1alpha1.AttributeFilterRequirement, admitLabels, dropLabels map[string]string) {
	t.Helper()
	suffix := strings.ToLower(name)
	source := sigTestSource("attrfilter-op-"+suffix+"-src", "attrfilter-op-"+suffix+"-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{requirement}
	pipeline := sigTestPipeline("attrfilter-op-"+suffix+"-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "op-admit-1", "payload": "fine", "labels": admitLabels},
		{"initiationId": "op-drop-1", "payload": "boom", "labels": dropLabels},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["reason"] != ReasonFiltered {
		t.Fatalf("operator %s: reason = %v, want %q", name, resp["reason"], ReasonFiltered)
	}

	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 {
		t.Fatalf("operator %s: want exactly 1 admitted Signal, got %d", name, len(convs.Items))
	}
	if convs.Items[0].Spec.InitiationID != "op-admit-1" {
		t.Fatalf("operator %s: the routed conversation must be the ADMITTED signal, got conversationId %q",
			name, convs.Items[0].Spec.InitiationID)
	}
}

// signalsource-attribute-filter: "A signal failing one requirement is
// dropped before cooldown" — with MULTIPLE requirements, exercised through
// the httpapi layer rather than only ingest.Admits, to confirm the AND
// across requirements survives the conversion into the handler's own
// []ingest.FilterRequirement.
func TestSignalInboundAttributeFilterMultipleRequirementsRequiresAll(t *testing.T) {
	source := sigTestSource("attrfilter-and-src", "attrfilter-and-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: agentopsv1alpha1.AttributeFilterIn, Values: []string{"payments"}},
		{Key: "env", Operator: agentopsv1alpha1.AttributeFilterIn, Values: []string{"prod"}},
	}
	pipeline := sigTestPipeline("attrfilter-and-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		// Satisfies BOTH requirements: must route.
		{"initiationId": "and-both-1", "payload": "fine", "labels": map[string]string{"team": "payments", "env": "prod"}},
		// Satisfies only the FIRST requirement — the second's failure must
		// still drop it, not be forgiven by the first one passing.
		{"initiationId": "and-one-1", "payload": "boom", "labels": map[string]string{"team": "payments", "env": "staging"}},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 {
		t.Fatalf("a signal failing only the SECOND requirement must still be dropped — want 1 Signal, got %d", len(convs.Items))
	}
	if convs.Items[0].Spec.InitiationID != "and-both-1" {
		t.Fatalf("the routed conversation must be the one satisfying BOTH requirements, got conversationId %q",
			convs.Items[0].Spec.InitiationID)
	}
}

// signalsource-attribute-filter: design.md's "filtering happens once ...
// ahead of the lane split" — a chat-kind signal failing attributeFilters is
// dropped exactly like an alert/job/task signal, never reaching
// routeChatSignals, never bumping receivedTotal.
func TestSignalInboundAttributeFilterDropsChatKindSignal(t *testing.T) {
	source := sigTestSource("attrfilter-chat-src", "attrfilter-chat-t")
	source.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: agentopsv1alpha1.AttributeFilterIn, Values: []string{"payments"}},
	}
	// A Pipeline claims the source so the chat lane, if ever wrongly reached,
	// would have exactly one server to route to (never ambiguous) — isolating
	// the assertion to "filtering happened before the chat split", not
	// "nothing claims this chat source".
	pipeline := sigTestPipeline("attrfilter-chat-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{
			"initiationId": "attrfilter-chat-1", "kind": KindChat, "payload": "hello",
			// chatChannel is never resolved — the signal never reaches
			// routeChatSignals — so an unresolvable channel name is fine here.
			"labels": map[string]string{"team": "search", LabelChatChannel: "nonexistent-channel"},
		},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["reason"] != ReasonFiltered {
		t.Fatalf("a filtered chat signal must report reason=%q, got %v", ReasonFiltered, resp["reason"])
	}

	var convs agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 0 {
		t.Fatalf("a filtered chat signal must never open a Signal — want 0, got %d", len(convs.Items))
	}

	var refreshed agentopsv1alpha1.SignalSource
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigTestNamespace, Name: source.Name}, &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Status.ReceivedTotal != 0 {
		t.Fatalf("a filtered chat signal must never bump receivedTotal, got %d", refreshed.Status.ReceivedTotal)
	}

	events, _ := s.Activity.Since("", 0)
	found := false
	for _, e := range events {
		if e.Kind == activity.KindSignalDropped && e.Code == activity.CodeFiltered {
			found = true
		}
	}
	if !found {
		t.Fatal("a filtered chat signal must still be recorded as a signal.dropped event (logFiltered unset)")
	}
}
