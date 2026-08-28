//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// 9.2 first: no session, no write.
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations", []byte(`{"task":"echo nope"}`), ""); code != 401 {
		t.Fatalf("an unauthenticated write must be refused, got %d %s", code, out)
	}
	start := time.Now().Add(-5 * time.Second)
	task := "echo console " + stamp
	code, out := e.ConsoleStart(t, task)
	if code/100 != 2 {
		t.Fatalf("start: %d %s", code, out)
	}
	// The console originates through its chat SignalSource; the text rides a
	// payloadRef, so the conversation is matched by source and age.
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
	conv := e.WaitRun(t, name, 1, 4*time.Minute)
	if got := conv.Status.Runs[0].Result; !strings.Contains(got, "[stub] console "+stamp) {
		t.Fatalf("first run result: %q", got)
	}
	// Delivered to the console's bound thread — the transcript shows the answer.
	waitFor(t, "the answer in the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, name), "[stub] console "+stamp), nil
	})
	// Continue.
	if code, out := e.ConsoleSend(t, name, "echo second "+stamp); code/100 != 2 {
		t.Fatalf("send: %d %s", code, out)
	}
	e.WaitRun(t, name, 2, 4*time.Minute)
	waitFor(t, "the second answer in the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, name), "[stub] second "+stamp), nil
	})
	// The person's own words are Kubernetes-API state, beside the answer.
	conv, _ = e.K.Conversation(ctx, name)
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

	// 9.3 /close sets a phase, archives every bound thread, and the object
	// survives; delete is the second verb and refuses anything not Closed.
	t.Run("close then delete", func(t *testing.T) {
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
	})
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
	_ = metav1.Now()
}
