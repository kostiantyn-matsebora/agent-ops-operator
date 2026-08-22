package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

// The work contract passes through here, and that is where the access decision
// comes from.
//
// The proxy is ALREADY forwarding this stream — every connection out of the pod
// is redirected to it — so reading the allowlist off a work unit costs no
// credential, no API call and no second source of truth. What the runtime is
// configured with and what this process enforces are then literally the same
// message. See design decision D3.
//
// Nothing is modified on this path. The work unit reaches the runtime exactly
// as the manager sent it.

// serveControl forwards the work contract, reading each work unit on the way.
func serveControl(cr *bufio.Reader, client io.Writer, upstream io.ReadWriter, state *policy) {
	ur := bufio.NewReader(upstream)

	for {
		req, err := http.ReadRequest(cr)
		if err != nil {
			return
		}
		path := req.URL.Path
		if err := req.Write(upstream); err != nil {
			return
		}
		resp, err := http.ReadResponse(ur, req)
		if err != nil {
			return
		}

		// Only /work carries a unit. Everything else — /work/done, the context
		// report, the long-poll's empty 204 — is forwarded untouched.
		if isWorkPath(path) && resp.StatusCode == http.StatusOK {
			body, tooBig, rerr := readBounded(resp.Body)
			if rerr != nil {
				return
			}
			if !tooBig {
				learn(body, state)
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))
			resp.ContentLength = int64(len(body))
		}
		if err := resp.Write(client); err != nil {
			return
		}
		if !keepAlive(req) {
			return
		}
	}
}

// learn records the access decision a work unit carries.
//
// The unit's `allowedTools` is the WIRING's contribution — the bound toolsets,
// deduped in ref order. It is deliberately NOT the final list the CLI applies,
// which also folds in the agent definition's own `tools:` frontmatter.
//
// That difference is the point, not an omission. The definition lives in the
// repository the agent checks out and can write to, so treating it as an input
// to enforcement would let the enforced set be edited by the party being
// enforced against. Enforcement uses the half that comes from wiring. See
// design decision D4.
func learn(body []byte, state *policy) {
	var unit struct {
		AllowedTools string `json:"allowedTools"`
		ToolsMode    string `json:"toolsMode"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &unit) != nil {
		return
	}
	tools := splitTools(unit.AllowedTools)
	state.set(tools)
	log.Printf("work unit: enforcing %d granted tool pattern(s)", len(tools))
}

// isWorkPath matches the work long-poll and nothing adjacent to it. /work/done
// and /work/context report RESULTS and carry no grant.
func isWorkPath(path string) bool {
	return path == "/work"
}

func splitTools(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
