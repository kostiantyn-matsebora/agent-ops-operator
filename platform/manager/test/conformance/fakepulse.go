//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// FakePulse is the slice of Pulse's REST API signal-pulse speaks: the
// notification-webhooks list, create and update calls, behind Pulse's
// bearer API token. It stores what the adapter registered, and redacts the
// signing secret on read the way Pulse does, so the adapter's
// create-or-repair comparison runs against the shape it meets in
// production.
type FakePulse struct {
	srv   *httptest.Server
	Token string

	mu           sync.Mutex
	webhooks     []PulseWebhook
	nextID       int
	unauthorized int
}

// PulseWebhook is one registered Generic webhook, as the fake stores it —
// the signing secret unredacted, since the suite signs deliveries with it.
type PulseWebhook struct {
	ID            string            `json:"id,omitempty"`
	Name          string            `json:"name"`
	URL           string            `json:"url"`
	Template      string            `json:"template"`
	Headers       map[string]string `json:"headers,omitempty"`
	SigningSecret string            `json:"signingSecret,omitempty"`
	Enabled       bool              `json:"enabled"`
}

// pulseRedacted stands in for the stored signing secret on every read.
const pulseRedacted = "********"

// NewFakePulse starts the fake. URL() is what a source's config.baseUrl
// names.
func NewFakePulse(t *testing.T, token string) *FakePulse {
	t.Helper()
	f := &FakePulse{Token: token}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/notifications/webhooks", f.authed(f.list))
	mux.HandleFunc("POST /api/notifications/webhooks", f.authed(f.create))
	mux.HandleFunc("PUT /api/notifications/webhooks/{id}", f.authed(f.update))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// URL is Pulse's base URL.
func (f *FakePulse) URL() string { return f.srv.URL }

// Webhooks lists what is registered, secrets included.
func (f *FakePulse) Webhooks() []PulseWebhook {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PulseWebhook{}, f.webhooks...)
}

// Unauthorized counts calls that did not present the API token.
func (f *FakePulse) Unauthorized() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unauthorized
}

func (f *FakePulse) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != f.Token {
			f.mu.Lock()
			f.unauthorized++
			f.mu.Unlock()
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (f *FakePulse) list(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	out := make([]PulseWebhook, 0, len(f.webhooks))
	for _, h := range f.webhooks {
		if h.SigningSecret != "" {
			h.SigningSecret = pulseRedacted
		}
		out = append(out, h)
	}
	f.mu.Unlock()
	writeJSON(w, 200, out)
}

func (f *FakePulse) create(w http.ResponseWriter, r *http.Request) {
	var h PulseWebhook
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	f.mu.Lock()
	f.nextID++
	h.ID = fmt.Sprintf("wh-%d", f.nextID)
	f.webhooks = append(f.webhooks, h)
	f.mu.Unlock()
	writeJSON(w, 200, h)
}

func (f *FakePulse) update(w http.ResponseWriter, r *http.Request) {
	var h PulseWebhook
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	id := r.PathValue("id")
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.webhooks {
		if f.webhooks[i].ID == id {
			h.ID = id
			f.webhooks[i] = h
			writeJSON(w, 200, h)
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "no webhook " + id})
}
