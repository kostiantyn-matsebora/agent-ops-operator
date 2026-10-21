package controlplane

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// readJSON decodes a request body up to 1MiB, the same ceiling every other
// handler in this process applies. An EMPTY body decodes as the zero value
// rather than an error — handlers that require a field (Key, on PUT) reject
// its absence explicitly, rather than this helper guessing whether a bare
// PUT was meant as a request for defaults.
func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}
