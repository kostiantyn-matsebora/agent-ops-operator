package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// MCP mediation.
//
// Two message kinds matter and nothing else is touched:
//
//	tools/call  refused before it reaches the server, if the wiring did not
//	            grant that tool
//	tools/list  filtered on the way back, so what is advertised is what is
//	            callable
//
// A refusal is an MCP-level error, never a dropped connection. An agent that
// receives a transport failure retries blindly, and a retry loop against a wall
// is indistinguishable from a broken network — the agent has to be TOLD it is
// not allowed so it can report that instead.

// maxMessage bounds a body read into memory. MCP messages are small. A body
// larger than this is streamed through unparsed rather than buffered, because
// an enforcement proxy that can be made to hold a gigabyte is a denial of
// service against its own pod.
const maxMessage = 1 << 20

// serveMCP mediates one connection to a bound MCP endpoint.
func serveMCP(cr *bufio.Reader, client io.Writer, upstream io.ReadWriter, endpointKey string, state *policy) {
	ur := bufio.NewReader(upstream)

	for {
		req, err := http.ReadRequest(cr)
		if err != nil {
			return
		}
		body, tooBig, err := readBounded(req.Body)
		if err != nil {
			return
		}

		if !tooBig {
			if names, id, ok := callTargets(body); ok {
				if denied, found := firstDenied(names, endpointKey, state); found {
					// Refused HERE. The server never sees it, which is the
					// difference between an allowlist and a suggestion.
					writeRefusal(client, req, id, denied, state.ready())
					if !keepAlive(req) {
						return
					}
					continue
				}
			}
		}

		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		if err := req.Write(upstream); err != nil {
			return
		}
		resp, err := http.ReadResponse(ur, req)
		if err != nil {
			return
		}
		if err := forwardResponse(client, resp, endpointKey, state); err != nil {
			return
		}
		if !keepAlive(req) {
			return
		}
	}
}

// forwardResponse passes a server response back, filtering tool listings so
// discovery cannot advertise what invocation would refuse.
func forwardResponse(client io.Writer, resp *http.Response, endpointKey string, state *policy) error {
	ct := resp.Header.Get("Content-Type")

	// An event stream is the streamable-HTTP transport. It must keep streaming
	// — buffering it would stall every long-running tool call — so it is
	// filtered line by line as it passes.
	if strings.HasPrefix(ct, "text/event-stream") {
		return streamFiltered(client, resp, endpointKey, state)
	}

	body, tooBig, err := readBounded(resp.Body)
	if err != nil {
		return err
	}
	if !tooBig {
		if filtered, changed := filterListing(body, endpointKey, state); changed {
			body = filtered
			resp.Header.Set("Content-Length", fmt.Sprint(len(body)))
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp.Write(client)
}

// streamFiltered rewrites `data:` payloads in an SSE stream as they pass.
func streamFiltered(client io.Writer, resp *http.Response, endpointKey string, state *policy) error {
	head := *resp
	head.Body = nil
	if err := writeResponseHead(client, resp); err != nil {
		return err
	}
	br := bufio.NewReader(resp.Body)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			out := line
			if payload, ok := cutPrefix(line, "data:"); ok {
				if filtered, changed := filterListing(bytes.TrimSpace(payload), endpointKey, state); changed {
					out = append([]byte("data: "), append(filtered, '\n')...)
				}
			}
			if _, werr := client.Write(out); werr != nil {
				return werr
			}
			if f, ok := client.(interface{ Flush() error }); ok {
				_ = f.Flush()
			}
		}
		if err != nil {
			return nil
		}
	}
}

// callTargets reports EVERY tool a message invokes.
//
// Every one, not the first: a batch is judged as a whole, so a batch whose
// second member calls an ungranted tool is refused even though its first is
// fine. Checking only the first would let any denied call through behind a
// permitted one, which is a bypass with a two-line payload.
//
// Splitting the batch instead — answering the granted half — would hand back a
// shape the client never asked for.
func callTargets(body []byte) (tools []string, id json.RawMessage, ok bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil, false
	}
	type rpc struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if trimmed[0] == '[' {
		var batch []rpc
		if json.Unmarshal(trimmed, &batch) != nil {
			return nil, nil, false
		}
		var names []string
		var first json.RawMessage
		for _, m := range batch {
			if m.Method == "tools/call" && m.Params.Name != "" {
				if first == nil {
					first = m.ID
				}
				names = append(names, m.Params.Name)
			}
		}
		return names, first, len(names) > 0
	}
	var m rpc
	if json.Unmarshal(trimmed, &m) != nil {
		return nil, nil, false
	}
	if m.Method != "tools/call" || m.Params.Name == "" {
		return nil, nil, false
	}
	return []string{m.Params.Name}, m.ID, true
}

// firstDenied returns the first tool the wiring does not grant, qualified.
func firstDenied(names []string, endpointKey string, state *policy) (string, bool) {
	for _, n := range names {
		q := qualify(endpointKey, n)
		if !state.permits(q) {
			return q, true
		}
	}
	return "", false
}

// filterListing removes ungranted tools from a tools/list result.
//
// Discovery and invocation share `state.permits`, so the two cannot disagree:
// a tool that would be refused is never advertised, and an agent is not led
// into a refusal it could have been spared.
func filterListing(body []byte, endpointKey string, state *policy) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return body, false
	}
	var msg map[string]json.RawMessage
	if json.Unmarshal(trimmed, &msg) != nil {
		return body, false
	}
	raw, ok := msg["result"]
	if !ok {
		return body, false
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil {
		return body, false
	}
	toolsRaw, ok := result["tools"]
	if !ok {
		return body, false
	}
	var tools []map[string]json.RawMessage
	if json.Unmarshal(toolsRaw, &tools) != nil {
		return body, false
	}

	kept := make([]map[string]json.RawMessage, 0, len(tools))
	for _, t := range tools {
		var name string
		if json.Unmarshal(t["name"], &name) != nil {
			continue
		}
		if state.permits(qualify(endpointKey, name)) {
			kept = append(kept, t)
		}
	}
	if len(kept) == len(tools) {
		return body, false
	}
	newTools, err := json.Marshal(kept)
	if err != nil {
		return body, false
	}
	result["tools"] = newTools
	newResult, err := json.Marshal(result)
	if err != nil {
		return body, false
	}
	msg["result"] = newResult
	out, err := json.Marshal(msg)
	if err != nil {
		return body, false
	}
	// The listing is where a grant stops being theoretical: the server has just
	// named every tool it registers, so this is the one moment the proxy can
	// say what the wiring actually resolved to.
	log.Printf("tool listing for %q: %d of %d granted", endpointKey, len(kept), len(tools))
	if len(kept) == 0 {
		// A binding that grants NOTHING against a server that offers tools is
		// almost always a typo in a toolset pattern — `mcp__kubernets__*`. It
		// otherwise presents as an agent that mysteriously cannot do anything,
		// which is a long way from the toolset that caused it.
		log.Printf("WARNING: no tool on %q is granted; check the bound toolsets' patterns "+
			"against the server key %q", endpointKey, endpointKey)
	}
	return out, true
}

// writeRefusal answers the agent directly, in its own protocol.
//
// The two refusals are told apart on purpose. "Not granted" is a wiring answer
// an operator can act on. "No work unit seen yet" is the fail-closed initial
// state, and reporting it as a denial of the tool would send someone editing a
// toolset that was never the problem.
func writeRefusal(client io.Writer, req *http.Request, id json.RawMessage, qualified string, ready bool) {
	reason := fmt.Sprintf("tool %q is not granted by this conversation's wiring", qualified)
	if !ready {
		reason = fmt.Sprintf("tool %q refused: no work unit has been dispatched yet, so nothing is granted", qualified)
	}
	if id == nil {
		id = json.RawMessage("null")
	}
	payload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			// -32601 is JSON-RPC "method not found". An MCP client reports it
			// as a tool that is not available, which is exactly what happened.
			"code":    -32601,
			"message": reason,
		},
	})
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Request:       req,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(payload)),
		ContentLength: int64(len(payload)),
	}
	_ = resp.Write(client)
	log.Printf("refused %s", qualified)
}

// readBounded reads a body up to maxMessage, reporting whether it overflowed.
// An overflowing body is returned whole so it can still be forwarded — it is
// not parsed, which is the honest outcome for a message this component cannot
// hold.
func readBounded(r io.Reader) ([]byte, bool, error) {
	if r == nil {
		return nil, false, nil
	}
	buf, err := io.ReadAll(io.LimitReader(r, maxMessage+1))
	if err != nil {
		return nil, false, err
	}
	return buf, len(buf) > maxMessage, nil
}

func keepAlive(req *http.Request) bool {
	return req.ProtoMajor == 1 && req.ProtoMinor >= 1 &&
		!strings.EqualFold(req.Header.Get("Connection"), "close")
}

func writeResponseHead(w io.Writer, resp *http.Response) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "HTTP/%d.%d %03d %s\r\n", resp.ProtoMajor, resp.ProtoMinor,
		resp.StatusCode, http.StatusText(resp.StatusCode))
	if err := resp.Header.Write(&b); err != nil {
		return err
	}
	b.WriteString("\r\n")
	_, err := w.Write(b.Bytes())
	return err
}

func cutPrefix(line []byte, prefix string) ([]byte, bool) {
	if bytes.HasPrefix(line, []byte(prefix)) {
		return line[len(prefix):], true
	}
	return nil, false
}
