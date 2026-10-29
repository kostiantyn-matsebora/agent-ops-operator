package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// Publication (signal-persistence D-2, D-8, D-9): /signal/inbound validates,
// applies attributeFilters, stores one Signal per admitted signal and
// acknowledges. Nothing is routed at publish.

func respBody(t *testing.T, s *Server, source string, signals ...map[string]any) (int, map[string]any) {
	t.Helper()
	rec := postSig(s, source, signals)
	out := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, rec.Body.String())
	}
	return rec.Code, out
}

func TestPublishStoresASignalOwnedBySourceAndRoutesNothing(t *testing.T) {
	// No Pipeline claims the source: an unclaimed signal is STILL stored and
	// acknowledged — the drop is the consumer's decision, not publication's.
	source := sigTestSource("pub-src", "pub-adapter")
	s, c := sigServer(t, source)

	code, resp := respBody(t, s, source.Name, map[string]any{
		"initiationId": "pub-1", "kind": "alert", "title": "disk", "severity": "warning",
		"payload": "disk 91%", "labels": map[string]string{"team": "ops"},
	})
	if code != 200 || resp["queued"] != float64(1) {
		t.Fatalf("want 200 queued=1, got %d %v", code, resp)
	}
	if _, has := resp["conversations"]; has {
		t.Fatalf("the response must no longer carry conversations: %v", resp)
	}
	if _, has := resp["reason"]; has {
		t.Fatalf("an unrouted-at-publish signal carries no reason: %v", resp)
	}

	sigs := listSignals(t, c)
	if len(sigs) != 1 {
		t.Fatalf("want 1 Signal, got %d", len(sigs))
	}
	got := sigs[0]
	if got.Spec.SourceRef.Name != source.Name || got.Spec.Adapter != "pub-adapter" ||
		got.Spec.InitiationID != "pub-1" || got.Spec.Payload != "disk 91%" ||
		got.Spec.Title != "disk" || got.Spec.Severity != agentopsv1alpha1.SeverityWarning {
		t.Fatalf("spec not stored verbatim: %+v", got.Spec)
	}
	if got.Status.Phase != "" {
		t.Fatalf("publication must leave the phase absent (Pending), got %q", got.Status.Phase)
	}
	if !strings.HasPrefix(got.Name, agentopsv1alpha1.SignalNamePrefix) {
		t.Fatalf("name %q lacks the sig- prefix", got.Name)
	}
	if len(got.OwnerReferences) != 1 || got.OwnerReferences[0].Kind != "SignalSource" || got.OwnerReferences[0].Name != source.Name {
		t.Fatalf("the Signal must be owned by its SignalSource: %+v", got.OwnerReferences)
	}

	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil || len(convs.Items) != 0 {
		t.Fatalf("publication must create no conversation: %v %d", err, len(convs.Items))
	}
	var src agentopsv1alpha1.SignalSource
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigTestNamespace, Name: source.Name}, &src); err != nil {
		t.Fatal(err)
	}
	if len(src.Status.Cooldown) != 0 || src.Status.ReceivedTotal != 0 {
		t.Fatalf("publication must spend no cooldown and move no counter: %+v", src.Status)
	}
	events, _ := s.Activity.Since("", 0)
	for _, e := range events {
		if e.Kind == activity.KindSignalClaimed || e.Kind == activity.KindSignalDropped {
			t.Fatalf("publication must emit no claim or drop, got %+v", e)
		}
	}
}

func TestPublishEveryAdmittedSignalIsItsOwnSignal(t *testing.T) {
	source := sigTestSource("own-src", "own-adapter")
	s, c := sigServer(t, source)

	code, resp := respBody(t, s, source.Name,
		map[string]any{"initiationId": "same", "payload": "a"},
		map[string]any{"initiationId": "same", "payload": "b"},
		map[string]any{"initiationId": "other", "payload": "c"},
	)
	if code != 200 || resp["queued"] != float64(3) {
		t.Fatalf("want queued=3, got %d %v", code, resp)
	}
	if n := len(listSignals(t, c)); n != 3 {
		t.Fatalf("three signals, two sharing an initiationId, are three Signals — got %d", n)
	}
}

func TestPublishSignalIDDeduplicates(t *testing.T) {
	source := sigTestSource("dup-src", "dup-adapter")
	s, c := sigServer(t, source)
	sig := map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"}

	for i := 0; i < 2; i++ {
		code, resp := respBody(t, s, source.Name, sig)
		if code != 200 || resp["queued"] != float64(1) {
			t.Fatalf("send %d: want 200 queued=1, got %d %v", i, code, resp)
		}
	}
	sigs := listSignals(t, c)
	if len(sigs) != 1 {
		t.Fatalf("a re-send with the same signalId must yield one Signal, got %d", len(sigs))
	}
	if want := agentopsv1alpha1.SignalNameFor(source.Name, "m-1"); sigs[0].Name != want {
		t.Fatalf("name = %q, want the derived %q", sigs[0].Name, want)
	}
	if dup := countCode(s, activity.CodeDuplicate); dup != 1 {
		t.Fatalf("want exactly one duplicate event, got %d", dup)
	}
}

func countCode(s *Server, code string) int {
	events, _ := s.Activity.Since("", 0)
	n := 0
	for _, e := range events {
		if e.Code == code {
			n++
		}
	}
	return n
}

func TestPublishSameSignalIDTwiceInOneBatchYieldsOne(t *testing.T) {
	source := sigTestSource("batch-src", "batch-adapter")
	s, c := sigServer(t, source)

	code, resp := respBody(t, s, source.Name,
		map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"},
		map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"},
	)
	if code != 200 || resp["queued"] != float64(2) {
		t.Fatalf("want 200 queued=2 (stored or deduplicated), got %d %v", code, resp)
	}
	if n := len(listSignals(t, c)); n != 1 {
		t.Fatalf("want one Signal, got %d", n)
	}
	if dup := countCode(s, activity.CodeDuplicate); dup != 1 {
		t.Fatalf("want one duplicate event, got %d", dup)
	}
}

func TestPublishResendAfterRoutedChangesNothing(t *testing.T) {
	source := sigTestSource("routed-src", "routed-adapter")
	s, c := sigServer(t, source)
	sig := map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"}
	respBody(t, s, source.Name, sig)

	got := listSignals(t, c)[0]
	got.Status.Phase = agentopsv1alpha1.SignalRouted
	got.Status.Claimants = []agentopsv1alpha1.SignalClaimant{{
		Ref:             agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindPipeline, Name: "p"},
		State:           agentopsv1alpha1.ClaimantRouted,
		ConversationRef: &agentopsv1alpha1.ObjectRef{Name: "conv"},
	}}
	if err := c.Status().Update(context.Background(), &got); err != nil {
		t.Fatal(err)
	}

	code, resp := respBody(t, s, source.Name, sig)
	if code != 200 || resp["queued"] != float64(1) {
		t.Fatalf("want 200 queued=1, got %d %v", code, resp)
	}
	after := listSignals(t, c)
	if len(after) != 1 || after[0].Status.Phase != agentopsv1alpha1.SignalRouted ||
		after[0].ResourceVersion != got.ResourceVersion {
		t.Fatalf("a re-send after Routed must leave the Signal untouched: %+v", after)
	}
}

func TestPublishConcurrentRaceYieldsOneSignal(t *testing.T) {
	source := sigTestSource("race-src", "race-adapter")
	s, c := sigServer(t, source)

	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := postSig(s, source.Name, []map[string]any{{"initiationId": "i-1", "signalId": "m-race", "payload": "x"}})
			codes[i] = rec.Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != 200 {
			t.Fatalf("request %d answered %d, every racer must be acknowledged", i, code)
		}
	}
	if n := len(listSignals(t, c)); n != 1 {
		t.Fatalf("a concurrent race on one signalId must yield one Signal, got %d", n)
	}
}

func TestPublishSameSignalIDOnTwoSourcesYieldsTwoSignals(t *testing.T) {
	a, b := sigTestSource("src-a", "ad"), sigTestSource("src-b", "ad")
	s, c := sigServer(t, a, b)
	for _, src := range []string{a.Name, b.Name} {
		respBody(t, s, src, map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"})
	}
	sigs := listSignals(t, c)
	if len(sigs) != 2 || sigs[0].Name == sigs[1].Name {
		t.Fatalf("two sources never collide on one signalId: %+v", sigs)
	}
}

func TestPublishSameInitiationIDDifferentSignalIDsAreTwoMessages(t *testing.T) {
	source := sigTestSource("two-src", "ad")
	s, c := sigServer(t, source)
	respBody(t, s, source.Name,
		map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"},
		map[string]any{"initiationId": "i-1", "signalId": "m-2", "payload": "y"})
	if n := len(listSignals(t, c)); n != 2 {
		t.Fatalf("want 2 Signals, got %d", n)
	}
}

func TestPublishWithoutSignalIDDoesNotDeduplicate(t *testing.T) {
	source := sigTestSource("nodedup-src", "ad")
	s, c := sigServer(t, source)
	sig := map[string]any{"initiationId": "i-1", "payload": "x"}
	respBody(t, s, source.Name, sig)
	respBody(t, s, source.Name, sig)
	sigs := listSignals(t, c)
	if len(sigs) != 2 || sigs[0].Name == sigs[1].Name {
		t.Fatalf("no signalId means two Signals under generated names: %+v", sigs)
	}
	if dup := countCode(s, activity.CodeDuplicate); dup != 0 {
		t.Fatalf("no duplicate event without a signalId, got %d", dup)
	}
}

func TestPublishInvalidSignalIDRefusesTheWholeBatch(t *testing.T) {
	for name, id := range map[string]string{
		"too long":    strings.Repeat("a", 257),
		"has a space": "m 1",
		"non-ascii":   "m-é",
		"control":     "m\x01",
	} {
		t.Run(name, func(t *testing.T) {
			source := sigTestSource("badid-src", "ad")
			s, c := sigServer(t, source)
			code, resp := respBody(t, s, source.Name,
				map[string]any{"initiationId": "i-0", "payload": "fine"},
				map[string]any{"initiationId": "i-1", "signalId": id, "payload": "x"})
			if code != 400 || !strings.Contains(fmt.Sprint(resp["error"]), "signalId") {
				t.Fatalf("want 400 naming signalId, got %d %v", code, resp)
			}
			if n := len(listSignals(t, c)); n != 0 {
				t.Fatalf("a refused batch stores nothing, got %d Signals", n)
			}
		})
	}
	// the 256-byte boundary is accepted
	source := sigTestSource("okid-src", "ad")
	s, _ := sigServer(t, source)
	if code, resp := respBody(t, s, source.Name,
		map[string]any{"initiationId": "i-1", "signalId": strings.Repeat("a", 256), "payload": "x"}); code != 200 {
		t.Fatalf("a 256-byte signalId is valid: %d %v", code, resp)
	}
}

func TestPublishFailedStoreIsNotAcknowledgedAndResendIsDeduplicated(t *testing.T) {
	source := sigTestSource("fail-src", "ad")
	base := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(source)
	var mu sync.Mutex
	creates := 0
	failSecond := interceptor.Funcs{Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
		if _, isSignal := obj.(*agentopsv1alpha1.Signal); isSignal {
			mu.Lock()
			creates++
			n := creates
			mu.Unlock()
			if n == 2 {
				return apierrors.NewInternalError(fmt.Errorf("etcd unavailable"))
			}
		}
		return cl.Create(ctx, obj, opts...)
	}}
	c := base.WithInterceptorFuncs(failSecond).Build()
	s := &Server{Reader: c, Client: c, Namespace: sigTestNamespace, AdapterToken: "sig-test-master", Activity: activity.New(16)}

	batch := []map[string]any{
		{"initiationId": "i-1", "signalId": "m-1", "payload": "x"},
		{"initiationId": "i-2", "signalId": "m-2", "payload": "y"},
	}
	rec := postSig(s, source.Name, batch)
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("a failed store must answer 5xx with an error, got %d %s", rec.Code, rec.Body.String())
	}
	// the adapter's at-least-once re-send: m-1 landed and is deduplicated, m-2 is stored
	rec = postSig(s, source.Name, batch)
	if rec.Code != 200 {
		t.Fatalf("re-send: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(listSignals(t, c)); n != 2 {
		t.Fatalf("after the re-send both messages exist exactly once, got %d", n)
	}
}

func chatChannelObj(name, adapter string) *agentopsv1alpha1.Channel {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = name, sigTestNamespace
	ch.Spec.Adapter = adapter
	return ch
}

func chatPost(t *testing.T, s *Server, source, channel, payload string) (int, map[string]any) {
	t.Helper()
	return respBody(t, s, source, map[string]any{
		"initiationId": "chat-" + payload, "kind": KindChat, "payload": payload,
		"labels": map[string]string{LabelChatChannel: channel},
	})
}

func TestPublishChatCommandsThatOnlyAnswerStoreNoSignal(t *testing.T) {
	for _, text := range []string{"/pipelines", "/k8s-ops", "/close", "/exit"} {
		t.Run(text, func(t *testing.T) {
			source := sigTestSource("cmd-src", "tg")
			ch := chatChannelObj("cmd-chan", "cmdtype")
			s, c := sigServer(t, source, ch, sigTestPipeline("k8s-ops", source.Name))
			code, resp := chatPost(t, s, source.Name, ch.Name, text)
			if code != 200 {
				t.Fatalf("%d %v", code, resp)
			}
			if n := len(listSignals(t, c)); n != 0 {
				t.Fatalf("%s answers in place and stores no Signal, got %d", text, n)
			}
			if op := s.Ops.Claim("cmdtype"); op == nil || op.Message == nil {
				t.Fatalf("%s must be answered on the surface", text)
			}
		})
	}
}

func TestPublishAddressedAndPlainChatAreStoredVerbatim(t *testing.T) {
	source := sigTestSource("chat-src", "tg")
	ch := chatChannelObj("chat-chan", "chattype")
	s, c := sigServer(t, source, ch, sigTestPipeline("k8s-ops", source.Name))

	for _, text := range []string{"/k8s-ops check nodes", "are we up?"} {
		if code, resp := chatPost(t, s, source.Name, ch.Name, text); code != 200 || resp["queued"] != float64(1) {
			t.Fatalf("%q: %d %v", text, code, resp)
		}
	}
	sigs := listSignals(t, c)
	if len(sigs) != 2 {
		t.Fatalf("want 2 Signals, got %d", len(sigs))
	}
	seen := map[string]bool{}
	for _, sg := range sigs {
		seen[sg.Spec.Payload] = true
		if sg.Spec.Kind != KindChat || sg.Spec.Labels[LabelChatChannel] != ch.Name {
			t.Fatalf("chat Signal lost its kind or surface label: %+v", sg.Spec)
		}
	}
	if !seen["/k8s-ops check nodes"] || !seen["are we up?"] {
		t.Fatalf("payloads must be stored verbatim: %v", seen)
	}
	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil || len(convs.Items) != 0 {
		t.Fatalf("no conversation exists until a consumer processes it: %v %d", err, len(convs.Items))
	}
	if op := s.Ops.Claim("chattype"); op != nil {
		t.Fatalf("publication tells no surface anything: %+v", op)
	}
}

func TestBoundedLabelsKeepsTheReservedSurfaceLabel(t *testing.T) {
	in := map[string]string{LabelChatChannel: "c"}
	for i := 0; i < agentopsv1alpha1.MaxSignalLabels+5; i++ {
		in[fmt.Sprintf("a-%03d", i)] = "v"
	}
	out := boundedLabels(in)
	if len(out) != agentopsv1alpha1.MaxSignalLabels || out[LabelChatChannel] != "c" {
		t.Fatalf("the reserved label must survive the cap: %d %v", len(out), out[LabelChatChannel])
	}
}

func TestGeneratedSignalNamesSortInPublishOrderAndAreValid(t *testing.T) {
	prev := ""
	for i := 0; i < 200; i++ {
		name := generatedSignalName()
		if len(name) != len("sig-")+generatedTimeLen+1+generatedSuffixLen || !strings.HasPrefix(name, "sig-") {
			t.Fatalf("unexpected shape %q", name)
		}
		for _, r := range name {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				t.Fatalf("%q is not a valid object name", name)
			}
		}
		if prev != "" && name[:len(name)-generatedSuffixLen-1] < prev[:len(prev)-generatedSuffixLen-1] {
			t.Fatalf("names must sort by time: %q after %q", name, prev)
		}
		prev = name
	}
}
