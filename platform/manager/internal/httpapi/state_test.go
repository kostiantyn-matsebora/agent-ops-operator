package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

func stateTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := agentopsv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// testConversations, testRuns, testBlobs and testSignals are the kube backend
// over a test's fake client, one per Stores field a Server test needs — the
// same mapping the manager runs, with no envtest.
func testConversations(t *testing.T, c client.WithWatch, ns string) state.Collection[state.ConversationRecord] {
	t.Helper()
	coll, err := kube.Open(c, ns, kube.ConversationLayout())
	if err != nil {
		t.Fatal(err)
	}
	return coll
}

func testRuns(t *testing.T, c client.WithWatch, ns string) state.Appended[state.ConversationRecord, state.Run] {
	t.Helper()
	runs, err := kube.OpenAppended(c, ns, kube.RunsLayout())
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

func testBlobs(c client.WithWatch, ns string) state.Blobs { return kube.OpenBlobs(c, ns) }

func testSignals(t *testing.T, c client.WithWatch, ns string) state.Expiring[state.SignalRecord] {
	t.Helper()
	signals, err := kube.OpenExpiring(c, ns, kube.SignalLayout())
	if err != nil {
		t.Fatal(err)
	}
	return signals
}

func testAdapterState(t *testing.T, c client.WithWatch, ns string) state.Collection[string] {
	t.Helper()
	coll, err := kube.OpenAdapterState(c, ns)
	if err != nil {
		t.Fatal(err)
	}
	return coll
}

// Malformed JSON bodies are refused the same way here as everywhere else --
// but both handlers resolve the channel FIRST, so covering them needs a
// real (fake) Reader, not the zero-value Server the other two call sites
// take.
func TestStateHandlersRefuseAMalformedBodyOnceTheChannelResolves(t *testing.T) {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace, ch.Spec.Adapter = "c1", "agent-ops", "slack"
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(ch).Build()
	s := &Server{Reader: c, Conversations: testConversations(t, c, "agent-ops"), Namespace: "agent-ops"}

	t.Run("handleStatePut", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/channel/c1/state/key", strings.NewReader("{not json"))
		req.SetPathValue("channel", "c1")
		req.SetPathValue("key", "key")
		rec := httptest.NewRecorder()
		s.handleStatePut(rec, req)
		if rec.Code != 400 {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), errInvalidJSON) {
			t.Fatalf("body = %q, want it to name the parse failure", rec.Body.String())
		}
	})

	t.Run("handleChannelStatus", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/channel/c1/status", strings.NewReader("{not json"))
		req.SetPathValue("name", "c1")
		rec := httptest.NewRecorder()
		s.handleChannelStatus(rec, req)
		if rec.Code != 400 {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), errInvalidJSON) {
			t.Fatalf("body = %q, want it to name the parse failure", rec.Body.String())
		}
	})
}
