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
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// sigTestNamespace is the namespace every fixture in this file shares.
const sigTestNamespace = "agent-ops"

// sigServer builds a Server wired for /signal/inbound: a fake client seeded
// with the given objects, indexed on spec.conversationId exactly as the real
// manager's cache is (routeSignalGroup's equality routing depends on it).
func sigServer(t *testing.T, objs ...client.Object) (*Server, client.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithIndex(&agentopsv1alpha1.Conversation{}, agentopsv1alpha1.IndexConversationID, controller.ConversationIDIndexer).
		WithObjects(objs...).Build()
	return &Server{Reader: c, Client: c, Namespace: sigTestNamespace, AdapterToken: "sig-test-master"}, c
}

// sigTestSource is a minimal SignalSource fixture.
func sigTestSource(name, adapter string) *agentopsv1alpha1.SignalSource {
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, sigTestNamespace
	src.Spec.Adapter = adapter
	return src
}

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

	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 0 {
		t.Fatalf("one bad signal must refuse the WHOLE batch — want 0 conversations, got %d", len(convs.Items))
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
	pipeline := sigTestPipeline("sev-absent-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "sev-absent-1", "payload": "hello"},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs.Items))
	}
	conv := convs.Items[0]
	if conv.Spec.Signal == nil || conv.Spec.Signal.Severity != "" {
		t.Fatalf("absent severity must leave provenance.severity omitted, got %+v", conv.Spec.Signal)
	}
	if len(conv.Spec.Inputs) != 1 || conv.Spec.Inputs[0].PayloadRef == nil {
		t.Fatalf("opening input missing: %+v", conv.Spec.Inputs)
	}
	var ci agentopsv1alpha1.ConversationInput
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: sigTestNamespace, Name: conv.Spec.Inputs[0].PayloadRef.Name}, &ci); err != nil {
		t.Fatal(err)
	}
	if ci.Spec.Severity != "" {
		t.Fatalf("absent severity must leave the input's severity omitted, got %q", ci.Spec.Severity)
	}
}

// signal-severity: "Each vocabulary value is accepted", threaded onto
// SignalProvenance and the opening ConversationInputSpec exactly where
// Labels already lands.
// signal-severity: "Each vocabulary value is accepted" — all four, not just
// the one this test used to exercise (critical). info was previously only
// ever named in a 400 rejection's body, never posted and stored as a valid
// value in its own right.
func TestSignalInboundValidSeverityThreadsOntoProvenanceAndInput(t *testing.T) {
	for _, sev := range []agentopsv1alpha1.Severity{
		agentopsv1alpha1.SeverityCritical, agentopsv1alpha1.SeverityError,
		agentopsv1alpha1.SeverityWarning, agentopsv1alpha1.SeverityInfo,
	} {
		t.Run(string(sev), func(t *testing.T) {
			source := sigTestSource("sev-valid-src-"+string(sev), "sev-valid-t")
			pipeline := sigTestPipeline("sev-valid-pipe-"+string(sev), source.Name)
			s, c := sigServer(t, source, pipeline)

			rec := postSig(s, source.Name, []map[string]any{
				{"initiationId": "sev-valid-1", "payload": "hello", "severity": string(sev)},
			})
			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}

			var convs agentopsv1alpha1.ConversationList
			if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
				t.Fatal(err)
			}
			if len(convs.Items) != 1 {
				t.Fatalf("want 1 conversation, got %d", len(convs.Items))
			}
			conv := convs.Items[0]
			if conv.Spec.Signal == nil || conv.Spec.Signal.Severity != sev {
				t.Fatalf("provenance severity wrong: %+v", conv.Spec.Signal)
			}
			var ci agentopsv1alpha1.ConversationInput
			if err := c.Get(context.Background(), types.NamespacedName{
				Namespace: sigTestNamespace, Name: conv.Spec.Inputs[0].PayloadRef.Name}, &ci); err != nil {
				t.Fatal(err)
			}
			if ci.Spec.Severity != sev {
				t.Fatalf("input severity wrong: %q", ci.Spec.Severity)
			}
		})
	}
}

// signal-severity: "Provenance keeps the opening severity" / "A recurrence
// records its own severity" — a conversation opened by a `warning` signal
// keeps `warning` on its provenance after a later `error` recurrence lands,
// while the NEW input records `error`.
func TestSignalInboundRecurrenceNeverRewritesProvenanceSeverity(t *testing.T) {
	source := sigTestSource("sev-recur-src", "sev-recur-t")
	pipeline := sigTestPipeline("sev-recur-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "sev-recur-1", "payload": "first", "severity": "warning"},
	})
	if rec.Code != 200 {
		t.Fatalf("opening signal: %d %s", rec.Code, rec.Body.String())
	}
	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs.Items))
	}
	conv := &convs.Items[0]

	// A session must exist for the SAME initiationId to land as a recurrence
	// rather than reopen the conversation, exactly as
	// assertRecurrenceIntoSameConversation does in internal/integration.
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.SessionID = "sess-sev-recur"
	if err := c.Status().Patch(context.Background(), conv, patch); err != nil {
		t.Fatal(err)
	}

	// Cooldown is in-process (s.cooldowns), so a second Server stands in for
	// "outside the source's cooldown window" without waiting one out — same
	// device TestOpenConversationWithMatchingConversationIDGetsRecurrence
	// uses.
	s2 := &Server{Reader: c, Client: c, Namespace: sigTestNamespace, AdapterToken: s.AdapterToken}
	rec = postSig(s2, source.Name, []map[string]any{
		{"initiationId": "sev-recur-1", "payload": "second", "severity": "error"},
	})
	if rec.Code != 200 {
		t.Fatalf("recurrence signal: %d %s", rec.Code, rec.Body.String())
	}

	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigTestNamespace, Name: conv.Name}, conv); err != nil {
		t.Fatal(err)
	}
	if conv.Spec.Signal == nil || conv.Spec.Signal.Severity != agentopsv1alpha1.SeverityWarning {
		t.Fatalf("provenance must keep the OPENING severity (warning), got %+v", conv.Spec.Signal)
	}
	if len(conv.Spec.Inputs) != 2 || conv.Spec.Inputs[1].Type != agentopsv1alpha1.InputRecurrence {
		t.Fatalf("recurrence input expected: %+v", conv.Spec.Inputs)
	}
	var recurCI agentopsv1alpha1.ConversationInput
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: sigTestNamespace, Name: conv.Spec.Inputs[1].PayloadRef.Name}, &recurCI); err != nil {
		t.Fatal(err)
	}
	if recurCI.Spec.Severity != agentopsv1alpha1.SeverityError {
		t.Fatalf("the recurrence's OWN input must record its own severity (error), got %q", recurCI.Spec.Severity)
	}
}
