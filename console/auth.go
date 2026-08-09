package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Auth sized to what the console can DO.
//
// Requirements 4 and 6 make this a control plane, not a viewer: it can instruct
// an agent that, in this install, holds cluster-admin. Three properties follow,
// and each is here rather than in a doc because each is easy to erode:
//
//   - AN UNCONFIGURED TOKEN AUTHORIZES NOBODY, and is indistinguishable from a
//     wrong one. "No token set" must never read as "no authentication required"
//     — that is the failure mode where a fresh install is wide open.
//   - WRITES NEED AN IDENTITY, not just a session. A trusted forward-auth header
//     supplies one when a proxy has already authenticated the user (oauth2-proxy
//     in front of an Ingress); otherwise the identity is "token", which is
//     honest about what was actually proven.
//   - EVERY WRITE IS LOGGED with that identity: who started what, and who said
//     what to an agent.

const sessionCookie = "agentops_console"
const sessionTTL = 12 * time.Hour

// forwardAuthHeaders are the identity headers a fronting proxy sets, in
// preference order. They are trusted ONLY because reaching this port at all
// requires being inside the cluster or past that proxy — the console is
// ClusterIP with no Ingress by default, and the documented way to expose it is
// behind one of these.
var forwardAuthHeaders = []string{
	"X-Forwarded-Preferred-Username",
	"X-Forwarded-Email",
	"X-Forwarded-User",
	"X-Auth-Request-Preferred-Username",
	"X-Auth-Request-Email",
	"X-Auth-Request-User",
}

// identityKey carries the resolved writer identity through the request context.
type identityKey struct{}

// Identity returns the authenticated writer for a request ("token" when only
// the shared token was proven).
func Identity(r *http.Request) string {
	if v, ok := r.Context().Value(identityKey{}).(string); ok && v != "" {
		return v
	}
	return "token"
}

// Sessions holds browser sessions minted from the shared token.
type Sessions struct {
	mu   sync.Mutex
	byID map[string]time.Time
}

// NewSessions builds an empty session store.
func NewSessions() *Sessions { return &Sessions{byID: map[string]time.Time{}} }

func (s *Sessions) valid(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.byID[id]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.byID, id)
		return false
	}
	return true
}

func (s *Sessions) mint() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	s.mu.Lock()
	s.byID[id] = time.Now().Add(sessionTTL)
	for sid, exp := range s.byID { // opportunistic expiry sweep
		if time.Now().After(exp) {
			delete(s.byID, sid)
		}
	}
	s.mu.Unlock()
	return id, nil
}

func (s *Sessions) drop(id string) {
	s.mu.Lock()
	delete(s.byID, id)
	s.mu.Unlock()
}

// resolveIdentity reads a trusted forward-auth identity, or "" when none is
// present. It never invents one: "token" is recorded by Identity() instead, so
// a log line distinguishes "alice signed in through OIDC" from "somebody held
// the shared token".
func resolveIdentity(r *http.Request) string {
	for _, h := range forwardAuthHeaders {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			return v
		}
	}
	return ""
}

// auth guards a READ route.
func (a *API) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, resolveIdentity(r))))
	}
}

// write guards a WRITE route: authentication, plus the install-wide write gate.
// The two failures are reported apart because they need different fixes — one
// is "sign in", the other is "your operator turned this off".
func (a *API) write(action string, next http.HandlerFunc) http.HandlerFunc {
	return a.auth(func(w http.ResponseWriter, r *http.Request) {
		if !a.writesAllowed() {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "this console is read-only (console.write.enabled=false)",
			})
			return
		}
		log.Printf("console write: action=%s identity=%s path=%s", action, Identity(r), r.URL.Path)
		next(w, r)
	})
}

// authorized accepts a live session cookie or a bearer token equal to the
// configured one (constant-time). An unconfigured console authorizes nobody.
func (a *API) authorized(r *http.Request) bool {
	want := a.token()
	if want == "" {
		return false
	}
	if c, err := r.Cookie(sessionCookie); err == nil && a.sessions.valid(c.Value) {
		return true
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"token":"…"}`})
		return
	}
	want := a.token()
	if want == "" || subtle.ConstantTimeCompare([]byte(in.Token), []byte(want)) != 1 {
		// same answer either way: an unconfigured console must not be
		// distinguishable from a wrong password
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	id, err := a.sessions.mint()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL / time.Second),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.sessions.drop(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// handleSession lets the SPA decide between the login form and the app without
// provoking a 401 on every load, and tells it which write affordances to render
// at all. Hiding a button the server would refuse is presentation; the server
// refuses regardless.
func (a *API) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": a.authorized(r),
		"configured":    a.token() != "",
		"identity":      resolveIdentity(r),
		"writeEnabled":  a.writesAllowed(),
		"canOriginate":  a.originator != nil,
		"metrics":       a.metricsBackend() != nil,
	})
}
