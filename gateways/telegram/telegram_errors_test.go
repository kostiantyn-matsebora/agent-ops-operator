package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAPIReturnsTelegramError closes the gap on API()'s !out.OK branch: none
// of the existing tests ever had the fake Bot API reject a call. The error
// must surface Telegram's own description, since that is the only diagnostic
// an operator watching logs gets.
func TestAPIReturnsTelegramError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"Forbidden: bot was blocked by the user"}`))
	}))
	defer srv.Close()

	tg := &Telegram{Token: "t", HTTP: srv.Client(), BaseURL: srv.URL}
	_, err := tg.API(context.Background(), "sendMessage", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("expected Telegram's description in the error, got %v", err)
	}
}

// TestAPIReturnsErrorWhenTheHostIsUnreachable closes the gap on API()'s
// HTTP.Do error branch, using a real closed listener rather than a mock.
func TestAPIReturnsErrorWhenTheHostIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	url := srv.URL
	srv.Close() // now genuinely unreachable

	tg := &Telegram{Token: "t", HTTP: client, BaseURL: url}
	if _, err := tg.API(context.Background(), "getUpdates", map[string]any{}); err == nil {
		t.Fatal("expected an error when the Bot API host is unreachable")
	}
}

// TestAPIReturnsErrorOnMalformedResponse closes the gap on API()'s JSON
// decode-error branch.
func TestAPIReturnsErrorOnMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	tg := &Telegram{Token: "t", HTTP: srv.Client(), BaseURL: srv.URL}
	if _, err := tg.API(context.Background(), "getUpdates", map[string]any{}); err == nil {
		t.Fatal("expected a decode error for a malformed Bot API response")
	}
}

// TestGetUpdatesReturnsErrorOnMalformedResult closes the gap on GetUpdates'
// own json.Unmarshal of the result field: a non-array result must surface an
// error rather than silently returning zero updates.
func TestGetUpdatesReturnsErrorOnMalformedResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":"not-an-array"}`))
	}))
	defer srv.Close()

	tg := &Telegram{Token: "t", HTTP: srv.Client(), BaseURL: srv.URL}
	if _, err := tg.GetUpdates(context.Background(), 0); err == nil {
		t.Fatal("expected an error when the result is not an array of updates")
	}
}

// TestGetUpdatesReturnsErrorWhenAnUpdateFailsToClassify closes the gap on
// GetUpdates' propagation of classifyUpdate's own error — a single malformed
// update in an otherwise valid batch must fail the whole call rather than be
// silently dropped, since offset advancement is per-batch.
func TestGetUpdatesReturnsErrorWhenAnUpdateFailsToClassify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":"not-a-number"}]}`))
	}))
	defer srv.Close()

	tg := &Telegram{Token: "t", HTTP: srv.Client(), BaseURL: srv.URL}
	if _, err := tg.GetUpdates(context.Background(), 0); err == nil {
		t.Fatal("expected classifyUpdate's error to propagate from GetUpdates")
	}
}

// TestClassifyUpdateReturnsErrorOnMalformedJSON closes the gap on
// classifyUpdate's own json.Unmarshal error branch, called directly rather
// than through GetUpdates.
func TestClassifyUpdateReturnsErrorOnMalformedJSON(t *testing.T) {
	if _, err := classifyUpdate([]byte(`{not-json`)); err == nil {
		t.Fatal("expected an error for malformed update JSON")
	}
}
