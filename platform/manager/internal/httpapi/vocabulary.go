package httpapi

import (
	"context"
	"net/http"
)

// VocabularyRevisionHeader carries the current vocabulary revision on the
// outbound long-poll — on the 200 that delivers an op and on the 204 that
// delivers none.
//
// ADDITIVE AND OPTIONAL TO ACT ON. An adapter that ignores it is fully
// conformant and behaves exactly as one built before the vocabulary existed, so
// the outbound contract version does not change: nothing an existing adapter
// reads is altered, moved or removed.
const VocabularyRevisionHeader = "X-Agentops-Vocabulary-Revision"

// setVocabularyRevision stamps the current revision on a response, if there is
// a router to derive it from.
//
// Best-effort by design: a manager with no chat router still serves ops, and a
// missing header means "nothing to tell you", which is the same thing an older
// manager said.
func (s *Server) setVocabularyRevision(ctx context.Context, w http.ResponseWriter) {
	if s.Router == nil {
		return
	}
	if rev := s.Router.Vocabulary(ctx).Revision; rev != "" {
		w.Header().Set(VocabularyRevisionHeader, rev)
	}
}

// handleVocabulary serves what may be typed on a chat surface.
//
// It exists because a channel adapter is granted NO Kubernetes access: it
// cannot read a Pipeline and never will, so the manager is the only component
// that can tell it what is addressable. Authentication is the adapter auth the
// contract already defines — nothing further is required, and nothing further
// is granted.
//
// The response is UNFILTERED. Which entries a surface can express is transport
// knowledge and stays with the adapter, exactly as escaping, length limits and
// thread naming already do.
func (s *Server) handleVocabulary(w http.ResponseWriter, r *http.Request) {
	if s.Router == nil {
		writeJSON(w, 503, map[string]string{"error": "no chat router configured"})
		return
	}
	v := s.Router.Vocabulary(r.Context())
	w.Header().Set(VocabularyRevisionHeader, v.Revision)
	writeJSON(w, 200, v)
}
