package controlplane

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Auth is the constant-time bearer-token middleware guarding every
// /control-plane/* route. token is CONTROL_PLANE_TOKEN, read ONCE by the
// caller at process start and passed in here as a plain argument — this
// package never reads the environment itself, the same way
// httpapi.Server's AdapterToken field is threaded in rather than read with
// os.Getenv inside handler logic.
//
// An empty token means the surface is unconfigured. It refuses every
// request with 503 rather than falling open, mirroring httpapi's
// adapterAuth/signalAuth and the documented rule in
// platform/console/auth.go: an unconfigured token authorizes nobody, and is
// indistinguishable from a wrong one — "no token set" must never read as
// "no authentication required".
//
// A non-empty, non-matching token is refused with 401. The comparison is
// constant-time (crypto/subtle), the same precaution every other
// bearer-token surface in this process takes.
//
// CONTROL_PLANE_TOKEN is deliberately a single flat shared secret (not
// per-caller, not scoped, not derived — design.md), and it is a DISTINCT
// credential from ADAPTER_TOKEN and the coordinator token: it is never
// derived from either, and neither is accepted here. A caller scoped to one
// surface must not reach another.
func Auth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "control-plane auth not configured"})
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}
