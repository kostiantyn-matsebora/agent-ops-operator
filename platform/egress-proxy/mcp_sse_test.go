package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// streamFiltered, writeResponseHead and cutPrefix were entirely 0%: every
// other forwardResponse test uses an application/json body, never the
// streamable-HTTP (text/event-stream) transport, which is the one case
// forwardResponse must NOT buffer. This drives forwardResponse with a real
// SSE body containing two `data:` frames — one that filtering leaves
// unchanged and one it must rewrite — plus a bare keep-alive line with no
// "data:" prefix, so cutPrefix's false branch is exercised too.
func TestSSEToolListingIsFilteredFrameByFrame(t *testing.T) {
	allGranted := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"pods_list"}]}}`
	partiallyDenied := `{"jsonrpc":"2.0","id":2,"result":{"tools":[` +
		`{"name":"pods_list"},{"name":"pods_delete"}]}}`
	body := "data: " + allGranted + "\n\n" + // unchanged: nothing to filter
		": keep-alive\n\n" + // not a data frame at all — cutPrefix must say so
		"data: " + partiallyDenied + "\n\n" // must be rewritten

	resp := &http.Response{
		StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
	state := newPolicy()
	state.set([]string{"mcp__kubernetes__pods_list"})

	var out bytes.Buffer
	if err := forwardResponse(&out, resp, "kubernetes", state); err != nil {
		t.Fatalf("forwardResponse: %v", err)
	}
	got := out.String()

	if !strings.HasPrefix(got, "HTTP/1.1 200 OK\r\n") {
		t.Fatalf("writeResponseHead did not write the status line: %q", got)
	}
	if !strings.Contains(got, ": keep-alive") {
		t.Fatalf("a non-data SSE line must pass through untouched: %s", got)
	}
	if !strings.Contains(got, `"id":1`) || !strings.Contains(got, "pods_list") {
		t.Fatalf("the unfiltered frame must still be present: %s", got)
	}
	if !strings.Contains(got, `"id":2`) {
		t.Fatalf("the filtered frame must still be present: %s", got)
	}
	if strings.Contains(got, "pods_delete") {
		t.Fatalf("the ungranted tool leaked through the SSE stream: %s", got)
	}
}
