package integration

import (
	"encoding/json"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

// The vocabulary exists because a channel adapter holds NO Kubernetes access:
// it cannot read a Pipeline and never will, so the manager is the only thing
// that can tell a surface what is addressable.

func getVocabulary(t *testing.T, srv *httpapi.Server) chat.Vocabulary {
	t.Helper()
	rec := adapterReq(srv, "GET", "/channel/vocabulary", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("vocabulary: %d %s", rec.Code, rec.Body.String())
	}
	var v chat.Vocabulary
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func opsPoll(t *testing.T, srv *httpapi.Server) (int, string) {
	t.Helper()
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	return rec.Code, rec.Header().Get(httpapi.VocabularyRevisionHeader)
}

// The endpoint is reachable with the adapter token the contract already
// defines, and refused without one — no new grant, no new credential.
func TestVocabularyServedToAnAuthenticatedAdapter(t *testing.T) {
	srv := apiServer()
	if rec := adapterReq(srv, "GET", "/channel/vocabulary", nil, ""); rec.Code != 401 {
		t.Fatalf("unauthenticated vocabulary: %d, want 401", rec.Code)
	}
	v := getVocabulary(t, srv)
	if v.Revision == "" {
		t.Fatal("vocabulary carries no revision")
	}
	var builtins []string
	for _, e := range v.Entries {
		if e.Kind == chat.KindBuiltin {
			builtins = append(builtins, e.Name)
		}
	}
	for _, want := range []string{"pipelines", "help", "exit", "close"} {
		if !hasName(builtins, want) {
			t.Fatalf("builtin %q missing from %v", want, builtins)
		}
	}
	if hasName(builtins, "agents") {
		t.Fatalf("retired listing name published: %v", builtins)
	}
}

// The long-poll carries the revision on BOTH outcomes. That is what reaches an
// otherwise idle adapter without the manager dialing it — which it cannot do,
// since an adapter need not be addressable at all.
func TestOpsPollCarriesTheRevisionOnBothOutcomes(t *testing.T) {
	mkChannel(t, "vocab-chan", "telegram")
	srv := apiServer()

	code, rev := opsPoll(t, srv)
	if code != 204 {
		t.Fatalf("expected an empty poll, got %d", code)
	}
	if rev == "" {
		t.Fatal("empty poll carried no revision — an idle adapter never learns")
	}

	// The 204 must stay bodyless: giving it a body would change what every
	// adapter built before the vocabulary parses.
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Body.Len() != 0 {
		t.Fatalf("204 grew a body: %q", rec.Body.String())
	}

	// And on a delivered op.
	mkChatSource(t, "vocab-src", "vocab-chan")
	mkProfile(t, "vocab-profile")
	mkPipeline(t, "vocab-pipe", []string{"vocab-src"}, []string{"vocab-chan"}, "vocab-profile")
	reconcilePipeline(t, "vocab-pipe")
	if rec := chatSignal(t, srv, "vocab-src", "vocab-chan", "/"+chat.ListCommand); rec.Code != 200 {
		t.Fatalf("listing command: %d %s", rec.Code, rec.Body.String())
	}
	code, rev = opsPoll(t, srv)
	if code != 200 {
		t.Fatalf("expected a delivered op, got %d", code)
	}
	if rev == "" {
		t.Fatal("delivered op carried no revision")
	}
}

// A Pipeline becoming addressable changes the revision an adapter observes,
// with nothing pushed at it.
func TestReadyPipelineChangesTheObservedRevision(t *testing.T) {
	mkChannel(t, "rev-chan", "telegram")
	srv := apiServer()
	_, before := opsPoll(t, srv)

	mkChatSource(t, "rev-src", "rev-chan")
	mkProfile(t, "rev-profile")
	mkPipeline(t, "rev-pipe", []string{"rev-src"}, []string{"rev-chan"}, "rev-profile")
	reconcilePipeline(t, "rev-pipe")

	_, after := opsPoll(t, srv)
	if before == after {
		t.Fatalf("revision unchanged after a pipeline became Ready (%s)", before)
	}
	v := getVocabulary(t, srv)
	if v.Revision != after {
		t.Fatalf("header %q disagrees with the endpoint %q", after, v.Revision)
	}
	var found bool
	for _, e := range v.Entries {
		if e.Kind == chat.KindPipeline && e.Name == "rev-pipe" {
			found = true
			if e.Profile != "rev-profile" {
				t.Fatalf("profile not derived: %+v", e)
			}
		}
	}
	if !found {
		t.Fatalf("ready pipeline absent from the vocabulary: %+v", v.Entries)
	}
}

// An adapter that knows nothing of the vocabulary keeps working, and the
// outbound contract version is unchanged — every addition here is optional.
func TestVocabularyIsPurelyAdditiveForOlderAdapters(t *testing.T) {
	mkChannel(t, "old-chan", "telegram")
	srv := apiServer()
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 204 {
		t.Fatalf("older adapter refused: %d %s", rec.Code, rec.Body.String())
	}
	if chat.ContractVersion != "2" {
		t.Fatalf("contract version moved to %q — nothing here required it", chat.ContractVersion)
	}
	if rec := adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=1&wait=0", nil, "test-adapter-token"); rec.Code != 400 {
		t.Fatalf("contract handshake weakened: %d", rec.Code)
	}
}

func hasName(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
