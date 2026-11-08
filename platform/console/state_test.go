package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stateServer answers the manager's /state/* surface with canned bodies and
// records the bearer token each request carried.
func stateServer(t *testing.T, routes map[string]string, gotAuth *string) *stateSource {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			http.Error(w, "nope", http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/stream") {
			w.Header().Set("Content-Type", "text/event-stream")
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return newStateSource(&Config{ManagerURL: srv.URL + "/", AdapterToken: "tok"})
}

const convRecord = `{"name":"c1","id":"u1","revision":"7","createdAt":"2026-01-01T00:00:00Z","deletionRequestedAt":"2026-01-02T00:00:00Z","spec":{"a":1},"status":{"phase":"Idle"}}`

func TestStateSourceListMapsRecordsToObjects(t *testing.T) {
	var auth string
	s := stateServer(t, map[string]string{"/state/conversations": `{"revision":"9","items":[` + convRecord + `]}`}, &auth)
	objs, rev, err := s.List(context.Background(), "conversations")
	if err != nil || rev != "9" || len(objs) != 1 {
		t.Fatalf("List = %v, %q, %v", objs, rev, err)
	}
	o := objs[0]
	if o.Metadata.Name != "c1" || o.Metadata.UID != "u1" || o.Metadata.ResourceVersion != "7" ||
		o.Metadata.DeletionTimestamp != "2026-01-02T00:00:00Z" || o.Kind != "conversations" {
		t.Fatalf("mapped object = %+v", o)
	}
	if auth != "Bearer tok" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestStateSourceRefusesAKindItDoesNotServe(t *testing.T) {
	s := stateServer(t, nil, nil)
	if _, _, err := s.List(context.Background(), "pipelines"); err == nil {
		t.Fatal("List of a non-state kind must fail")
	}
	if err := s.Watch(context.Background(), "pipelines", "", nil); err == nil {
		t.Fatal("Watch of a non-state kind must fail")
	}
	if _, err := s.Get(context.Background(), "signals", "x"); err == nil {
		t.Fatal("Get of a signal must fail: no such endpoint")
	}
}

func TestStateSourceGetAndHTTPError(t *testing.T) {
	s := stateServer(t, map[string]string{"/state/conversations/c1": convRecord}, nil)
	o, err := s.Get(context.Background(), "conversations", "c1")
	if err != nil || o.Metadata.Name != "c1" {
		t.Fatalf("Get = %v, %v", o, err)
	}
	_, err = s.Get(context.Background(), "conversations", "missing")
	var ae *apiError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("missing conversation error = %v, want a 404 apiError", err)
	}
}

func TestStateSourceWatchTranslatesFrames(t *testing.T) {
	stream := ": keep-alive\n\n" +
		"event: put\nid: 11\ndata: " + convRecord + "\n\n" +
		"event: delete\nid: 12\ndata: {\"name\":\"c1\"}\n\n" +
		"event: bookmark\nid: 13\ndata: {}\n\n"
	s := stateServer(t, map[string]string{"/state/conversations/stream": stream}, nil)
	var got []string
	err := s.Watch(context.Background(), "conversations", "5", func(ev string, o *Object) {
		got = append(got, ev+":"+o.Metadata.ResourceVersion)
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "MODIFIED:11 DELETED:12 BOOKMARK:13"; strings.Join(got, " ") != want {
		t.Fatalf("events = %v, want %s", got, want)
	}
}

func TestStateSourceWatchResyncExpiresTheWatch(t *testing.T) {
	s := stateServer(t, map[string]string{"/state/signals/stream": "event: resync\nid: 1\ndata: {}\n\n"}, nil)
	err := s.Watch(context.Background(), "signals", "", func(string, *Object) {})
	if !errors.Is(err, ErrWatchExpired) {
		t.Fatalf("resync = %v, want ErrWatchExpired", err)
	}
}

func TestApplySSEFrameRejectsMalformedData(t *testing.T) {
	for _, ev := range []string{"put", "delete"} {
		err := applySSEFrame("conversations", sseFrame{event: ev, data: []byte("{")}, func(string, *Object) {})
		if err == nil {
			t.Fatalf("%s frame with bad JSON must fail", ev)
		}
	}
}

func TestScanSSEStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := scanSSE(ctx, strings.NewReader("event: put\n\n"), func(sseFrame) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("scanSSE on a cancelled context = %v", err)
	}
}

type recordingSource struct{ name string }

func (r recordingSource) List(context.Context, string) ([]*Object, string, error) {
	return []*Object{{Kind: r.name}}, r.name, nil
}
func (r recordingSource) Get(context.Context, string, string) (*Object, error) {
	return &Object{Kind: r.name}, nil
}
func (r recordingSource) Watch(_ context.Context, _, _ string, fn func(string, *Object)) error {
	fn("ADDED", &Object{Kind: r.name})
	return nil
}

func TestCompositeSourceRoutesStateKindsToTheManager(t *testing.T) {
	c := newCompositeSource(recordingSource{"kube"}, recordingSource{"state"})
	for kind, want := range map[string]string{"conversations": "state", "signals": "state", "pipelines": "kube"} {
		_, rev, _ := c.List(context.Background(), kind)
		o, _ := c.Get(context.Background(), kind, "x")
		var watched string
		_ = c.Watch(context.Background(), kind, "", func(_ string, o *Object) { watched = o.Kind })
		if rev != want || o.Kind != want || watched != want {
			t.Fatalf("%s routed to list=%s get=%s watch=%s, want %s", kind, rev, o.Kind, watched, want)
		}
	}
}
