package httpapi

import "net/http"

// ResetContextForTest exposes the context-reset handler to the integration
// suite, which drives handlers directly rather than through the adapter-auth
// middleware. Test-only surface; the routed path is authenticated normally.
func (s *Server) ResetContextForTest(w http.ResponseWriter, r *http.Request) {
	s.handleContextReset(w, r)
}
