package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The two ways in, and the one way that must never open.
//
// The console authenticates browsers itself OR the release declares that
// something in front of it already did. Everything worth pinning here is about
// keeping those two apart: an absent token is not a declaration, half a
// declaration is not one either, and a request nobody vouched for cannot write.

// authAPI builds the browser surface with an explicit auth mode. The process
// config is what varies — the served Channel's config would override it, and
// the fake serves none, which is also how a hand-run console behaves.
func authAPI(t *testing.T, staticToken string, authEnabled bool, external string, objs ...*Object) (*API, *fakeManager) {
	t.Helper()
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	adapter, tr, cache := consoleUnderTest(t, f, objs...)
	adapter.refreshChannels(context.Background())
	api := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500),
		Manager:  adapter.mgr,
		Config: &Config{
			Namespace: "agent-ops", AdapterName: "console", UIToken: staticToken,
			WriteEnabled: true, SignalSourceName: "console",
			AuthEnabled: authEnabled, ExternalAuthenticator: external,
		},
	})
	return api, f
}

// joinedConversation is a conversation the console channel carries, so a write
// has somewhere to go.
func joinedConversation(name string) *Object {
	return obj("conversations", name, "1",
		`{"profileRef":{"name":"ops"},"channelRefs":[{"name":"console"}]}`,
		`{"threads":[{"channel":"console","threadId":"console-uid-`+name+`"}]}`)
}

func withIdentity(r *http.Request, who string) *http.Request {
	r.Header.Set("X-Forwarded-Email", who)
	return r
}

// THE case that must never regress. Authentication on and no token configured
// authorizes nobody — not with a forward-auth header, and not with half an
// external declaration either. `auth.enabled: false` alone is a configuration
// that lost its other half, and a lost value must not open a door.
func TestEmptyTokenStaysClosedWhateverElseIsSet(t *testing.T) {
	for _, tc := range []struct {
		name        string
		authEnabled bool
		external    string
	}{
		{"authentication on", true, ""},
		{"authentication on, authenticator named anyway", true, "oauth2-proxy"},
		{"authentication off with nothing named", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, _ := authAPI(t, "", tc.authEnabled, tc.external)
			h := api.Handler(http.NotFoundHandler())

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, withIdentity(httptest.NewRequest("GET", "/api/topology", nil), "alice"))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("want 401, got %d %s", rec.Code, rec.Body.String())
			}
			// and the session endpoint must not claim otherwise
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/session", nil))
			if s := decodeSession(t, rec); s.Authenticated {
				t.Fatalf("session reported authenticated: %+v", s)
			}
		})
	}
}

// A console that authenticates behaves exactly as it did: the token gets in,
// the mode is reported as token, and a write is attributed to the credential
// actually proven.
func TestTokenAuthIsUnchanged(t *testing.T) {
	api, f := authAPI(t, "tok", true, "", joinedConversation("c"))
	h := api.Handler(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/session", nil))
	s := decodeSession(t, rec)
	if s.Authenticated || s.AuthMode != "token" || !s.Configured {
		t.Fatalf("unauthenticated session on a token console: %+v", s)
	}

	rec = authed(t, h, "GET", "/api/session", "")
	if s := decodeSession(t, rec); !s.Authenticated || s.Identity != "token" || s.IdentitySource != "token" || !s.CanWrite {
		t.Fatalf("token session: %+v", s)
	}
	if rec := authed(t, h, "GET", "/api/topology", ""); rec.Code != http.StatusOK {
		t.Fatalf("read with the token: %d", rec.Code)
	}
	if rec := authed(t, h, "POST", "/api/conversations/c/messages", `{"text":"hi"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("write with the token: %d %s", rec.Code, rec.Body.String())
	}
	if in := f.inbounds(); len(in) != 1 || in[0]["sender"] != "token" {
		t.Fatalf("a write with only the token proven is recorded as token: %+v", in)
	}
}

// The declaration — not the absence of a credential — is what opens the door.
// No token is configured at all here, which under any other configuration is
// the closed case.
func TestExternalAuthServesReadsWithoutAToken(t *testing.T) {
	api, _ := authAPI(t, "", false, "oauth2-proxy")
	h := api.Handler(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/topology", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("external auth must serve reads: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/session", nil))
	s := decodeSession(t, rec)
	if !s.Authenticated || s.AuthMode != "external" || s.ExternalAuthenticator != "oauth2-proxy" {
		t.Fatalf("session must report the mode and who authenticates: %+v", s)
	}
	// no login form for a console that accepts no token: it reports the request
	// as authenticated, so the SPA never renders one
	if s.Identity != "" || s.IdentitySource != "" || s.CanWrite {
		t.Fatalf("no forward-auth header must mean no identity and no writes: %+v", s)
	}
}

// The proxy says who you are, and that is what the write is recorded against —
// in the manager's inbound and in the console's own log, which is the only
// record of who instructed an agent.
func TestExternalAuthWritesUnderTheForwardedIdentity(t *testing.T) {
	api, f := authAPI(t, "", false, "oauth2-proxy", joinedConversation("c"))
	h := api.Handler(http.NotFoundHandler())

	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withIdentity(jsonReq("POST", "/api/conversations/c/messages", `{"text":"hi"}`), "alice@example.com"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("a forwarded identity must be able to write: %d %s", rec.Code, rec.Body.String())
	}
	if in := f.inbounds(); len(in) != 1 || in[0]["sender"] != "alice@example.com" {
		t.Fatalf("the write must carry the forwarded identity: %+v", in)
	}
	if !strings.Contains(logged.String(), "identity=alice@example.com") {
		t.Fatalf("the write log must name the identity, got %q", logged.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withIdentity(httptest.NewRequest("GET", "/api/session", nil), "alice@example.com"))
	if s := decodeSession(t, rec); s.Identity != "alice@example.com" || s.IdentitySource != "forward-auth" || !s.CanWrite {
		t.Fatalf("session: %+v", s)
	}
}

// A proxy that authenticates but forwards no identity gives a READ-ONLY
// console. The write is refused rather than attributed to "token", which would
// name a credential nobody presented, and the refusal says what to fix.
func TestExternalAuthWithoutAnIdentityIsReadOnly(t *testing.T) {
	api, f := authAPI(t, "", false, "oauth2-proxy", joinedConversation("c"))
	h := api.Handler(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/conversations/c", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reads must still be served: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq("POST", "/api/conversations/c/messages", `{"text":"hi"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "oauth2-proxy") ||
		!strings.Contains(body, "X-Forwarded-Email") {
		t.Fatalf("the refusal must name who authenticates and what to forward: %s", body)
	}
	if len(f.inbounds()) != 0 {
		t.Fatal("nothing may reach the manager without an identity")
	}
}

// sessionView mirrors GET /api/session.
type sessionView struct {
	Authenticated         bool   `json:"authenticated"`
	Configured            bool   `json:"configured"`
	Identity              string `json:"identity"`
	IdentitySource        string `json:"identitySource"`
	AuthMode              string `json:"authMode"`
	ExternalAuthenticator string `json:"externalAuthenticator"`
	WriteEnabled          bool   `json:"writeEnabled"`
	CanWrite              bool   `json:"canWrite"`
}

func decodeSession(t *testing.T, rec *httptest.ResponseRecorder) sessionView {
	t.Helper()
	var s sessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("session body %q: %v", rec.Body.String(), err)
	}
	return s
}
