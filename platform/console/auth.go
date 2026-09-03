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
//     The ONE named exception is an explicit declaration that something else
//     authenticates: `auth.enabled: false` TOGETHER WITH a named
//     `externalAuthenticator`. Two deliberate statements, neither of which can
//     be arrived at by omission — which is precisely what separates it from the
//     failure mode above. Half of it (a false with nothing named) leaves the
//     console closed.
//   - WRITES NEED AN IDENTITY, not just a session. A trusted forward-auth header
//     supplies one when a proxy has already authenticated the user (oauth2-proxy
//     in front of an Ingress); otherwise the identity is "token", which is
//     honest about what was actually proven. Under external authentication
//     there is no such fallback: no token was proven, so a write with no
//     forward-auth identity is REFUSED rather than attributed to one. Reads
//     continue — a proxy forwarding no identity yields a read-only console.
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

// Identity returns the authenticated writer for a request, or "" when none
// could be resolved.
//
// The value is DECIDED BY THE MIDDLEWARE (see auth), not reconstructed here:
// "token" is a legitimate identity only where a token was actually proven, and
// this function cannot see which mode admitted the request. It used to default
// to "token", which under external authentication would have written a name for
// a credential nobody presented.
func Identity(r *http.Request) string {
	v, _ := r.Context().Value(identityKey{}).(string)
	return v
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
// present. It never invents one: "token" is supplied by the middleware, and
// only where a token was proven, so a log line distinguishes "alice signed in
// through OIDC" from "somebody held the shared token" — and, under external
// authentication, from "nobody said who this was".
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
		identity, ok := a.admit(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	}
}

// write guards a WRITE route: authentication, the install-wide write gate, and
// an identity to record it against.
//
// The three failures are reported apart because they need different fixes —
// "sign in", "your operator turned this off", and "the proxy in front of this
// console is not forwarding who you are". The last one is the honest end of
// dropping the `token` fallback: it is better to say a write cannot be
// attributed than to attribute it to a credential nobody presented.
func (a *API) write(action string, next http.HandlerFunc) http.HandlerFunc {
	return a.auth(func(w http.ResponseWriter, r *http.Request) {
		if !a.writesAllowed() {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "this console is read-only (console.write.enabled=false)",
			})
			return
		}
		if Identity(r) == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "writes need an identity: this console authenticates nobody itself (" +
					a.externalAuthenticatorName() + " does) and the request carries no forward-auth " +
					"identity header, so there is nothing to record this write against. " +
					"Configure the proxy to forward one (X-Forwarded-Email or X-Auth-Request-User).",
			})
			return
		}
		log.Printf("console write: action=%s identity=%s path=%s", action, Identity(r), r.URL.Path)
		next(w, r)
	})
}

// admit is THE authentication decision, and it is one decision rather than two
// code paths: was this request authenticated BY US, or did the release declare
// that someone else authenticated it? It returns the identity to record along
// with the verdict, because who you are depends on which of those it was.
//
// By us: a live session cookie or a bearer token equal to the configured one
// (constant-time), identity being the forward-auth header when a proxy supplied
// one and otherwise "token" — honest about what was proven. An unconfigured
// console authorizes nobody, and that is NOT relaxed by anything here.
//
// By someone else: only when the release said so twice over (see
// authIsExternal). Identity is then the forward-auth header or nothing at all.
func (a *API) admit(r *http.Request) (identity string, ok bool) {
	if a.authIsExternal() {
		return resolveIdentity(r), true
	}
	want := a.token()
	if want == "" {
		return "", false
	}
	if c, err := r.Cookie(sessionCookie); err == nil && a.sessions.valid(c.Value) {
		return identityOrToken(r), true
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
		return identityOrToken(r), true
	}
	return "", false
}

// authorized answers the same question without the identity, for the session
// endpoint and anything else that only needs the verdict.
func (a *API) authorized(r *http.Request) bool {
	_, ok := a.admit(r)
	return ok
}

// identityOrToken names the writer when the CONSOLE authenticated the request:
// the proxy's identity when there is one, else the shared token.
func identityOrToken(r *http.Request) string {
	if id := resolveIdentity(r); id != "" {
		return id
	}
	return "token"
}

// secureCookie reports whether THIS request reached us over TLS, directly or
// through a proxy that terminated it (oauth2-proxy, an ingress) and said so
// via the standard header. Hardcoding Secure:true (go:S2092) would break
// login outright for an install whose internal hop to the console is plain
// HTTP -- a real, not hypothetical, deployment shape this rule cannot see.
//
// TRUSTING the header rides the SAME boundary forwardAuthHeaders above does
// for X-Forwarded-Email and its siblings -- docs/console.md, "What this mode
// requires of the proxy": the console must be the only route to its Service,
// and the proxy must set this header from the connection it terminated
// rather than relay a client's own. Not re-verified here, for the same
// reason the identity headers are not: a second, weaker check beside an
// already-documented one is a second place for the two to drift apart.
func secureCookie(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// newSessionCookie is the ONE place either flag is set, for either the live
// cookie login mints or the cleared one logout sends -- login and logout
// used to build near-identical http.Cookie literals separately, and the
// clearing one was missing both flags entirely until go:S2092/S3330 caught
// it. One constructor means Secure's origin (always secureCookie(r), never
// a literal true) is asserted once, not re-typed at every call site.
func newSessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, Secure: secureCookie(r),
		SameSite: http.SameSiteStrictMode, MaxAge: maxAge,
	}
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
	http.SetCookie(w, newSessionCookie(r, id, int(sessionTTL/time.Second)))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.sessions.drop(c.Value)
	}
	http.SetCookie(w, newSessionCookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

// handleSession lets the SPA decide between the login form and the app without
// provoking a 401 on every load, and tells it which write affordances to render
// at all. Hiding a button the server would refuse is presentation; the server
// refuses regardless.
//
// It also reports HOW the request was authenticated, because the two modes need
// different screens: a login form on a console that accepts no token is a dead
// end, and an identity of "unknown" is the only visible sign that the fronting
// proxy forwards none — otherwise discoverable solely by attempting a write.
func (a *API) handleSession(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.admit(r)
	external := a.authIsExternal()
	mode, source := "token", ""
	if external {
		mode = "external"
	}
	switch {
	case identity == "":
	case identity == "token":
		source = "token"
	default:
		source = "forward-auth"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": ok,
		"configured":    a.token() != "",
		"identity":      identity,
		// where that identity came from: "forward-auth" (a proxy said so),
		// "token" (only the shared credential was proven), or "" (nobody said)
		"identitySource":        source,
		"authMode":              mode,
		"externalAuthenticator": a.externalAuthenticatorName(),
		"writeEnabled":          a.writesAllowed(),
		// writes need something to attribute them to; under external
		// authentication a missing identity is a read-only console, not an
		// opaque 403 at the moment of sending
		"canWrite":     a.writesAllowed() && identity != "",
		"canOriginate": a.originator != nil,
		"metrics":      a.metricsBackend() != nil,
	})
}
