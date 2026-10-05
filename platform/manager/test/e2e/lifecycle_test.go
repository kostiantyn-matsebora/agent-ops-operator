//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Section 9 — the conversation lifecycle through the console: a conforming
// ChannelAdapter with no third-party dependency, driven through its own HTTP
// API — the path a person takes minus the browser. Nothing in the path is a
// double.

// 9.1 + 9.2: start, continue, delivery to the bound thread; the write path
// refuses an unauthenticated request rather than being bypassed.
func TestConsoleLifecycle(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	assertUnauthenticatedConsoleWriteRefused(t, e)
	name := consoleStartAndFindConversation(t, ctx, e, stamp)
	assertConsoleFirstRunDelivered(t, e, name, stamp)
	consoleContinueAndAssertSecondAnswer(t, e, name, stamp)
	assertPersonsWordsRecordedOnRun(t, ctx, e, name, stamp)

	// 9.3 /close sets a phase, archives every bound thread, and the object
	// survives; delete is the second verb and refuses anything not Closed.
	t.Run("close then delete", func(t *testing.T) {
		assertCloseThenDelete(t, ctx, e, name)
	})
}

// 9.2 first: no session, no write.
func assertUnauthenticatedConsoleWriteRefused(t *testing.T, e *Env) {
	t.Helper()
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations", []byte(`{"task":"echo nope"}`), ""); code != 401 {
		t.Fatalf("an unauthenticated write must be refused, got %d %s", code, out)
	}
}

// consoleStartAndFindConversation starts a task and returns the conversation
// it opened. The console originates through its chat SignalSource; the text
// rides a payloadRef, so the conversation is matched by source and age.
func consoleStartAndFindConversation(t *testing.T, ctx context.Context, e *Env, stamp string) string {
	t.Helper()
	start := time.Now().Add(-5 * time.Second)
	task := "echo console " + stamp
	// Retried, not a single call: right after a reused install's manager and
	// adapters are restarted (resetReusedInstall), the console Channel's own
	// `Served` condition takes a reconcile pass to come back, which this test
	// hitting the console FIRST — before anything else has incidentally
	// warmed it up — can otherwise race, exactly as the alertmanager lane's
	// own webhook retries the same class of transient condition.
	var code int
	var out string
	waitFor(t, "the console to accept a start (its Channel served again)", time.Minute, func() (bool, error) {
		code, out = e.ConsoleStart(t, task)
		return code/100 == 2, nil
	})
	if code/100 != 2 {
		t.Fatalf("start: %d %s", code, out)
	}
	var name string
	waitFor(t, "the started conversation", 2*time.Minute, func() (bool, error) {
		items, err := e.K.Conversations(ctx)
		if err != nil {
			return false, err
		}
		for _, c := range items {
			if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == SourceConsole &&
				c.CreationTimestamp.Time.After(start) {
				name = c.Name
				return true, nil
			}
		}
		return false, nil
	})
	return name
}

// assertConsoleFirstRunDelivered: delivered to the console's bound thread —
// the transcript shows the answer.
func assertConsoleFirstRunDelivered(t *testing.T, e *Env, name, stamp string) {
	t.Helper()
	conv := e.WaitRun(t, name, 1, 4*time.Minute)
	if got := conv.Status.Runs[0].Result; !strings.Contains(got, "[stub] console "+stamp) {
		t.Fatalf("first run result: %q", got)
	}
	waitFor(t, "the answer in the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, name), "[stub] console "+stamp), nil
	})
}

func consoleContinueAndAssertSecondAnswer(t *testing.T, e *Env, name, stamp string) {
	t.Helper()
	if code, out := e.ConsoleSend(t, name, "echo second "+stamp); code/100 != 2 {
		t.Fatalf("send: %d %s", code, out)
	}
	e.WaitRun(t, name, 2, 4*time.Minute)
	waitFor(t, "the second answer in the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, name), "[stub] second "+stamp), nil
	})
}

// assertPersonsWordsRecordedOnRun: the person's own words are Kubernetes-API
// state, beside the answer.
func assertPersonsWordsRecordedOnRun(t *testing.T, ctx context.Context, e *Env, name, stamp string) {
	t.Helper()
	conv, _ := e.K.Conversation(ctx, name)
	recorded := false
	for _, r := range conv.Status.Runs {
		for _, in := range r.Inputs {
			if strings.Contains(in.Text, "second "+stamp) {
				recorded = true
			}
		}
	}
	if !recorded {
		t.Fatalf("what a person typed must be recorded on the run: %+v", conv.Status.Runs)
	}
}

func assertCloseThenDelete(t *testing.T, ctx context.Context, e *Env, name string) {
	t.Helper()
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/delete",
		[]byte(`{"names":["`+name+`"]}`), "Bearer "+e.Values.UIToken); code/100 == 2 && !strings.Contains(out, "refused") && !strings.Contains(out, "Closed") {
		// A 2xx with an all-failed report is still a refusal; look at the CR.
		c, _ := e.K.Conversation(ctx, name)
		if c == nil {
			t.Fatalf("delete of a conversation that is not Closed must be refused (%d %s)", code, out)
		}
	}
	if code, out := e.ConsoleSend(t, name, "/close"); code/100 != 2 {
		t.Fatalf("/close: %d %s", code, out)
	}
	waitFor(t, "phase Closed", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, name)
		if err != nil {
			return false, err
		}
		return c.Status.Phase == "Closed" && c.Status.ClosedAt != nil, nil
	})
	conv, _ := e.K.Conversation(ctx, name)
	if len(conv.Status.Threads) == 0 {
		t.Fatalf("threads must survive a close")
	}
	waitFor(t, "every bound thread archived", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, name)
		if err != nil {
			return false, err
		}
		return len(c.Status.ThreadsArchived) >= len(c.Status.Threads), nil
	})
	if len(conv.Status.Runs) < 2 || conv.Status.RuntimeContextID == "" {
		t.Fatalf("runs and the context handle must survive a close: %+v", conv.Status)
	}
	// Now delete is allowed.
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/delete",
		[]byte(`{"names":["`+name+`"]}`), "Bearer "+e.Values.UIToken); code/100 != 2 {
		t.Fatalf("delete of a Closed conversation: %d %s", code, out)
	}
	waitFor(t, "the object to be gone", 3*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, name)
		return err != nil, nil
	})
}

// 9.3.1 The SAME bulk close/delete surface the coordinator cascade test
// (TestCoordinatorCloseAndDeleteCascadeThroughConsole) exercises — proven
// here against an ORDINARY pipeline conversation, so both kinds are shown to
// behave identically where they are supposed to. This is the gap the QA pass
// actually found (table item #12): the console's new surface had only ever
// been exercised against a live coordinator.
func TestConsolePlainConversationBulkCloseAndDelete(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	name := consoleStartAndFindConversation(t, ctx, e, stamp)
	assertConsoleFirstRunDelivered(t, e, name, stamp)
	body, _ := json.Marshal(map[string]any{"names": []string{name}})

	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/close", body, "Bearer "+e.Values.UIToken); code/100 != 2 || !strings.Contains(out, `"closed":1`) {
		t.Fatalf("bulk close: %d %s", code, out)
	}
	waitFor(t, "phase Closed", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, name)
		return err == nil && c.Status.Phase == "Closed", err
	})
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/delete", body, "Bearer "+e.Values.UIToken); code/100 != 2 || !strings.Contains(out, `"deleted":1`) {
		t.Fatalf("bulk delete: %d %s", code, out)
	}
	waitFor(t, "the object gone", 3*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, name)
		return err != nil, nil
	})
}

// 9.3.2 The console's mark-unread rewind (design D-E), against a REAL
// manager and a REAL console reading its own live activity feed.
//
// THIS TEST PINNED A CONFIRMED PRODUCTION DEFECT THIS E2E PASS FOUND, AND IS
// NOW THE FIX'S OWN ACCEPTANCE CHECK.
//
// `ThreadBinding.ReadAt` / `ReaderMark.ReadAt` were `*metav1.Time`
// (api/v1alpha1/conversation_types.go), which the Kubernetes API serializes
// at SECOND granularity — any sub-second component was silently dropped on
// write. `conversations.go`'s `readReportTime()` reports the newest counted
// message's OWN timestamp verbatim, read off the console's LIVE transcript
// buffer (nanosecond precision). The stored watermark was therefore always
// the FLOOR of that message's real timestamp, strictly EARLIER than it
// whenever the message's nanosecond component was nonzero — and
// `countUnread`'s strict `at.After(wm)` then counted that SAME message as
// unread forever, for as long as the console process kept it in its live
// buffer (unbounded in practice: `transcript.go`'s buffer evicts by LRU
// across THREADS, not by message age). Marking a conversation read did not
// reliably clear its own unread flag in ordinary, continuous operation.
//
// FIXED by widening both fields to a plain `*string` (RFC3339Nano) — the
// shape the console's own types already used everywhere else — so the
// watermark round-trips at full precision and no longer loses to the
// message it was meant to cover.
func TestConsoleMarkReadThenUnreadRewind(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	name := consoleStartAndFindConversation(t, ctx, e, stamp)
	assertConsoleFirstRunDelivered(t, e, name, stamp)
	body, _ := json.Marshal(map[string]any{"names": []string{name}})

	// Mark read first, so the rewind below has something to prove. The
	// console projects state from its OWN informer-fed cache, not from this
	// write, so — exactly like every other console-driven assertion in this
	// pack — the result is polled rather than read once immediately after.
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/read", body, "Bearer "+e.Values.UIToken); code/100 != 2 {
		t.Fatalf("mark read: %d %s", code, out)
	}
	var readAt string
	waitFor(t, "the conversation to read as read, with a stamped watermark", 20*time.Second, func() (bool, error) {
		unread, at := consoleConversationReadState(t, e, name)
		readAt = at
		return !unread && at != "", nil
	})

	// Rewind it ("mark unread"): the manager SETS the reader's own watermark
	// to just before the newest counted message, earlier than what is stored.
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/unread", body, "Bearer "+e.Values.UIToken); code/100 != 2 || !strings.Contains(out, `"marked":1`) {
		t.Fatalf("mark unread: %d %s", code, out)
	}
	var rewoundReadAt string
	waitFor(t, "the rewind to take: the thread reads unread again", time.Minute, func() (bool, error) {
		unread, at := consoleConversationReadState(t, e, name)
		rewoundReadAt = at
		return unread, nil
	})
	if rewoundReadAt == readAt {
		t.Fatalf("mark-unread must actually MOVE the watermark earlier, not merely flip a flag: before=%q after=%q", readAt, rewoundReadAt)
	}

	// A LATER OPEN must not silently re-advance past the rewind: the detail
	// view (handleConversation) is a pure read — it calls no ReportRead — so
	// viewing the conversation again must leave the rewound watermark intact.
	_ = e.ConsoleTranscript(t, name) // "opens" the conversation, as the console's detail view does
	unreadAfterOpen, readAtAfterOpen := consoleConversationReadState(t, e, name)
	if !unreadAfterOpen || readAtAfterOpen != rewoundReadAt {
		t.Fatalf("a later open must not silently re-advance the rewound watermark: rewound=%q afterOpen=%q (unread=%v)",
			rewoundReadAt, readAtAfterOpen, unreadAfterOpen)
	}
}

// consoleConversationReadState reads a conversation's console-thread
// unread/readAt projection straight off the detail endpoint's own JSON.
func consoleConversationReadState(t *testing.T, e *Env, name string) (bool, string) {
	t.Helper()
	out := e.ConsoleTranscript(t, name)
	var parsed struct {
		Conversation struct {
			Unread bool   `json:"unread"`
			ReadAt string `json:"readAt"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parsing conversation detail: %v (%s)", err, out)
	}
	return parsed.Conversation.Unread, parsed.Conversation.ReadAt
}

// 9.4 Multi-channel fan-out: console plus the Telegram lane bound to one
// conversation; both threads receive the answer — the console's transcript
// and a sendMessage recorded on the fake Bot API.
func TestFanOutToBothChannels(t *testing.T) {
	e := requireEnv(t)
	stamp := fmt.Sprint(time.Now().UnixNano())
	fp := "e2e-fanout-" + stamp
	e.PostTask(t, SourceFanout, fp, "echo fanout "+stamp)
	conv := e.ConversationFor(t, fp, time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	waitFor(t, "two thread bindings", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(context.Background(), conv.Name)
		return err == nil && len(c.Status.Threads) == 2, err
	})
	waitFor(t, "the answer in the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, conv.Name), "fanout "+stamp), nil
	})
	waitFor(t, "the answer sent to Telegram", 2*time.Minute, func() (bool, error) {
		for _, c := range e.BotCalls(t, "sendMessage") {
			b, _ := json.Marshal(c["body"])
			if strings.Contains(string(b), "fanout "+stamp) {
				return true, nil
			}
		}
		return false, nil
	})
	// Delivery is a recorded fact per thread, marked on op COMPLETION.
	waitFor(t, "delivery recorded on both threads", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(context.Background(), conv.Name)
		if err != nil {
			return false, err
		}
		run := c.Status.Runs[len(c.Status.Runs)-1]
		return run.DeliveryTracked && len(run.Delivered) == 2, nil
	})
}
