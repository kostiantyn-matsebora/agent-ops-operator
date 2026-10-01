package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// erroringReader always fails to read, for exercising readJSON's error path —
// an httptest body backed by a string or bytes.Reader never errors, so this is
// the only way to reach it.
type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

const coordTestMasterKey = "master-key"

// coordServer builds a Server wired for /coordinate/*: a fake client seeded
// with the given objects, a Router sharing the same client, and the master
// key every derived token in these tests is checked against.
func coordServer(t *testing.T, objs ...client.Object) (*Server, client.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(objs...).Build()
	q := &chat.OpQueue{Client: c, Namespace: "agent-ops", Registry: chat.NewRegistry()}
	router := &chat.Router{Client: c, Reader: c, Namespace: "agent-ops", Ops: q}
	return &Server{Reader: c, Client: c, Namespace: "agent-ops", AdapterToken: coordTestMasterKey, Router: router}, c
}

func postCoordinate(s *Server, path, token string, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func coordCoordinator(name string, agents ...agentopsv1alpha1.CoordinatorAgentEntry) *agentopsv1alpha1.Coordinator {
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = name, "agent-ops"
	co.Spec.Agents = agents
	return co
}

func coordRoot(name, coordinator string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name, c.Namespace = name, "agent-ops"
	c.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: coordinator}
	return c
}

func TestHandleCoordinateInvokeRefusesTheWrongToken(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	rec := postCoordinate(s, "/coordinate/invoke", "not-the-right-token",
		map[string]any{"conversation": "root-1", "agent": "worker", "task": "do it"})
	if rec.Code != 401 {
		t.Fatalf("want 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateInvokeSucceedsWithTheDerivedToken(t *testing.T) {
	capability := &agentopsv1alpha1.AgentCapability{}
	capability.Name, capability.Namespace = "cap-worker", "agent-ops"
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Name, profile.Namespace = "profile-worker", "agent-ops"
	capability.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: profile.Name}
	co := coordCoordinator("co-a", agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "worker", Description: "does it", CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-worker"},
	})
	root := coordRoot("root-1", "co-a")
	s, c := coordServer(t, co, root, capability, profile)

	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")
	rec := postCoordinate(s, "/coordinate/invoke", token,
		map[string]any{"conversation": "root-1", "agent": "worker", "task": "do it"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "created" || out["member"] == "" {
		t.Fatalf("want a created member, got %+v", out)
	}
	var member agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: out["member"]}, &member); err != nil {
		t.Fatal(err)
	}
	if member.Spec.CausedBy == nil || member.Spec.CausedBy.Parent != "root-1" {
		t.Fatalf("the created member must be caused by the caller, got %+v", member.Spec.CausedBy)
	}
}

func TestHandleCoordinateCloseEnforcesOneHopReach(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", "agent-ops"
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "worker"}
	grandchild := &agentopsv1alpha1.Conversation{}
	grandchild.Name, grandchild.Namespace = "grandchild-1", "agent-ops"
	grandchild.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "member-1", Entry: "helper"}
	s, c := coordServer(t, coordCoordinator("co-a"), root, member, grandchild)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	// The direct member: allowed.
	rec := postCoordinate(s, "/coordinate/close", token,
		map[string]any{"conversation": "root-1", "target": "member-1", "reason": "done"})
	if rec.Code != 200 {
		t.Fatalf("closing a direct member must succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	var gotMember agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "member-1"}, &gotMember)
	if gotMember.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatal("the direct member must be closed")
	}

	// The grandchild, reached directly from the root: refused.
	root2 := coordRoot("root-2", "co-a")
	memberOf2 := &agentopsv1alpha1.Conversation{}
	memberOf2.Name, memberOf2.Namespace = "member-2", "agent-ops"
	memberOf2.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-2", Entry: "worker"}
	grandOf2 := &agentopsv1alpha1.Conversation{}
	grandOf2.Name, grandOf2.Namespace = "grandchild-2", "agent-ops"
	grandOf2.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "member-2", Entry: "helper"}
	s2, c2 := coordServer(t, coordCoordinator("co-a"), root2, memberOf2, grandOf2)
	token2 := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-2")
	rec2 := postCoordinate(s2, "/coordinate/close", token2,
		map[string]any{"conversation": "root-2", "target": "grandchild-2", "reason": "trying to reach too far"})
	if rec2.Code != 403 {
		t.Fatalf("closing a grandchild directly must be refused, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var gotGrand agentopsv1alpha1.Conversation
	c2.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "grandchild-2"}, &gotGrand)
	if gotGrand.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("an out-of-scope close must change nothing")
	}
}

func TestHandleCoordinateEscalateOpensAHumanThreadOnTheUncausedRoot(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	root.Spec.EscalationChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "ops-desk"}}
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = "ops-desk", "agent-ops"
	ch.Spec.Adapter = "telegram"
	s, c := coordServer(t, coordCoordinator("co-a"), root, ch)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/escalate", token,
		map[string]any{"conversation": "root-1", "message": "3 members failed"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &got)
	if got.Status.EscalatedAt == nil || got.Status.EscalationMessage != "3 members failed" {
		t.Fatalf("escalate must stamp the digest, got %+v", got.Status)
	}
	if len(got.Spec.ChannelRefs) != 1 || got.Spec.ChannelRefs[0].Name != "ops-desk" {
		t.Fatalf("escalate must bind the snapshotted channels, got %v", got.Spec.ChannelRefs)
	}
}

func TestHandleCoordinateReadChannelReaderProjection(t *testing.T) {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = "voice-desk", "agent-ops"
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "conv-1", "agent-ops"
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "voice-desk"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "voice-desk", ThreadID: "t1"}}
	conv.Status.Brief = "discussing the outage"
	other := &agentopsv1alpha1.Conversation{}
	other.Name, other.Namespace = "conv-2", "agent-ops"
	s, _ := coordServer(t, ch, conv, other)

	token := chat.DeriveChannelReaderToken(coordTestMasterKey, "voice-desk")
	rec := postCoordinate(s, "/coordinate/read", token, map[string]any{"channel": "voice-desk"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Conversations []conversationProjection `json:"conversations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Conversations) != 1 || out.Conversations[0].Name != "conv-1" || out.Conversations[0].Brief != "discussing the outage" {
		t.Fatalf("must list only conversations with a thread on this channel, got %+v", out.Conversations)
	}
}

func TestHandleCoordinateReadRefusesOutOfScope(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	unrelated := &agentopsv1alpha1.Conversation{}
	unrelated.Name, unrelated.Namespace = "unrelated-1", "agent-ops"
	s, _ := coordServer(t, coordCoordinator("co-a"), root, unrelated)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/read", token,
		map[string]any{"conversation": "root-1", "target": "unrelated-1"})
	if rec.Code != 403 {
		t.Fatalf("reading outside the caller's own subtree must be refused, got %d: %s", rec.Code, rec.Body.String())
	}
}

// descendsFrom walks UP the causedBy chain, so a target more than one hop
// below the caller must still resolve — the loop's "fetch the parent and
// keep walking" branch, which a direct member or an unrelated conversation
// (both one comparison away from an answer) never exercises.
func TestHandleCoordinateReadReachesADescendantAtAnyDepth(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", "agent-ops"
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "worker"}
	grandchild := &agentopsv1alpha1.Conversation{}
	grandchild.Name, grandchild.Namespace = "grandchild-1", "agent-ops"
	grandchild.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "member-1", Entry: "helper"}
	s, _ := coordServer(t, coordCoordinator("co-a"), root, member, grandchild)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/read", token,
		map[string]any{"conversation": "root-1", "target": "member-1"})
	if rec.Code != 200 {
		t.Fatalf("a direct member is in scope, got %d: %s", rec.Code, rec.Body.String())
	}

	rec2 := postCoordinate(s, "/coordinate/read", token,
		map[string]any{"conversation": "root-1", "target": "grandchild-1"})
	if rec2.Code != 200 {
		t.Fatalf("the caller's own subtree reaches any depth, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var out conversationProjection
	if err := json.Unmarshal(rec2.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "grandchild-1" {
		t.Fatalf("want the grandchild's own projection, got %+v", out)
	}
}

func TestReadJSONPropagatesAReadError(t *testing.T) {
	req := httptest.NewRequest("POST", "/coordinate/invoke", erroringReader{})
	var v map[string]any
	if err := readJSON(req, &v); err == nil {
		t.Fatal("want an error when the body cannot be read")
	}
}

func TestStatusForDefaultsTo500ForAnUnrecognizedError(t *testing.T) {
	if got := statusFor(errors.New("something unexpected")); got != 500 {
		t.Fatalf("want 500 for an unrecognized error, got %d", got)
	}
}

func TestCallerConversationRefusesEmptyInputs(t *testing.T) {
	s, _ := coordServer(t)
	if _, err := s.callerConversation(context.Background(), "", "sometoken"); err != errWrongToken {
		t.Fatalf("want errWrongToken for an empty name, got %v", err)
	}
	if _, err := s.callerConversation(context.Background(), "root-1", ""); err != errWrongToken {
		t.Fatalf("want errWrongToken for an empty token, got %v", err)
	}
}

func TestCallerConversationRefusesAnUnknownConversation(t *testing.T) {
	s, _ := coordServer(t)
	if _, err := s.callerConversation(context.Background(), "does-not-exist", "sometoken"); err != errWrongToken {
		t.Fatalf("want errWrongToken for an unknown conversation, got %v", err)
	}
}

func TestCallerConversationRefusesANonCoordinatorConversation(t *testing.T) {
	plain := &agentopsv1alpha1.Conversation{}
	plain.Name, plain.Namespace = "plain-1", "agent-ops"
	s, _ := coordServer(t, plain)
	if _, err := s.callerConversation(context.Background(), "plain-1", "sometoken"); err != errWrongToken {
		t.Fatalf("want errWrongToken for a conversation with no coordinatorRef, got %v", err)
	}
}

func TestCallerChannelRefusesEmptyInputs(t *testing.T) {
	s, _ := coordServer(t)
	if _, err := s.callerChannel(context.Background(), "", "sometoken"); err != errWrongToken {
		t.Fatalf("want errWrongToken for an empty name, got %v", err)
	}
}

func TestCallerChannelRefusesAnUnknownChannel(t *testing.T) {
	s, _ := coordServer(t)
	if _, err := s.callerChannel(context.Background(), "does-not-exist", "sometoken"); err != errWrongToken {
		t.Fatalf("want errWrongToken for an unknown channel, got %v", err)
	}
}

func TestCallerChannelRefusesAWrongToken(t *testing.T) {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = "voice-desk", "agent-ops"
	s, _ := coordServer(t, ch)
	if _, err := s.callerChannel(context.Background(), "voice-desk", "wrong"); err != errWrongToken {
		t.Fatalf("want errWrongToken for a wrong token, got %v", err)
	}
}

func TestHandleCoordinateInvokeRejectsMissingFields(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")
	rec := postCoordinate(s, "/coordinate/invoke", token,
		map[string]any{"conversation": "root-1", "agent": "worker"})
	if rec.Code != 400 {
		t.Fatalf("want 400 for a missing task, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateInvokePropagatesAnUnknownAgentRefusal(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")
	rec := postCoordinate(s, "/coordinate/invoke", token,
		map[string]any{"conversation": "root-1", "agent": "nope", "task": "do it"})
	if rec.Code != 403 {
		t.Fatalf("want 403 for an unknown agents[] entry, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateInvokeReports500ForAnUnrecognizedRouterError(t *testing.T) {
	root := coordRoot("root-1", "co-missing") // names a Coordinator that does not exist
	s, _ := coordServer(t, root)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-missing", "root-1")
	rec := postCoordinate(s, "/coordinate/invoke", token,
		map[string]any{"conversation": "root-1", "agent": "worker", "task": "do it"})
	if rec.Code != 500 {
		t.Fatalf("want 500 for an unrecognized router error, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateInvokeReportsAttachedOnReuse(t *testing.T) {
	capability := &agentopsv1alpha1.AgentCapability{}
	capability.Name, capability.Namespace = "cap-worker", "agent-ops"
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Name, profile.Namespace = "profile-worker", "agent-ops"
	capability.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: profile.Name}
	co := coordCoordinator("co-a", agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "worker", Description: "does it", CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-worker"},
	})
	root := coordRoot("root-1", "co-a")
	s, _ := coordServer(t, co, root, capability, profile)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	first := postCoordinate(s, "/coordinate/invoke", token,
		map[string]any{"conversation": "root-1", "agent": "worker", "task": "first"})
	if first.Code != 200 {
		t.Fatalf("first invoke: want 200, got %d: %s", first.Code, first.Body.String())
	}
	second := postCoordinate(s, "/coordinate/invoke", token,
		map[string]any{"conversation": "root-1", "agent": "worker", "task": "second"})
	if second.Code != 200 {
		t.Fatalf("second invoke: want 200, got %d: %s", second.Code, second.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(second.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "attached" {
		t.Fatalf("a second invoke of the same entry must attach, got %+v", out)
	}
}

func TestHandleCoordinateCloseRejectsMissingReason(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")
	rec := postCoordinate(s, "/coordinate/close", token, map[string]any{"conversation": "root-1"})
	if rec.Code != 400 {
		t.Fatalf("want 400 for a missing reason, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateCloseRefusesAWrongToken(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	rec := postCoordinate(s, "/coordinate/close", "wrong-token",
		map[string]any{"conversation": "root-1", "reason": "done"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a wrong token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateCloseDefaultsTargetToTheCaller(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	s, c := coordServer(t, coordCoordinator("co-a"), root)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/close", token, map[string]any{"conversation": "root-1", "reason": "done"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatal("omitting target must close the caller itself")
	}
}

func TestHandleCoordinateEscalateRejectsMissingMessage(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")
	rec := postCoordinate(s, "/coordinate/escalate", token, map[string]any{"conversation": "root-1"})
	if rec.Code != 400 {
		t.Fatalf("want 400 for a missing message, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateEscalateRefusesAWrongToken(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	rec := postCoordinate(s, "/coordinate/escalate", "wrong-token",
		map[string]any{"conversation": "root-1", "message": "help"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a wrong token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectConversationNamesThePipelineWhenNotCoordinated(t *testing.T) {
	c := &agentopsv1alpha1.Conversation{}
	c.Name = "conv-1"
	c.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: "pipeline-a"}
	p := projectConversation(c)
	if p.Pipeline != "pipeline-a" {
		t.Fatalf("want the pipelineRef's name when there is no coordinatorRef, got %q", p.Pipeline)
	}
}

func TestHandleCoordinateReadRejectsBadJSON(t *testing.T) {
	s, _ := coordServer(t)
	req := httptest.NewRequest("POST", "/coordinate/read", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("want 400 for malformed JSON, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateReadChannelRefusesAWrongToken(t *testing.T) {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = "voice-desk", "agent-ops"
	s, _ := coordServer(t, ch)
	rec := postCoordinate(s, "/coordinate/read", "wrong-token", map[string]any{"channel": "voice-desk"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a wrong channel token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateReadConversationRefusesAWrongToken(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	rec := postCoordinate(s, "/coordinate/read", "wrong-token", map[string]any{"conversation": "root-1"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a wrong conversation token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateReadDefaultsTargetToTheCallerItself(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	root.Spec.Title = "the root"
	s, _ := coordServer(t, coordCoordinator("co-a"), root)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/read", token, map[string]any{"conversation": "root-1"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out conversationProjection
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "root-1" {
		t.Fatalf("omitting target must project the caller itself, got %+v", out)
	}
}

func TestHandleCoordinateReadRefusesAMissingTarget(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")
	rec := postCoordinate(s, "/coordinate/read", token,
		map[string]any{"conversation": "root-1", "target": "does-not-exist"})
	if rec.Code != 404 {
		t.Fatalf("want 404 for a missing target, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateReadRefusesADanglingAncestryChain(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	target := &agentopsv1alpha1.Conversation{}
	target.Name, target.Namespace = "target-1", "agent-ops"
	target.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "does-not-exist", Entry: "worker"}
	s, _ := coordServer(t, coordCoordinator("co-a"), root, target)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/read", token,
		map[string]any{"conversation": "root-1", "target": "target-1"})
	if rec.Code != 403 {
		t.Fatalf("a target whose ancestry chain breaks before reaching the caller must be refused, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateInvokeRefusesAChannelReaderToken(t *testing.T) {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = "voice-desk", "agent-ops"
	root := coordRoot("root-1", "co-a")
	s, _ := coordServer(t, coordCoordinator("co-a"), root, ch)

	token := chat.DeriveChannelReaderToken(coordTestMasterKey, "voice-desk")
	rec := postCoordinate(s, "/coordinate/invoke", token,
		map[string]any{"conversation": "root-1", "agent": "worker", "task": "do it"})
	if rec.Code != 401 {
		t.Fatalf("a channel-reader token must be refused for invoke, got %d: %s", rec.Code, rec.Body.String())
	}
}

// coordMember builds a member conversation the way createMember does: the
// causedBy field AND the label, since Descendants trusts only the field and
// uses the label merely as an index.
func coordMember(name, parent, entry string) *agentopsv1alpha1.Conversation {
	m := &agentopsv1alpha1.Conversation{}
	m.Name, m.Namespace = name, "agent-ops"
	m.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: parent}
	m.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent, Entry: entry}
	return m
}

func TestHandleCoordinateAgentsListsTheCallersOwnEntries(t *testing.T) {
	co := coordCoordinator("co-a",
		agentopsv1alpha1.CoordinatorAgentEntry{Name: "worker", Description: "does it", CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-worker"}},
	)
	root := coordRoot("root-1", "co-a")
	s, _ := coordServer(t, co, root)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/agents", token, map[string]any{"conversation": "root-1"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Agents []agentEntryView `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Agents) != 1 || out.Agents[0].Name != "worker" || out.Agents[0].Description != "does it" {
		t.Fatalf("want the single worker entry (name+description only), got %+v", out.Agents)
	}
}

func TestHandleCoordinateAgentsRefusesAWrongToken(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	rec := postCoordinate(s, "/coordinate/agents", "wrong-token", map[string]any{"conversation": "root-1"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a wrong token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateTreeDefaultsToTheCallersOwnSubtree(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	member := coordMember("member-1", "root-1", "worker")
	grandchild := coordMember("grandchild-1", "member-1", "helper")
	s, _ := coordServer(t, coordCoordinator("co-a"), root, member, grandchild)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/tree", token, map[string]any{"conversation": "root-1"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out treeNode
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "root-1" || len(out.Members) != 1 || out.Members[0].Name != "member-1" {
		t.Fatalf("want root-1 with member-1 nested, got %+v", out)
	}
	if len(out.Members[0].Members) != 1 || out.Members[0].Members[0].Name != "grandchild-1" {
		t.Fatalf("want member-1's own member grandchild-1 nested beneath it, got %+v", out.Members[0])
	}
}

func TestHandleCoordinateTreeRefusesATargetOutsideTheCallersSubtree(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	stranger := &agentopsv1alpha1.Conversation{}
	stranger.Name, stranger.Namespace = "stranger-1", "agent-ops"
	s, _ := coordServer(t, coordCoordinator("co-a"), root, stranger)
	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "root-1")

	rec := postCoordinate(s, "/coordinate/tree", token,
		map[string]any{"conversation": "root-1", "target": "stranger-1"})
	if rec.Code != 403 {
		t.Fatalf("want 403 for a target outside the caller's own subtree, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateTreeRefusesAWrongToken(t *testing.T) {
	s, _ := coordServer(t, coordCoordinator("co-a"), coordRoot("root-1", "co-a"))
	rec := postCoordinate(s, "/coordinate/tree", "wrong-token", map[string]any{"conversation": "root-1"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a wrong token, got %d: %s", rec.Code, rec.Body.String())
	}
}

// coordMember mirrors chat's own helper: the causedBy field AND the label,
// since the Router trusts only the field.
func coordMemberOf(name, parent, entry string) *agentopsv1alpha1.Conversation {
	m := &agentopsv1alpha1.Conversation{}
	m.Name, m.Namespace = name, "agent-ops"
	m.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: parent}
	m.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent, Entry: entry}
	return m
}

// The reaper's own shape: a plain member (no coordinatorRef of its own)
// authenticates /coordinate/open-roots with a token derived against the
// Coordinator its ANCESTOR ROOT names — callerActingForCoordinator's whole
// point (design D-A, coordinator-owner-reach).
func TestHandleCoordinateOpenRootsAuthenticatesAPlainMemberViaItsAncestorRoot(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	reaper := coordMemberOf("reaper-1", "root-1", "reaper")
	incident := coordRoot("incident-1", "co-a")
	s, _ := coordServer(t, coordCoordinator("co-a"), root, reaper, incident)

	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "reaper-1")
	rec := postCoordinate(s, "/coordinate/open-roots", token, map[string]any{"conversation": "reaper-1"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Roots []openRootView `json:"roots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Roots) != 1 || out.Roots[0].Name != "incident-1" {
		t.Fatalf("want only incident-1 (root-1 is the caller's own ancestor root), got %+v", out.Roots)
	}
}

func TestHandleCoordinateOpenRootsRefusesAWrongToken(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	reaper := coordMemberOf("reaper-1", "root-1", "reaper")
	s, _ := coordServer(t, coordCoordinator("co-a"), root, reaper)

	rec := postCoordinate(s, "/coordinate/open-roots", "wrong-token", map[string]any{"conversation": "reaper-1"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a wrong token, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A conversation resolving to no Coordinator at all (plain, no causedBy, no
// coordinatorRef) cannot be authenticated for this reach class — there is no
// name to re-derive a token against, so any presented token is refused
// identically to a wrong one.
func TestHandleCoordinateOpenRootsRefusesACallerWithNoCoordinatorScope(t *testing.T) {
	plain := &agentopsv1alpha1.Conversation{}
	plain.Name, plain.Namespace = "plain-1", "agent-ops"
	s, _ := coordServer(t, plain)

	rec := postCoordinate(s, "/coordinate/open-roots", "anything", map[string]any{"conversation": "plain-1"})
	if rec.Code != 401 {
		t.Fatalf("want 401 for a caller with no Coordinator scope, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCoordinateOpenRootsRejectsBadJSON(t *testing.T) {
	s, _ := coordServer(t)
	req := httptest.NewRequest("POST", "/coordinate/open-roots", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("want 400 for malformed JSON, got %d: %s", rec.Code, rec.Body.String())
	}
}

// The widened bound reachable at the HTTP layer: the reaper (a plain member)
// closes a SIBLING root of its own Coordinator it did not directly cause.
func TestHandleCoordinateCloseWidenedBoundPermitsAPlainMemberToCloseASiblingRoot(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	reaper := coordMemberOf("reaper-1", "root-1", "reaper")
	incident := coordRoot("incident-1", "co-a")
	s, c := coordServer(t, coordCoordinator("co-a"), root, reaper, incident)

	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "reaper-1")
	rec := postCoordinate(s, "/coordinate/close", token,
		map[string]any{"conversation": "reaper-1", "target": "incident-1", "reason": "resolved by the reaper"})
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "incident-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatal("the sibling root must be closed")
	}
}

// The reaper cannot reach its OWN ancestor root through the widened bound,
// even named directly.
func TestHandleCoordinateCloseWidenedBoundRefusesTheCallersOwnAncestorRoot(t *testing.T) {
	root := coordRoot("root-1", "co-a")
	reaper := coordMemberOf("reaper-1", "root-1", "reaper")
	s, c := coordServer(t, coordCoordinator("co-a"), root, reaper)

	token := chat.DeriveCoordinatorToken(coordTestMasterKey, "co-a", "reaper-1")
	rec := postCoordinate(s, "/coordinate/close", token,
		map[string]any{"conversation": "reaper-1", "target": "root-1", "reason": "trying to close my own root"})
	if rec.Code != 403 {
		t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "root-1"}, &got)
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("the reaper must never close its own ancestor root")
	}
}
