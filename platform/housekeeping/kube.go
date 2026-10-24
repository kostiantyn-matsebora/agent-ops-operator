package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// In-cluster Kubernetes access over net/http — no client-go, keeping this
// module dependency-free like every other satellite. READ ONLY: this workload
// performs no API writes at all, because both etcd-side stages of the
// conversation lifecycle belong to the manager. If a write ever appears here,
// the split in D7 has been undone.

// saDir is a var, not a const, ONLY so a test can point it at a temp
// directory: the real path requires root to create outside a pod, so the
// success path of NewInClusterKube (and its PEM-parsing error branches) is
// otherwise unreachable from a unit test. Production code always sees the
// real path; nothing here changes runtime behavior.
var saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// Kube is a minimal in-cluster API client.
type Kube struct {
	BaseURL   string
	HTTP      *http.Client
	TokenPath string
	Namespace string
}

// NewInClusterKube builds a client from the pod's ServiceAccount mount and the
// API server address the kubelet injects.
func NewInClusterKube(namespace string) (*Kube, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("not running in a cluster: KUBERNETES_SERVICE_HOST/PORT unset")
	}
	caPEM, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("reading API server CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("API server CA at %s/ca.crt is not valid PEM", saDir)
	}
	return &Kube{
		BaseURL:   "https://" + net.JoinHostPort(host, port),
		TokenPath: saDir + "/token",
		Namespace: namespace,
		HTTP: &http.Client{
			Timeout:   60 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}, nil
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

type convList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			RuntimeContextID string `json:"runtimeContextId"`
			SessionID        string `json:"sessionId"`
		} `json:"status"`
	} `json:"items"`
}

// ListConversations returns EVERY conversation in the namespace.
//
// PHASE-BLIND, and that is the property the whole design rests on. A closed
// conversation still has a CR, so its workspace and its transcripts are
// protected by the same rule that identifies an orphan — this job needs no
// knowledge of phases, no closed list and no second rule.
//
// A future "optimisation" that filtered to live conversations here, to avoid
// looking at ones nobody is using, would reclaim the workspace of every
// conversation an operator was deliberately keeping. Do not add a phase filter.
func (k *Kube) ListConversations(ctx context.Context) ([]Conversation, error) {
	path := fmt.Sprintf("/apis/agentops.dev/v1alpha1/namespaces/%s/conversations", url.PathEscape(k.Namespace))
	token, err := os.ReadFile(k.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("reading ServiceAccount token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", k.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Accept", "application/json")
	resp, err := k.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("listing conversations: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var list convList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(list.Items))
	for _, it := range list.Items {
		// Dual-read: sessionId is the retired spelling of runtimeContextId, and
		// a conversation written by an older manager still carries only that.
		// Missing it here would make its transcript look unreferenced.
		id := it.Status.RuntimeContextID
		if id == "" {
			id = it.Status.SessionID
		}
		out = append(out, Conversation{Name: it.Metadata.Name, ContextID: id})
	}
	return out, nil
}
