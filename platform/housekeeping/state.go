package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// State is a client for the manager's state read surface
// (state-store-generic-contract, design D-5/D-6) — plain net/http against the
// manager's in-cluster Service, not the Kubernetes API server. Unlike the
// Kube client this replaces, it needs no CA trust pool of its own: the
// manager's Service terminates no TLS inside the cluster, so an ordinary
// http.Client is the whole transport.
type State struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewState builds a client from the manager's Service URL and the
// STATE_READ_TOKEN the chart's stateReadAuth Secret projects into this
// CronJob's env (design D-6). Housekeeping holds no derived adapter token and
// must not hold the master token — this credential opens /state/* alone.
func NewState(baseURL, token string) *State {
	return &State{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Conversation is the slice of a Conversation this job reads. Deliberately
// tiny: a name to match a directory against, and the context handle that keeps
// a transcript referenced.
//
// NOTE the absence of a phase field, and keep it absent. See ListConversations.
type Conversation struct {
	Name      string
	ContextID string
}

// stateConversationsEnvelope is the wire shape of GET /state/conversations:
// {"revision", "items": [record]} — see design D-5. Only the two fields this
// job reads are decoded; the rest of each record (spec, the run history) is
// ignored.
type stateConversationsEnvelope struct {
	Items []struct {
		Name   string `json:"name"`
		Status struct {
			RuntimeContextID string `json:"runtimeContextId"`
			SessionID        string `json:"sessionId"`
		} `json:"status"`
	} `json:"items"`
}

// ListConversations returns EVERY conversation the manager's state read
// surface serves.
//
// PHASE-BLIND, and that is the property the whole design rests on. A closed
// conversation still has a CR, so its workspace and its transcripts are
// protected by the same rule that identifies an orphan — this job needs no
// knowledge of phases, no closed list and no second rule. GET
// /state/conversations is itself phase-blind by design (no filter), so this
// client adds none.
//
// A future "optimisation" that filtered to live conversations here, to avoid
// looking at ones nobody is using, would reclaim the workspace of every
// conversation an operator was deliberately keeping. Do not add a phase filter.
func (s *State) ListConversations(ctx context.Context) ([]Conversation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/state/conversations", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing conversations: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("listing conversations: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var env stateConversationsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decoding conversation listing: %w", err)
	}
	out := make([]Conversation, 0, len(env.Items))
	for _, it := range env.Items {
		// Dual-read: sessionId is the retired spelling of runtimeContextId, and
		// a conversation written by an older manager still carries only that.
		// Missing it here would make its transcript look unreferenced.
		id := it.Status.RuntimeContextID
		if id == "" {
			id = it.Status.SessionID
		}
		out = append(out, Conversation{Name: it.Name, ContextID: id})
	}
	return out, nil
}
