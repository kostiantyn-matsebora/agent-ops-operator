// The manager's half of the `aops-mcp-server` contract (design D-F):
//
//	POST /coordinate/invoke     {conversation, agent, task}    -> {member, created}
//	POST /coordinate/close      {conversation, target, reason}
//	POST /coordinate/escalate   {conversation, message}
//	POST /coordinate/read       {conversation|channel, target} -> a projection
//
// Every verb carries the CALLER'S OWN declared identity — a conversation name
// or a channel name — proven by a bearer token the manager derives and
// compares by RE-DERIVATION, never stored: `coordinator:<name>:<conversation>`
// for a coordinated conversation's own token, `channel-reader:<channel>` for
// the second reach class. The server that will eventually forward these
// (`platform/mcp-aops/`, a later phase) decides nothing; every bound in the
// table below is enforced HERE, from the token alone.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// RegisterCoordinateRoutes wires the /coordinate/* surface. A separate
// function, called from Handler(), rather than more lines inline there — this
// surface has its own auth (per-conversation and per-channel tokens) rather
// than the adapter contract's, and keeping it grouped is what makes that
// visible.
func (s *Server) RegisterCoordinateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /coordinate/invoke", s.handleCoordinateInvoke)
	mux.HandleFunc("POST /coordinate/close", s.handleCoordinateClose)
	mux.HandleFunc("POST /coordinate/escalate", s.handleCoordinateEscalate)
	mux.HandleFunc("POST /coordinate/read", s.handleCoordinateRead)
}

// bearerToken reads the presented token, exactly as the adapter contract does.
func bearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// errWrongToken is returned (never exposed verbatim) when a presented token
// does not re-derive to the identity the caller declared.
var errWrongToken = errors.New("token does not match the declared identity")

// callerConversation resolves and authenticates the CALLING conversation: the
// caller declares its own name, and the manager re-derives
// `coordinator:<coordinatorRef>:<name>` from the master key and compares
// constant-time. A name naming no coordinator-rooted conversation, or a token
// that does not match, both fail identically — the caller learns nothing
// about which.
func (s *Server) callerConversation(ctx context.Context, name, token string) (*agentopsv1alpha1.Conversation, error) {
	if s.AdapterToken == "" || name == "" || token == "" {
		return nil, errWrongToken
	}
	var conv agentopsv1alpha1.Conversation
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: name}, &conv); err != nil {
		return nil, errWrongToken
	}
	if conv.Spec.CoordinatorRef == nil {
		return nil, errWrongToken
	}
	want := chat.DeriveCoordinatorToken(s.AdapterToken, conv.Spec.CoordinatorRef.Name, conv.Name)
	if subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
		return nil, errWrongToken
	}
	return &conv, nil
}

// callerChannel authenticates a channel-reader token against the channel it
// declares (design D-F's second reach class).
func (s *Server) callerChannel(ctx context.Context, name, token string) (*agentopsv1alpha1.Channel, error) {
	if s.AdapterToken == "" || name == "" || token == "" {
		return nil, errWrongToken
	}
	var ch agentopsv1alpha1.Channel
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: name}, &ch); err != nil {
		return nil, errWrongToken
	}
	want := chat.DeriveChannelReaderToken(s.AdapterToken, ch.Name)
	if subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
		return nil, errWrongToken
	}
	return &ch, nil
}

func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func writeCoordinateError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// statusFor maps a coordination refusal to the HTTP status a caller can act
// on: scope and validation refusals are 4xx, anything else this handler did
// not anticipate is a 500.
func statusFor(err error) int {
	switch {
	case errors.Is(err, errWrongToken):
		return 401
	case errors.Is(err, chat.ErrNotCoordinatorRoot),
		errors.Is(err, chat.ErrUnknownAgent),
		errors.Is(err, chat.ErrCoordinatorCycle),
		errors.Is(err, chat.ErrMaxAgents),
		errors.Is(err, chat.ErrOutOfScope):
		return 403
	default:
		return 500
	}
}

type invokeReq struct {
	Conversation string `json:"conversation"`
	Agent        string `json:"agent"`
	Task         string `json:"task"`
}

func (s *Server) handleCoordinateInvoke(w http.ResponseWriter, r *http.Request) {
	var in invokeReq
	if err := readJSON(r, &in); err != nil || in.Agent == "" || in.Task == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"conversation","agent","task"}`})
		return
	}
	ctx := r.Context()
	caller, err := s.callerConversation(ctx, in.Conversation, bearerToken(r))
	if err != nil {
		writeCoordinateError(w, statusFor(err), err)
		return
	}
	result, err := s.Router.InvokeMember(ctx, caller, in.Agent, in.Task)
	if err != nil {
		writeCoordinateError(w, statusFor(err), err)
		return
	}
	status := "created"
	if !result.Created {
		status = "attached"
	}
	writeJSON(w, 200, map[string]string{"member": result.Member, "status": status})
}

type closeReq struct {
	Conversation string `json:"conversation"`
	Target       string `json:"target"`
	Reason       string `json:"reason"`
}

func (s *Server) handleCoordinateClose(w http.ResponseWriter, r *http.Request) {
	var in closeReq
	if err := readJSON(r, &in); err != nil || in.Reason == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"conversation","target","reason"} — a coordinator's close requires a reason`})
		return
	}
	ctx := r.Context()
	caller, err := s.callerConversation(ctx, in.Conversation, bearerToken(r))
	if err != nil {
		writeCoordinateError(w, statusFor(err), err)
		return
	}
	target := in.Target
	if target == "" {
		target = caller.Name
	}
	if err := s.Router.CloseCoordinated(ctx, caller, target, in.Reason); err != nil {
		writeCoordinateError(w, statusFor(err), err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type escalateReq struct {
	Conversation string `json:"conversation"`
	Message      string `json:"message"`
}

func (s *Server) handleCoordinateEscalate(w http.ResponseWriter, r *http.Request) {
	var in escalateReq
	if err := readJSON(r, &in); err != nil || in.Message == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"conversation","message"}`})
		return
	}
	ctx := r.Context()
	// escalate takes NO conversation argument beyond the caller's own identity
	// (aops-mcp-server): it always acts on the calling conversation, never a
	// member reached through it.
	caller, err := s.callerConversation(ctx, in.Conversation, bearerToken(r))
	if err != nil {
		writeCoordinateError(w, statusFor(err), err)
		return
	}
	if err := s.Router.Escalate(ctx, caller, in.Message); err != nil {
		writeCoordinateError(w, statusFor(err), err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// conversationProjection is the bounded view both reach classes get: never a
// transcript, never a run, never the input queue.
type conversationProjection struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Brief       string `json:"brief,omitempty"`
	Phase       string `json:"phase,omitempty"`
	Pipeline    string `json:"pipeline,omitempty"`
	CloseReason string `json:"closeReason,omitempty"`
}

func projectConversation(c *agentopsv1alpha1.Conversation) conversationProjection {
	p := conversationProjection{
		Name: c.Name, Title: c.Spec.Title, Brief: c.Status.Brief,
		Phase: string(c.Status.Phase), CloseReason: c.Status.CloseReason,
	}
	if c.Spec.CoordinatorRef != nil {
		p.Pipeline = c.Spec.CoordinatorRef.Name
	} else if c.Spec.PipelineRef != nil {
		p.Pipeline = c.Spec.PipelineRef.Name
	}
	return p
}

type readReq struct {
	Conversation string `json:"conversation,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Target       string `json:"target,omitempty"`
}

// handleCoordinateRead serves BOTH reach classes (design D-F): a coordinated
// conversation reading ONE conversation within its own subtree, at any depth,
// and a channel-reader listing the projection of every conversation with a
// thread on its channel.
func (s *Server) handleCoordinateRead(w http.ResponseWriter, r *http.Request) {
	var in readReq
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": `need {"conversation","target"} or {"channel"}`})
		return
	}
	ctx := r.Context()
	token := bearerToken(r)
	if in.Channel != "" {
		ch, err := s.callerChannel(ctx, in.Channel, token)
		if err != nil {
			writeCoordinateError(w, statusFor(err), err)
			return
		}
		s.readChannelProjection(w, r, ch)
		return
	}
	caller, err := s.callerConversation(ctx, in.Conversation, token)
	if err != nil {
		writeCoordinateError(w, statusFor(err), err)
		return
	}
	target := in.Target
	if target == "" {
		target = caller.Name
	}
	if target == caller.Name {
		writeJSON(w, 200, projectConversation(caller))
		return
	}
	var t agentopsv1alpha1.Conversation
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: target}, &t); err != nil {
		writeCoordinateError(w, 404, chat.ErrOutOfScope)
		return
	}
	inScope, err := s.descendsFrom(ctx, &t, caller.Name)
	if err != nil {
		writeCoordinateError(w, 500, err)
		return
	}
	if !inScope {
		writeCoordinateError(w, 403, chat.ErrOutOfScope)
		return
	}
	writeJSON(w, 200, projectConversation(&t))
}

// descendsFrom walks UP from a target conversation's `causedBy` chain,
// reporting whether it reaches ancestorName — the "own subtree, at any depth"
// bound aops-mcp-server states for `read`.
func (s *Server) descendsFrom(ctx context.Context, target *agentopsv1alpha1.Conversation, ancestorName string) (bool, error) {
	cur := target
	for i := 0; i < 1000; i++ {
		if cur.Spec.CausedBy == nil {
			return false, nil
		}
		if cur.Spec.CausedBy.Parent == ancestorName {
			return true, nil
		}
		var parent agentopsv1alpha1.Conversation
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: cur.Spec.CausedBy.Parent}, &parent); err != nil {
			return false, client.IgnoreNotFound(err)
		}
		cur = &parent
	}
	return false, errors.New("causedBy chain did not terminate")
}

func (s *Server) readChannelProjection(w http.ResponseWriter, r *http.Request, ch *agentopsv1alpha1.Channel) {
	ctx := r.Context()
	var list agentopsv1alpha1.ConversationList
	if err := s.Reader.List(ctx, &list, client.InNamespace(s.Namespace)); err != nil {
		writeCoordinateError(w, 500, err)
		return
	}
	out := []conversationProjection{}
	for i := range list.Items {
		c := &list.Items[i]
		if c.ThreadFor(ch.Name) == nil {
			continue
		}
		out = append(out, projectConversation(c))
	}
	writeJSON(w, 200, map[string]any{"conversations": out})
}
