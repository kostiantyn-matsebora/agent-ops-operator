// kube_split.go's own tests (split-conversation-namespace task 9.8): a pod
// from EITHER namespace must reach the Infrastructure view, and every other
// kind must stay control-namespace only. Driven against two real
// httptest.Server fakes — one standing in for the control namespace, one for
// the conversations namespace — never a hand-rolled apiSource mock: List,
// Get and Watch all go through the real Kube client this change did not
// touch, only wrapped.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// podListServer answers every List/Get/Watch request with a fixed list body
// (and the rv given), regardless of kind or namespace — the fake is
// distinguished from its sibling by WHICH SERVER a request reaches, exactly
// as podsAcrossNamespaces itself distinguishes the two by which *Kube it
// calls, never by inspecting a namespace string.
func podListServer(t *testing.T, names []string, rv string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") == "1" {
			w.(http.Flusher).Flush()
			<-r.Context().Done() // block until the test cancels, like a real watch with nothing to say
			return
		}
		var items []string
		for _, n := range names {
			items = append(items, fmt.Sprintf(`{"metadata":{"name":%q}}`, n))
		}
		fmt.Fprintf(w, `{"metadata":{"resourceVersion":%q},"items":[%s]}`, rv, strings.Join(items, ","))
	}))
}

func kubeFor(srv *httptest.Server) *Kube {
	return &Kube{BaseURL: srv.URL, Namespace: "irrelevant-to-the-fake", HTTP: srv.Client()}
}

// unreachableKube points at a closed listener — any request against it fails
// at the transport, which is how a test proves a code path NEVER calls it.
func unreachableKube(t *testing.T) *Kube {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed before any request can land
	return &Kube{BaseURL: srv.URL, Namespace: "ns", HTTP: srv.Client()}
}

// TestPodsAcrossNamespacesListMergesPodsFromBoth pins the Infrastructure
// view's own contract: a pod from the control namespace AND a pod from the
// conversations namespace both come back from one List("pods") call.
func TestPodsAcrossNamespacesListMergesPodsFromBoth(t *testing.T) {
	control := podListServer(t, []string{"control-pod-1"}, "10")
	defer control.Close()
	conversations := podListServer(t, []string{"conv-pod-1"}, "20")
	defer conversations.Close()

	p := newPodsAcrossNamespaces(kubeFor(control), kubeFor(conversations))
	objs, rv, err := p.List(context.Background(), "pods")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, o := range objs {
		names = append(names, o.Metadata.Name)
	}
	if len(names) != 2 || !contains(names, "control-pod-1") || !contains(names, "conv-pod-1") {
		t.Fatalf("want one pod from each namespace, got %v", names)
	}
	// the OLDER resourceVersion is the safe resume point for both watches
	if rv != "10" {
		t.Fatalf("rv = %q, want the older of the two (%q)", rv, "10")
	}
}

// TestPodsAcrossNamespacesListNonPodKindStaysControlOnly pins the "NOT a
// general watch-everywhere mechanism" half of the design: every other kind
// never reaches the conversations namespace at all.
func TestPodsAcrossNamespacesListNonPodKindStaysControlOnly(t *testing.T) {
	control := podListServer(t, []string{"pipe-a"}, "5")
	defer control.Close()

	p := newPodsAcrossNamespaces(kubeFor(control), unreachableKube(t))
	objs, _, err := p.List(context.Background(), "pipelines")
	if err != nil {
		t.Fatalf("List: %v (the conversations client must never be dialed for a non-pod kind)", err)
	}
	if len(objs) != 1 || objs[0].Metadata.Name != "pipe-a" {
		t.Fatalf("want the control namespace's own list, got %v", objs)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// objectServer answers Get requests: 404 for every name except one, which it
// serves with the given body.
func objectServer(t *testing.T, foundName, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if name != foundName {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "not found")
			return
		}
		_, _ = io.WriteString(w, body)
	}))
}

// TestPodsAcrossNamespacesGetFallsBackToConversationsOn404 pins Get's own
// "not found here, maybe found there" shape: a pod absent from the control
// namespace's list is still reachable by name, from the conversations one.
func TestPodsAcrossNamespacesGetFallsBackToConversationsOn404(t *testing.T) {
	control := objectServer(t, "control-only", `{"metadata":{"name":"control-only"}}`)
	defer control.Close()
	conversations := objectServer(t, "conv-pod", `{"metadata":{"name":"conv-pod"}}`)
	defer conversations.Close()

	p := newPodsAcrossNamespaces(kubeFor(control), kubeFor(conversations))
	obj, err := p.Get(context.Background(), "pods", "conv-pod")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if obj.Metadata.Name != "conv-pod" {
		t.Fatalf("got %+v, want the conversations-namespace object", obj)
	}
}

// TestPodsAcrossNamespacesGetFoundInControlNeverTriesConversations: the
// common case — most pods, and every other kind's every object, live in the
// control namespace — must not pay for a second round trip.
func TestPodsAcrossNamespacesGetFoundInControlNeverTriesConversations(t *testing.T) {
	control := objectServer(t, "control-pod", `{"metadata":{"name":"control-pod"}}`)
	defer control.Close()

	p := newPodsAcrossNamespaces(kubeFor(control), unreachableKube(t))
	obj, err := p.Get(context.Background(), "pods", "control-pod")
	if err != nil {
		t.Fatalf("Get: %v (must not have dialed the conversations namespace)", err)
	}
	if obj.Metadata.Name != "control-pod" {
		t.Fatalf("got %+v", obj)
	}
}

// TestPodsAcrossNamespacesGetNonPodKindNeverFallsBack: a 404 on a non-pod
// kind is final — there is no second namespace for a wiring object to live
// in, so Get must return the error rather than ever dialing conversations.
func TestPodsAcrossNamespacesGetNonPodKindNeverFallsBack(t *testing.T) {
	control := objectServer(t, "only-this-one", `{"metadata":{"name":"only-this-one"}}`)
	defer control.Close()

	p := newPodsAcrossNamespaces(kubeFor(control), unreachableKube(t))
	_, err := p.Get(context.Background(), "pipelines", "missing")
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusNotFound {
		t.Fatalf("want a plain 404 from the control namespace alone, got %v", err)
	}
}

// watchFrameServer streams exactly one JSON frame then blocks until the
// request's context ends — the same "deliver, then wait" shape a real watch
// with no further changes has.
func watchFrameServer(t *testing.T, frame string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, frame+"\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
}

// TestPodsAcrossNamespacesWatchForwardsEventsFromBothNamespaces pins the
// merge at the streaming layer, not just List: an event from EITHER
// namespace's watch reaches the caller's callback untouched.
func TestPodsAcrossNamespacesWatchForwardsEventsFromBothNamespaces(t *testing.T) {
	control := watchFrameServer(t, `{"type":"ADDED","object":{"metadata":{"name":"control-pod"}}}`)
	defer control.Close()
	conversations := watchFrameServer(t, `{"type":"ADDED","object":{"metadata":{"name":"conv-pod"}}}`)
	defer conversations.Close()

	p := newPodsAcrossNamespaces(kubeFor(control), kubeFor(conversations))
	ctx, cancel := context.WithCancel(context.Background())

	var mu sync.Mutex
	var seen []string
	done := make(chan error, 1)
	go func() {
		done <- p.Watch(ctx, "pods", "1", func(eventType string, obj *Object) {
			mu.Lock()
			seen = append(seen, obj.Metadata.Name)
			mu.Unlock()
		})
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("did not observe both namespaces' events in time, got %v", seen)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch: %v", err)
	}
	if !contains(seen, "control-pod") || !contains(seen, "conv-pod") {
		t.Fatalf("want an event from each namespace, got %v", seen)
	}
}

// TestPodsAcrossNamespacesWatchEndsThePairWhenOneSideExpires: ErrWatchExpired
// from either side must win over the other side's context.Canceled — the
// pair resumes together, from one relist, never half-resumed.
func TestPodsAcrossNamespacesWatchEndsThePairWhenOneSideExpires(t *testing.T) {
	expired := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer expired.Close()
	blocking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer blocking.Close()

	p := newPodsAcrossNamespaces(kubeFor(expired), kubeFor(blocking))
	err := p.Watch(context.Background(), "pods", "1", func(string, *Object) {})
	if !errors.Is(err, ErrWatchExpired) {
		t.Fatalf("want ErrWatchExpired to win over the other side's cancellation, got %v", err)
	}
}

// TestPodsAcrossNamespacesWatchNonPodKindUsesControlAlone: the merge
// mechanism is pods-only — every other kind's Watch must never touch the
// conversations namespace.
func TestPodsAcrossNamespacesWatchNonPodKindUsesControlAlone(t *testing.T) {
	control := watchFrameServer(t, `{"type":"ADDED","object":{"metadata":{"name":"pipe-a"}}}`)
	defer control.Close()

	p := newPodsAcrossNamespaces(kubeFor(control), unreachableKube(t))
	ctx, cancel := context.WithCancel(context.Background())
	seen := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- p.Watch(ctx, "pipelines", "1", func(eventType string, obj *Object) {
			seen <- obj.Metadata.Name
		})
	}()
	select {
	case name := <-seen:
		if name != "pipe-a" {
			t.Fatalf("got %q", name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never observed the control namespace's event")
	}
	cancel()
	<-done
}

// olderResourceVersion/parseResourceVersion are pure functions pinned
// directly: List's safety argument rests on always picking the smaller
// (older) of the two values, and on "" or a garbled value losing to a real
// one rather than winning by accident.
func TestOlderResourceVersion(t *testing.T) {
	cases := []struct {
		a, b, want string
	}{
		{"10", "20", "10"},
		{"20", "10", "10"},
		{"10", "10", "10"},
		{"", "20", "20"},
		{"10", "", "10"},
		{"", "", ""},
		{"garbled", "20", "20"},
		{"10", "garbled", "10"},
	}
	for _, tc := range cases {
		if got := olderResourceVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("olderResourceVersion(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
