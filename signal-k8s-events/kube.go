package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// In-cluster Kubernetes access over net/http — no client-go, keeping this
// module dependency-free like every other adapter. Only what an Events watcher
// needs: list and watch on core v1 Events.

const (
	saDir       = "/var/run/secrets/kubernetes.io/serviceaccount"
	tokenMaxAge = 5 * time.Minute
)

// Kube is a minimal in-cluster API client.
type Kube struct {
	BaseURL string
	HTTP    *http.Client

	// TokenPath is re-read rather than cached forever: projected SA tokens are
	// short-lived and rotated in place, so a long-running watcher that read the
	// file once would start 401ing after the first rotation.
	TokenPath string

	mu          sync.Mutex
	token       string
	tokenLoaded time.Time
}

// NewInClusterKube builds a client from the pod's ServiceAccount mount and the
// API server address the kubelet injects.
func NewInClusterKube() (*Kube, error) {
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
	k := &Kube{
		BaseURL:   "https://" + net.JoinHostPort(host, port),
		TokenPath: saDir + "/token",
		HTTP: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}
	if _, err := k.bearer(); err != nil {
		return nil, err
	}
	return k, nil
}

// bearer returns the SA token, re-reading it when the cached copy is stale.
func (k *Kube) bearer() (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token != "" && time.Since(k.tokenLoaded) < tokenMaxAge {
		return k.token, nil
	}
	b, err := os.ReadFile(k.TokenPath)
	if err != nil {
		if k.token != "" {
			return k.token, nil // transient read failure — keep serving with the old token
		}
		return "", fmt.Errorf("reading ServiceAccount token: %w", err)
	}
	k.token = strings.TrimSpace(string(b))
	k.tokenLoaded = time.Now()
	return k.token, nil
}

func (k *Kube) get(ctx context.Context, path string, timeout time.Duration) (*http.Response, error) {
	token, err := k.bearer()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", k.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	client := k.HTTP
	if timeout > 0 {
		c := *k.HTTP
		c.Timeout = timeout
		client = &c
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		return nil, &apiError{Code: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return resp, nil
}

// apiError carries the API server's status code so callers can distinguish
// "the watch expired" (410) from "we lost our permissions" (403).
type apiError struct {
	Code int
	Body string
}

func (e *apiError) Error() string { return fmt.Sprintf("kubernetes API %d: %s", e.Code, e.Body) }

// Event is the subset of core/v1 Event this adapter reads.
type Event struct {
	Metadata struct {
		Name              string `json:"name"`
		Namespace         string `json:"namespace"`
		ResourceVersion   string `json:"resourceVersion"`
		UID               string `json:"uid"`
		CreationTimestamp string `json:"creationTimestamp"`
	} `json:"metadata"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"involvedObject"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	// Type is the Event severity: "Normal" or "Warning".
	Type           string `json:"type"`
	Count          int32  `json:"count"`
	FirstTimestamp string `json:"firstTimestamp"`
	LastTimestamp  string `json:"lastTimestamp"`
	// EventTime is set instead of LastTimestamp by newer event sources.
	EventTime string `json:"eventTime"`
}

// Namespace prefers the involved object's namespace and falls back to the
// event's own (they agree except for cluster-scoped objects).
func (e *Event) Namespace() string {
	if e.InvolvedObject.Namespace != "" {
		return e.InvolvedObject.Namespace
	}
	return e.Metadata.Namespace
}

// When returns the event's most recent occurrence, trying the fields in the
// order they are populated across event API versions. Zero when none parse —
// callers treat that as "no cursor information", never as "epoch".
func (e *Event) When() time.Time {
	for _, s := range []string{e.LastTimestamp, e.EventTime, e.FirstTimestamp, e.Metadata.CreationTimestamp} {
		if s == "" || s == "null" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC()
		}
		// microsecond precision (MicroTime, used by eventTime)
		if t, err := time.Parse("2006-01-02T15:04:05.999999Z07:00", s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

type eventList struct {
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
	Items []Event `json:"items"`
}

func eventsPath(namespace string) string {
	if namespace == "" {
		return "/api/v1/events"
	}
	return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/events"
}

// ListEvents returns the current events in a scope ("" = all namespaces) and
// the resourceVersion to start watching from.
func (k *Kube) ListEvents(ctx context.Context, namespace string) ([]Event, string, error) {
	resp, err := k.get(ctx, eventsPath(namespace)+"?limit=500", 30*time.Second)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	var list eventList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, "", fmt.Errorf("decoding event list: %w", err)
	}
	return list.Items, list.Metadata.ResourceVersion, nil
}

// watchFrame is one line of a streaming watch response.
type watchFrame struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

// ErrWatchExpired signals that the resourceVersion is too old (410 Gone) and
// the caller must relist rather than resume.
var ErrWatchExpired = fmt.Errorf("watch expired, relist required")

// WatchEvents streams events from resourceVersion, calling fn for each added or
// modified event. It returns when the stream ends, the context is cancelled, or
// the watch expires (ErrWatchExpired). DELETED frames are ignored: an event
// object aging out is not a signal.
func (k *Kube) WatchEvents(ctx context.Context, namespace, resourceVersion string, fn func(Event)) error {
	q := "?watch=1&allowWatchBookmarks=true&timeoutSeconds=300"
	if resourceVersion != "" {
		q += "&resourceVersion=" + url.QueryEscape(resourceVersion)
	}
	// no client timeout: a watch is a long-lived stream, bounded by
	// timeoutSeconds server-side and by ctx here
	resp, err := k.get(ctx, eventsPath(namespace)+q, 0)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusGone {
			return ErrWatchExpired
		}
		return err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	for {
		var frame watchFrame
		if err := dec.Decode(&frame); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return nil // server closed the stream — caller re-watches
			}
			return fmt.Errorf("decoding watch stream: %w", err)
		}
		switch frame.Type {
		case "ADDED", "MODIFIED":
			var ev Event
			if err := json.Unmarshal(frame.Object, &ev); err != nil {
				continue // a frame we cannot parse is not worth dropping the stream for
			}
			fn(ev)
		case "ERROR":
			var status struct {
				Code   int32  `json:"code"`
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(frame.Object, &status)
			if status.Code == http.StatusGone || status.Reason == "Expired" {
				return ErrWatchExpired
			}
			return fmt.Errorf("watch error frame: %s (%d)", status.Reason, status.Code)
		}
	}
}
