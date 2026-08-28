package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func call(t *testing.T, srv *httptest.Server, method string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/bot123:token/"+method, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.Header.Get("X-Agentops-Fake-Bot-Api") != "true" {
		t.Fatalf("every response must announce the fake")
	}
	return resp.StatusCode, out
}

// The fake answers each method in the shape the real API does — the fields
// the two Telegram components actually read. A fake whose shapes drift from
// the captured ones tests the components against a Telegram that does not
// exist.
func TestResponseShapesMatchTheRealOnes(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	defer srv.Close()

	code, out := call(t, srv, "createForumTopic", map[string]any{"chat_id": "-1001234567890", "name": "topic"})
	if code != 200 || out["ok"] != true {
		t.Fatalf("createForumTopic: %d %v", code, out)
	}
	if res := out["result"].(map[string]any); res["message_thread_id"].(float64) != 1 || res["name"] != "topic" {
		t.Fatalf("ForumTopic shape: %v", res)
	}
	_, out = call(t, srv, "sendMessage", map[string]any{"chat_id": "-1001234567890", "text": "hi", "message_thread_id": 1, "parse_mode": "HTML"})
	res := out["result"].(map[string]any)
	if res["message_id"].(float64) != 1 || res["message_thread_id"].(float64) != 1 || res["text"] != "hi" {
		t.Fatalf("Message shape: %v", res)
	}
	_, out = call(t, srv, "closeForumTopic", map[string]any{"chat_id": "-1001234567890", "message_thread_id": 1})
	if out["result"] != true {
		t.Fatalf("closeForumTopic must return true: %v", out)
	}

	// sendDocument is multipart, and the document's name is what matters.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("chat_id", "-1001234567890")
	_ = w.WriteField("caption", "c")
	part, _ := w.CreateFormFile("document", "report.md")
	_, _ = part.Write([]byte("# hi"))
	_ = w.Close()
	resp, err := http.Post(srv.URL+"/bot123:token/sendDocument", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&doc)
	resp.Body.Close()
	if doc["ok"] != true || doc["result"].(map[string]any)["document"].(map[string]any)["file_name"] != "report.md" {
		t.Fatalf("sendDocument shape: %v", doc)
	}

	// Every call is on record, with its body, queryable by method.
	r, _ := http.Get(srv.URL + "/control/calls?method=sendDocument")
	var calls []Call
	_ = json.NewDecoder(r.Body).Decode(&calls)
	r.Body.Close()
	if len(calls) != 1 || calls[0].Document == nil || calls[0].Document.Filename != "report.md" || calls[0].Body["caption"] != "c" {
		t.Fatalf("recorded sendDocument: %+v", calls)
	}
}

// getUpdates serves each fed update ONCE under Telegram's offset protocol,
// and a fixture replayed through it is byte-identical to what was fed —
// which is the whole reason the double is faithful.
func TestGetUpdatesOffsetProtocolAndVerbatimReplay(t *testing.T) {
	fixture, err := os.ReadFile("../fixtures/telegram-update-message.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New().Handler())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/control/updates", "application/json", bytes.NewReader(fixture))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("feed: %v %v", err, resp)
	}
	resp.Body.Close()

	_, out := call(t, srv, "getUpdates", map[string]any{"offset": 0, "timeout": 1})
	updates := out["result"].([]any)
	if len(updates) != 1 {
		t.Fatalf("one queued update, got %v", updates)
	}
	got, _ := json.Marshal(updates[0])
	var want, have any
	_ = json.Unmarshal(fixture, &want)
	_ = json.Unmarshal(got, &have)
	wb, _ := json.Marshal(want)
	if string(got) != string(wb) {
		t.Fatalf("replayed update must be byte-identical to the fixture:\n%s\n%s", got, wb)
	}
	// Not yet confirmed: the same offset serves it again, as Telegram does.
	_, out = call(t, srv, "getUpdates", map[string]any{"offset": 0, "timeout": 0})
	if len(out["result"].([]any)) != 1 {
		t.Fatalf("an unconfirmed update is served again")
	}
	// Confirmed by offset = last+1: gone.
	_, out = call(t, srv, "getUpdates", map[string]any{"offset": 78, "timeout": 0})
	if len(out["result"].([]any)) != 0 {
		t.Fatalf("a confirmed update must not be served again: %v", out)
	}
}

// Telegram serves ONE getUpdates stream per token and answers the second with
// 409. So does the fake — it is how the pack SEES the single-consumer rule
// broken instead of assuming it held.
func TestSecondConcurrentPollerGets409(t *testing.T) {
	srv := httptest.NewServer(New().Handler())
	defer srv.Close()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		call(t, srv, "getUpdates", map[string]any{"offset": 0, "timeout": 2})
	}()
	time.Sleep(200 * time.Millisecond)
	code, out := call(t, srv, "getUpdates", map[string]any{"offset": 0, "timeout": 0})
	if code != 409 || !strings.Contains(out["description"].(string), "Conflict") {
		t.Fatalf("second poller must get Telegram's 409: %d %v", code, out)
	}
	wg.Wait()
	r, _ := http.Get(srv.URL + "/control/consumers")
	var c map[string]int
	_ = json.NewDecoder(r.Body).Decode(&c)
	r.Body.Close()
	if c["conflicts"] != 1 || c["maxConcurrent"] != 1 || c["active"] != 0 {
		t.Fatalf("consumers = %v", c)
	}
}
