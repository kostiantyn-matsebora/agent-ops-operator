package controlplane

import (
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RegisterRoutes wires the whole /control-plane/* surface onto mux, every
// route guarded by Auth(token, ...). ControlPlaneProbe is the one kind with
// a handler today; a future resident gets its own route pair, registered
// here beside it, following the same Auth-wrapped shape.
//
// Go's http.ServeMux (1.22+) resolves the most specific registered pattern,
// so "/control-plane/probes/{name}" is preferred over the catch-all below
// for any request it matches. Any OTHER path under /control-plane/ — a kind
// this package does not yet handle, or a method neither probes route
// accepts — falls through to unknownKind, which answers 404 without
// touching any object: a request for an unhandled kind must find nothing,
// exactly as if the route did not exist, never partially create, read or
// modify a different kind's data.
func RegisterRoutes(mux *http.ServeMux, token string, writer client.Writer, reader client.Reader, namespace string) {
	probes := &ProbeHandler{Writer: writer, Reader: reader, Namespace: namespace}
	mux.HandleFunc("PUT /control-plane/probes/{name}", Auth(token, probes.Put))
	mux.HandleFunc("GET /control-plane/probes/{name}", Auth(token, probes.Get))
	mux.HandleFunc("/control-plane/", Auth(token, unknownKind))
}

func unknownKind(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no control-plane handler for this route"})
}
