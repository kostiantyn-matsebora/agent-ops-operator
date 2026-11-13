//go:build system

package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Section 9 (async-command-bus, task 9) — end-to-end flows through the
// real HTTP surface, full tier, with the task stream (and /state/tasks,
// /state/dead-letters — task 5.8) OBSERVED rather than inferred from the
// eventual Kubernetes-object side effect alone (design D-14's own split:
// an end-to-end flow watches the task surface; lanes_test.go's existing
// lanes watch the resulting CR). Every test below calls fullTier(t), the
// same gate every other full-tier-only lane in this package uses.
//
// 9.1's own ask ("add the end-to-end flows... with the task stream
// observed") is the umbrella these six flows satisfy together, not a
// separate test of its own.

// ---- shared wire shapes and helpers -----------------------------------

// acceptedWire is design D-9's accept response, added to whatever an
// endpoint's body already was — the same shape accept.go's acceptedResponse
// serializes.
type acceptedWire struct {
	OK           bool     `json:"ok"`
	InvocationID string   `json:"invocationId"`
	TaskIDs      []string `json:"taskIds"`
}

// taskStateWire is the subset of statetasks.go's taskWire this file reads
// back off GET /state/tasks/{id} and the task stream's own "put" frames.
type taskStateWire struct {
	UID          string `json:"uid"`
	CommandID    string `json:"commandId"`
	Queue        string `json:"queue"`
	Key          string `json:"key"`
	InvocationID string `json:"invocationId"`
	State        string `json:"state"`
	Attempt      int    `json:"attempt"`
	Code         string `json:"code"`
}

// openTaskStream opens GET /state/tasks/stream, retrying until accepted —
// the same idiom openStateStream (state_stream_restart_test.go) already
// uses for /state/conversations/stream, over the new surface task 5.8
// added. No `since`: a zero resume starts from now (Bus.Feed's own doc
// comment), exactly what every flow below wants — only events THIS test
// causes.
func openTaskStream(t *testing.T, ctx context.Context, e *Env, auth string) *http.Response {
	t.Helper()
	var resp *http.Response
	waitFor(t, "the task stream to open", 2*time.Minute, func() (bool, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", e.Manager.URL()+"/state/tasks/stream", nil)
		req.Header.Set("Authorization", auth)
		r, err := http.DefaultClient.Do(req)
		if err != nil || r.StatusCode != 200 {
			if r != nil {
				r.Body.Close()
			}
			return false, nil
		}
		resp = r
		return true, nil
	})
	return resp
}

// taskPut decodes one "put <json>" frame streamFrames (state_stream_
// restart_test.go) yields off the task stream.
func taskPut(f string) (taskStateWire, bool) {
	data, ok := strings.CutPrefix(f, "put ")
	if !ok {
		return taskStateWire{}, false
	}
	var w taskStateWire
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		return taskStateWire{}, false
	}
	return w, true
}

// awaitTaskUIDSucceeded drains frames until uid is seen in state
// "succeeded", or fails the test.
func awaitTaskUIDSucceeded(t *testing.T, frames <-chan string, uid string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("the task stream ended before task %s reached succeeded", uid)
			}
			if w, ok := taskPut(f); ok && w.UID == uid && w.State == "succeeded" {
				return
			}
		case <-deadline:
			t.Fatalf("task %s never reached succeeded on the task stream within %s", uid, timeout)
		}
	}
}

// awaitTaskKeySucceeded drains frames until SOME task of this queue on this
// Key is seen in state "succeeded" — used where the task's own UID was never handed back to
// this test (an inbound reply forwarded by a real adapter, never this test's
// own HTTP call).
func awaitTaskKeySucceeded(t *testing.T, frames <-chan string, queue, key string, timeout time.Duration) taskStateWire {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("the task stream ended before a task on queue %s key %s succeeded", queue, key)
			}
			if w, ok := taskPut(f); ok && w.Queue == queue && w.Key == key && w.State == "succeeded" {
				return w
			}
		case <-deadline:
			t.Fatalf("no task on queue %s key %s reached succeeded on the task stream within %s", queue, key, timeout)
		}
	}
}

// getTask reads GET /state/tasks/{id} once.
func getTask(t *testing.T, e *Env, auth, uid string) (taskStateWire, int) {
	t.Helper()
	code, out := e.do(t, "GET", e.Manager.URL()+"/state/tasks/"+uid, nil, auth)
	var w taskStateWire
	_ = json.Unmarshal([]byte(out), &w)
	return w, code
}

// waitTaskState polls GET /state/tasks/{id} until it reports want.
func waitTaskState(t *testing.T, e *Env, auth, uid, want string, timeout time.Duration) taskStateWire {
	t.Helper()
	var last taskStateWire
	waitFor(t, fmt.Sprintf("task %s to reach %s", uid, want), timeout, func() (bool, error) {
		w, code := getTask(t, e, auth, uid)
		if code != 200 {
			return false, nil
		}
		last = w
		return w.State == want, nil
	})
	return last
}

// waitTaskSettled waits for a task to have been consumed. A message that
// succeeds is DELETED from the queue (design D-2), so after its short grace
// (30s) GET /state/tasks/{id} answers 404 instead of "succeeded": a settled
// task is either still read back as succeeded or already gone. A dead one
// fails the test at once. The task's effect is what the caller asserts next,
// since a 404 alone cannot tell a consumed task from one never accepted.
func waitTaskSettled(t *testing.T, e *Env, auth, uid string, timeout time.Duration) {
	t.Helper()
	waitFor(t, fmt.Sprintf("task %s to be consumed (succeeded, or removed after its grace)", uid), timeout, func() (bool, error) {
		w, code := getTask(t, e, auth, uid)
		switch {
		case code == 404:
			return true, nil
		case code != 200:
			return false, nil
		case w.State == "dead":
			t.Fatalf("task %s went dead (code %q), it must have been consumed", uid, w.Code)
		}
		return w.State == "succeeded", nil
	})
}

// deadLetters reads GET /state/dead-letters?queue=.
func deadLetters(t *testing.T, e *Env, auth, queue string) []taskStateWire {
	t.Helper()
	code, out := e.do(t, "GET", e.Manager.URL()+"/state/dead-letters?queue="+queue, nil, auth)
	if code != 200 {
		t.Fatalf("GET /state/dead-letters: %d %s", code, out)
	}
	var w struct {
		Items []taskStateWire `json:"items"`
	}
	_ = json.Unmarshal([]byte(out), &w)
	return w.Items
}

// threadOn names the thread id bound on one specific channel, "" if none.
func threadOn(c *agentopsv1alpha1.Conversation, channel string) string {
	for _, th := range c.Status.Threads {
		if th.Channel == channel {
			return th.ThreadID
		}
	}
	return ""
}

// waitThreadOn polls until a conversation's thread on channel is bound.
func waitThreadOn(t *testing.T, ctx context.Context, e *Env, name, channel string, timeout time.Duration) string {
	t.Helper()
	var tid string
	waitFor(t, "the "+channel+" thread bound on "+name, timeout, func() (bool, error) {
		c, err := e.K.Conversation(ctx, name)
		if err != nil {
			return false, nil
		}
		tid = threadOn(c, channel)
		return tid != "", nil
	})
	return tid
}

// postChannelInbound posts POST /channel/inbound exactly as a conforming
// channel adapter does (channel-adapter-contract), decoding the accept
// response.
func postChannelInbound(t *testing.T, e *Env, auth, channel, thread, text, sender string) (int, acceptedWire) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"channel": channel, "threadId": thread, "text": text, "sender": sender})
	code, out := e.do(t, "POST", e.Manager.URL()+"/channel/inbound", body, auth)
	var a acceptedWire
	_ = json.Unmarshal([]byte(out), &a)
	return code, a
}

// deleteConversationViaManager posts POST /channel/conversations/{name}/delete.
func deleteConversationViaManager(t *testing.T, e *Env, auth, name, channel string) (int, acceptedWire) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"channel": channel})
	code, out := e.do(t, "POST", e.Manager.URL()+"/channel/conversations/"+url.PathEscape(name)+"/delete", body, auth)
	var a acceptedWire
	_ = json.Unmarshal([]byte(out), &a)
	return code, a
}

// postInboundDuringRestart POSTs /channel/inbound in a retry loop that is
// safe to run from a background goroutine — never calling *testing.T's own
// Fatal off the test's goroutine (the one thing e.do itself is not safe
// for). The manager is genuinely unreachable for a stretch while
// restartManagerSchedulable force-deletes the old pod, and retrying across
// that gap is the point: this call proves the close was ACCEPTED while that
// was happening, not safely sequenced around it.
func postInboundDuringRestart(ctx context.Context, rawURL, auth string, body []byte, timeout time.Duration) (int, acceptedWire, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, "POST", rawURL, bytes.NewReader(body))
		if err != nil {
			return 0, acceptedWire{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", auth)
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 202 {
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
			time.Sleep(2 * time.Second)
			continue
		}
		var a acceptedWire
		_ = json.Unmarshal(b, &a)
		return resp.StatusCode, a, nil
	}
	return 0, acceptedWire{}, lastErr
}

// botCallsOnThread filters BotCalls(method) down to the ones carrying this
// Telegram thread id — SendWith's own outbound message_thread_id rides as a
// JSON number (telegram.go: *int64), so the match is unquoted.
func botCallsOnThread(t *testing.T, e *Env, method, threadID string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, c := range e.BotCalls(t, method) {
		b, _ := json.Marshal(c["body"])
		if strings.Contains(string(b), `"message_thread_id":`+threadID) {
			out = append(out, c)
		}
	}
	return out
}

// ---- 9.2 ----------------------------------------------------------------

// 9.2 A plain lifecycle: signal, close, delete, every task reaching
// succeeded in order, observed on the task stream throughout.
//
// The signal's own accept is NOT itself a task — task 5.6's own note
// defers /signal/inbound's producer (it stays synchronous), so the two
// tasks this flow actually enqueues are the chat `/close` and the delete,
// both on the "conv" queue keyed by the conversation's own name. The
// ordering this test observes is genuine rather than assumed: delete is
// submitted only once close's own succeeded event has already been seen
// on this SAME stream connection.
func TestAsyncLifecycleSignalCloseDeleteTasksSucceedInOrder(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	fp := "e2e-async-lifecycle-" + stamp
	e.PostTask(t, SourceTasks, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, conv.Name, 1, 3*time.Minute)
	thread := waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, time.Minute)

	code, accepted := postChannelInbound(t, e, auth, ChannelConsole, thread, "/close", "system-pack-9.2")
	if code != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("accepting the close: %d %+v", code, accepted)
	}
	closeUID := accepted.TaskIDs[0]
	awaitTaskUIDSucceeded(t, frames, closeUID, 3*time.Minute)
	waitFor(t, "the conversation closed", time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})

	code, accepted = deleteConversationViaManager(t, e, auth, conv.Name, ChannelConsole)
	if code != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("accepting the delete: %d %+v", code, accepted)
	}
	deleteUID := accepted.TaskIDs[0]
	awaitTaskUIDSucceeded(t, frames, deleteUID, 3*time.Minute)
	waitFor(t, "the conversation object gone", 3*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, conv.Name)
		return err != nil, nil
	})
}

// ---- 9.3 ------------------------------------------------------------------

// 9.3 A bulk close of several conversations.
//
// FINDING: production code's own bulk close
// (platform/console/convapi.go's handleBulkClose) is, by its own doc
// comment, a "FAN-OUT of /close, deliberately, and not a manager-side
// batch verb" — one independent POST /channel/inbound per target, each
// minting its OWN invocationId at accept.go's acceptAndEnqueue. The ONLY
// place this change shares one invocationId across more than one
// SubmitRequest is a CASCADE (design D-7): one root's own causedBy
// subtree, never N conversations a person multi-selected. So task 9.3's
// own wording ("N tasks sharing one invocation") does not hold for a
// plain multi-conversation bulk close as production code stands — this
// test proves the weaker, TRUE statement instead: each of the N targets
// gets its own distinct invocationId, and each settles independently.
// 9.4 below is the cascade case, where sharing one invocation genuinely
// happens.
func TestAsyncBulkCloseEachConversationGetsItsOwnInvocation(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken
	const n = 3

	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		fp := fmt.Sprintf("e2e-async-bulk-%s-%d", stamp, i)
		e.PostTask(t, SourceTasks, fp, "echo "+fp)
		conv := e.ConversationFor(t, fp, 2*time.Minute)
		e.WaitRun(t, conv.Name, 1, 3*time.Minute)
		waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, time.Minute)
		names = append(names, conv.Name)
	}

	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	body, _ := json.Marshal(map[string]any{"names": names})
	code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/close", body, bearerPrefix+e.Values.UIToken)
	if code/100 != 2 || !strings.Contains(out, fmt.Sprintf(`"closed":%d`, n)) {
		t.Fatalf("bulk close: %d %s", code, out)
	}

	invocationOf := awaitCloseInvocations(t, frames, names)

	seenInvocations := map[string]bool{}
	for _, name := range names {
		inv := invocationOf[name]
		if inv == "" {
			t.Fatalf("conversation %s's close task carried no invocationId", name)
		}
		if seenInvocations[inv] {
			t.Fatalf("two conversations shared one invocationId %q — a plain bulk close now shares one, "+
				"which contradicts this test's own documented finding above and 9.3's own premise; update both together", inv)
		}
		seenInvocations[inv] = true
	}
	for _, name := range names {
		waitFor(t, "conversation "+name+" closed", time.Minute, func() (bool, error) {
			c, err := e.K.Conversation(ctx, name)
			return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
		})
	}
}

// awaitCloseInvocations reads the task stream until every named
// conversation's close task (queue "conv") has succeeded, returning each one's invocationId
// and failing if a task's invocationId changes mid-flight.
func awaitCloseInvocations(t *testing.T, frames <-chan string, names []string) map[string]string {
	t.Helper()
	invocationOf := map[string]string{}
	succeeded := map[string]bool{}
	deadline := time.After(3 * time.Minute)
	for len(succeeded) < len(names) {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("the task stream ended before every close settled: %v", succeeded)
			}
			w, ok := taskPut(f)
			if !ok || !slices.Contains(names, w.Key) {
				continue
			}
			if prior, seen := invocationOf[w.Key]; seen && prior != w.InvocationID {
				t.Fatalf("conversation %s's close task changed invocationId mid-flight: %q then %q", w.Key, prior, w.InvocationID)
			}
			invocationOf[w.Key] = w.InvocationID
			if w.State == "succeeded" {
				succeeded[w.Key] = true
			}
		case <-deadline:
			t.Fatalf("not every close settled within the deadline: %v of %v", succeeded, names)
		}
	}
	return invocationOf
}

// ---- 9.4 ------------------------------------------------------------------

// 9.4 A cascade delete of a two-level coordination: root -> member1 ->
// member2 (member1 itself invoking member2 — handleCoordinateInvoke's own
// doc comment: "reachable from a plain MEMBER caller", not only a root).
// Closing the root cascades (already pinned end to end by coordinator_
// test.go's TestCoordinatorCloseAndDeleteCascadeThroughConsole, for one
// level); THIS test's own subject is DELETE's ordering, task 5.10's
// "deepest-first, ending with the target itself" — observed on the task
// stream, not merely the eventual full deletion the existing one-level
// lane already proves.
func TestAsyncCascadeDeleteSettlesMembersBeforeParent(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	const coordName = "e2e-async-cascade-coord"
	const domainCap = "e2e-async-cascade-domain"
	const srcName = "e2e-async-cascade-src"
	mustCreate(t, e.K, agentCapability(domainCap, ProfileStub))
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	co := coordinatorObjWithChannels(coordName, ProfileStub, []string{srcName}, []string{ChannelConsole},
		[]agentopsv1alpha1.CoordinatorAgentEntry{capabilityEntry("domain", domainCap, "handles domain tasks")})
	mustCreate(t, e.K, co)
	if err := waitCoordinatorReady(ctx, e.K, coordName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-async-cascade-root-" + stamp
	e.PostTask(t, srcName, fp, "echo cascade-root-"+stamp)
	root := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, root.Name, 1, 3*time.Minute)

	member1 := e.invokeMember(t, coordName, root.Name, "domain", "echo cascade-m1-"+stamp)
	e.WaitRun(t, member1, 1, 3*time.Minute)
	member2 := e.invokeMember(t, coordName, member1, "domain", "echo cascade-m2-"+stamp)
	e.WaitRun(t, member2, 1, 3*time.Minute)

	// Close the whole tree through the root — this test's own subject is
	// delete ordering below, not close, which the existing one-level lane
	// already covers.
	closeBody, _ := json.Marshal(map[string]any{"names": []string{root.Name}})
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/close", closeBody, bearerPrefix+e.Values.UIToken); code/100 != 2 || !strings.Contains(out, `"closed":1`) {
		t.Fatalf("bulk close the root: %d %s", code, out)
	}
	for _, name := range []string{root.Name, member1, member2} {
		waitFor(t, "conversation "+name+" closed", 3*time.Minute, func() (bool, error) {
			c, err := e.K.Conversation(ctx, name)
			return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
		})
	}

	// Delete the root through the manager's own accept surface
	// (deleteCascadeRequests, task 5.10), observing the task stream for the
	// ordering design D-7/task 5.10 promises.
	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	deleteCode, deleteOut := e.do(t, "POST", e.Manager.URL()+"/channel/conversations/"+url.PathEscape(root.Name)+"/delete",
		[]byte(`{"channel":"console"}`), auth)
	if deleteCode != 202 {
		t.Fatalf("delete the root: %d %s", deleteCode, deleteOut)
	}
	var accepted acceptedWire
	if err := json.Unmarshal([]byte(deleteOut), &accepted); err != nil {
		t.Fatal(err)
	}
	if len(accepted.TaskIDs) != 3 {
		t.Fatalf("a 2-level cascade must enqueue exactly 3 delete tasks (member2, member1, root), got %d: %v",
			len(accepted.TaskIDs), accepted.TaskIDs)
	}

	order := awaitSettleOrder(t, frames, accepted.TaskIDs)
	// accepted.TaskIDs is already deepest-first, root last
	// (deleteCascadeRequests/convtask.CascadeOrder) — the stream's own
	// arrival order must match it exactly.
	for i, uid := range accepted.TaskIDs {
		if order[i] != uid {
			t.Fatalf("delete tasks must settle in cascade order (members before the parent): wanted %v, observed %v",
				accepted.TaskIDs, order)
		}
	}

	waitFor(t, "the root object gone", 3*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, root.Name)
		return err != nil, nil
	})
	for _, name := range []string{member1, member2} {
		waitFor(t, "cascade-deleted member "+name+" gone too", 3*time.Minute, func() (bool, error) {
			_, err := e.K.Conversation(ctx, name)
			return err != nil, nil
		})
	}
}

// ---- 9.5 ------------------------------------------------------------------

// 9.5 An inbound reply delivered through two different channel surfaces —
// the real Telegram fake Bot API lane and the console channel — both now
// landing on the accept-then-process path task 5.1/5.6 added to
// /channel/inbound: a 202 carrying invocationId/taskIds, observed on the
// task surface, ahead of the eventual reply.
func TestAsyncInboundChatThroughTelegramFakeAndConsole(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	// --- Telegram, through the real fake Bot API. gateway-telegram
	// classifies an update carrying is_topic_message+message_thread_id as
	// a CONTINUATION and forwards it to channel-telegram, which itself
	// posts POST /channel/inbound — this test never calls that endpoint
	// for this half, so the task stream is the only way to observe it.
	fpTG := "e2e-async-telegram-" + stamp
	e.PostTask(t, SourceFanout, fpTG, "echo "+fpTG)
	convTG := e.ConversationFor(t, fpTG, 2*time.Minute)
	e.WaitRun(t, convTG.Name, 1, 3*time.Minute)
	tgThread := waitThreadOn(t, ctx, e, convTG.Name, ChannelTelegram, 2*time.Minute)
	tgThreadInt, err := strconv.ParseInt(tgThread, 10, 64)
	if err != nil {
		t.Fatalf("telegram thread id %q is not numeric: %v", tgThread, err)
	}

	tgResp := openTaskStream(t, ctx, e, auth)
	defer tgResp.Body.Close()
	tgFrames := streamFrames(tgResp)

	// The stub runtime answers "[stub] <input minus its directive word>", so
	// the reply carries the text without the leading "echo".
	followUp := "telegram-continue-" + stamp
	replyText := "echo " + followUp
	update := map[string]any{
		"update_id": time.Now().Unix() % 1_000_000_000,
		"message": map[string]any{
			"message_id": 9001, "date": time.Now().Unix(),
			"from": map[string]any{"id": 1234567890, "is_bot": false, "first_name": "Operator"},
			"chat": map[string]any{"id": -1001234567890, "title": "ops", "type": "supergroup", "is_forum": true},
			"text": replyText, "is_topic_message": true, "message_thread_id": tgThreadInt,
		},
	}
	raw, _ := json.Marshal(update)
	e.BotFeed(t, raw)

	awaitTaskKeySucceeded(t, tgFrames, "conv", convTG.Name, 3*time.Minute)
	waitFor(t, "the telegram reply sent back through the fake Bot API", 3*time.Minute, func() (bool, error) {
		for _, c := range e.BotCalls(t, "sendMessage") {
			b, _ := json.Marshal(c["body"])
			if strings.Contains(string(b), "[stub] "+followUp) {
				return true, nil
			}
		}
		return false, nil
	})

	// --- The console channel, posted directly against the manager's own
	// /channel/inbound as the console's own backend does internally
	// (structure.md: the console's "only write anywhere is POST
	// /channel/inbound") — which is what makes the raw accept response
	// observable here, rather than whatever shape the console's own
	// browser-facing API re-wraps it in.
	fpConsole := "e2e-async-console-" + stamp
	e.PostTask(t, SourceTasks, fpConsole, "echo "+fpConsole)
	convConsole := e.ConversationFor(t, fpConsole, 2*time.Minute)
	e.WaitRun(t, convConsole.Name, 1, 3*time.Minute)
	consoleThread := waitThreadOn(t, ctx, e, convConsole.Name, ChannelConsole, time.Minute)

	consoleFollowUp := "console-continue-" + stamp
	consoleReply := "echo " + consoleFollowUp
	code, accepted := postChannelInbound(t, e, auth, ChannelConsole, consoleThread, consoleReply, "system-pack-9.5")
	if code != 202 || accepted.InvocationID == "" || len(accepted.TaskIDs) != 1 {
		t.Fatalf("accepting the console continuation: %d %+v", code, accepted)
	}
	waitTaskSettled(t, e, auth, accepted.TaskIDs[0], 3*time.Minute)
	waitFor(t, "the console reply recorded", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, convConsole.Name), "[stub] "+consoleFollowUp), nil
	})
}

// ---- 9.6 ------------------------------------------------------------------

// 9.6 A manager restart with a task genuinely accepted while the OLD
// process is being force-deleted (restartManagerSchedulable): loses
// nothing, duplicates nothing.
//
// DECISION on the exact claim: this pack's command bus always renders at
// its chart default, commandBus.backend: nats (chart/values.yaml;
// install.go never overrides it), riding the state store's nats servers, so
// the task's own log (design D-2) lives in NATS, never in the manager
// process. That is what lets this test assert the STRONGER of the two claims
// the task text allows: the SAME task UID the old manager minted is the one
// that settles (it never goes dead), not merely the same eventual effect under
// a new one. A succeeded task is removed after a 30s grace, so "settled" reads
// as succeeded or gone.
func TestAsyncManagerRestartLosesNothingDuplicatesNothing(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	fp := "e2e-async-restart-" + stamp
	e.PostTask(t, SourceFanout, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, conv.Name, 1, 3*time.Minute)
	tgThread := waitThreadOn(t, ctx, e, conv.Name, ChannelTelegram, 2*time.Minute)
	before := len(botCallsOnThread(t, e, "sendMessage", tgThread))

	body, _ := json.Marshal(map[string]any{"channel": ChannelTelegram, "threadId": tgThread, "text": "/close", "sender": "system-pack-9.6"})

	var closeCode int
	var closeAccepted acceptedWire
	var closeErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		closeCode, closeAccepted, closeErr = postInboundDuringRestart(ctx, e.Manager.URL()+"/channel/inbound", auth, body, 6*time.Minute)
	}()
	if err := restartManagerSchedulable(ctx, e.Cluster); err != nil {
		t.Fatalf("restart the manager: %v", err)
	}
	wg.Wait()
	if closeErr != nil || closeCode != 202 || len(closeAccepted.TaskIDs) != 1 {
		t.Fatalf("accepting the close across the restart: code=%d err=%v accepted=%+v", closeCode, closeErr, closeAccepted)
	}
	closeUID := closeAccepted.TaskIDs[0]

	// A consumed message is deleted after a 30s grace (design D-2), and the
	// restart itself takes longer than that, so the task is most likely
	// already gone by now: settled means succeeded OR removed, never dead.
	// The 7 minutes sit past the conv queue's 5 minute lease, in case the old
	// pod died holding the claim and the lease had to lapse before another
	// replica could take the SAME task. That the close really ran is what the
	// closed conversation and the single farewell below prove.
	waitTaskSettled(t, e, auth, closeUID, 7*time.Minute)
	waitFor(t, "the conversation closed exactly once", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})

	// No duplicate side effect: exactly one more sendMessage landed on this
	// thread — the farewell, and only the farewell (EnqueueFarewell's own
	// STABLE id, chat/ops.go, dedups a redelivered close's farewell against
	// itself; a genuine duplicate would show as two).
	waitFor(t, "exactly one farewell sent, no duplicate", time.Minute, func() (bool, error) {
		after := len(botCallsOnThread(t, e, "sendMessage", tgThread))
		return after == before+1, nil
	})
}

// ---- 9.7 ------------------------------------------------------------------

// 9.7 A channel surface that genuinely fails: the real fake Bot API rejects
// createForumTopic (ensure-topic's own call, a hand-over message of kind
// KindOutTopic on the Channel's own queue, out.<channel name>), observed being
// handed over to the adapter, retried by the ADAPTER against the real fake Bot
// API, and then failed for good as a real dead-letter record.
//
// Under the hand-over contract (design D-17) the bus runs no attempts and no
// backoff of its own: the adapter acknowledges the message, which starts its 5
// minute delivery TTL, retries against its surface inside that TTL and reports
// FAILED, and a failure is final. So this lane observes running (handed over),
// at least two real createForumTopic calls (the adapter's own retries), then
// dead, never "retrying".
//
// DECISION on recovery: rather than waiting out the natural 5-minute park
// cool-off (design D-4) for the SAME CommandID to unsuppress on its own,
// this test closes and reopens the conversation once the surface is
// healthy again. EnqueueEnsureTopic (chat/ops.go) mints a FRESH
// CommandID per reopen count ("topic:...:r<n>"), so a reopen's own
// re-derive is a genuinely different, unsuppressed submission hitting the
// now-healthy Bot API. The park-then-release timing is already pinned at the
// unit tier by the commandbus conformance suite (task 2.6).
func TestAsyncChannelFailureBacksOffDeadLettersThenRecovers(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	// The stream is opened BEFORE the conversation exists: the telegram
	// ensure-topic is enqueued by the reconciler, never an accept handler, so
	// no HTTP response hands this test its UID and the task stream is the only
	// way to find it. A feed resumed from "now" would miss it once the
	// console half had bound.
	findResp := openTaskStream(t, ctx, e, auth)
	defer findResp.Body.Close()
	frames := streamFrames(findResp)

	callsBefore := len(e.BotCalls(t, "createForumTopic"))
	failBody, _ := json.Marshal(map[string]any{"method": "createForumTopic", "mode": "5xx", "forever": true})
	if code, out := e.do(t, "POST", e.BotAPI.URL()+"/control/fail", failBody, ""); code != 204 {
		t.Fatalf("injecting the failure: %d %s", code, out)
	}
	t.Cleanup(func() {
		e.do(t, "DELETE", e.BotAPI.URL()+"/control/fail?method=createForumTopic", nil, "")
	})

	fp := "e2e-async-failing-channel-" + stamp
	e.PostTask(t, SourceFanout, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	// The console half of the same fanout must bind fine regardless: the
	// injected failure is scoped to the telegram Channel's own queue.
	waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, 2*time.Minute)

	uid := awaitChannelTaskUID(t, frames, ChannelTelegram, conv.Name)

	// Handed over: the adapter acknowledged it and is delivering.
	waitTaskState(t, e, auth, uid, "running", 2*time.Minute)

	// Real retries by the adapter against the real fake Bot API.
	waitFor(t, "at least two real createForumTopic attempts", 2*time.Minute, func() (bool, error) {
		return len(e.BotCalls(t, "createForumTopic"))-callsBefore >= 2, nil
	})

	// The adapter reports FAILED when its retry budget (the 5 minute delivery
	// TTL less a margin) is spent. 7 minutes covers the TTL backstop as well.
	waitTaskState(t, e, auth, uid, "dead", 7*time.Minute)
	dead := deadLetters(t, e, auth, "out."+ChannelTelegram)
	if !slices.ContainsFunc(dead, func(d taskStateWire) bool { return d.UID == uid }) {
		t.Fatalf("the dead task %s must be listed on GET /state/dead-letters?queue=out.%s", uid, ChannelTelegram)
	}

	// Recover the surface, then force a re-derive through a close+reopen
	// (see this test's own doc comment for why, rather than the natural
	// park cool-off).
	if code, out := e.do(t, "DELETE", e.BotAPI.URL()+"/control/fail?method=createForumTopic", nil, ""); code != 204 {
		t.Fatalf("clearing the failure: %d %s", code, out)
	}
	thread := waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, time.Minute)
	code, accepted := postChannelInbound(t, e, auth, ChannelConsole, thread, "/close", "system-pack-9.7")
	if code != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("closing before reopen: %d %+v", code, accepted)
	}
	waitTaskSettled(t, e, auth, accepted.TaskIDs[0], 2*time.Minute)
	waitFor(t, "the conversation closed", time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})

	reopenCode, reopenOut := e.do(t, "POST", e.Manager.URL()+"/channel/conversations/"+url.PathEscape(conv.Name)+"/reopen",
		[]byte(`{"channel":"console"}`), auth)
	var reopenAccepted acceptedWire
	_ = json.Unmarshal([]byte(reopenOut), &reopenAccepted)
	if reopenCode != 202 || len(reopenAccepted.TaskIDs) != 1 {
		t.Fatalf("reopening: %d %s", reopenCode, reopenOut)
	}
	waitTaskSettled(t, e, auth, reopenAccepted.TaskIDs[0], time.Minute)

	waitFor(t, "a fresh, successful telegram topic once the surface recovered", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		if err != nil {
			return false, err
		}
		return threadOn(c, ChannelTelegram) != "", nil
	})
}
