//go:build system

package system

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// chat-shaped-conversations, end to end: a Coordinator is addressable from
// chat exactly as a Pipeline is, and the root it opens binds its channels at
// CREATION (coordinator-unconditional-channels) rather than waiting for an
// `escalate` call — the regression this PR fixes, and the live substrate fact
// envtest cannot settle on its own: a real reconcile pass actually driving the
// topic-creation and delivery ops against the real channel adapters (the
// console's own HTTP surface and the Telegram lane against the fake Bot API).
//
// FULL TIER: two real conversation pods (the root, then an invoked member)
// across several dispatch cycles.

// boundToConsoleAndTelegram reports whether the attribution shows exactly the
// console thread and the Telegram thread.
func boundToConsoleAndTelegram(attr consoleAttribution) bool {
	seen := map[string]bool{}
	for _, th := range attr.Threads {
		seen[th.Channel] = true
	}
	return len(attr.Threads) == 2 && seen[ChannelConsole] && seen[ChannelTelegram]
}

// botSentContaining reports whether any sendMessage the fake Bot API saw
// carried the text.
func botSentContaining(t *testing.T, e *Env, text string) bool {
	t.Helper()
	for _, c := range e.BotCalls(t, "sendMessage") {
		b, _ := json.Marshal(c["body"])
		if strings.Contains(string(b), text) {
			return true
		}
	}
	return false
}

// runsRecordInput reports whether any run of the conversation recorded an
// input containing the text.
func runsRecordInput(c *agentopsv1alpha1.Conversation, text string) bool {
	for _, r := range c.Status.Runs {
		for _, in := range r.Inputs {
			if strings.Contains(in.Text, text) {
				return true
			}
		}
	}
	return false
}

// 1. A coordinator conversation is addressable from chat, end to end
//    (coordinator-unconditional-channels): `/<coordinator-name> <task>` on a
//    chat-capable source opens a ROOT whose `spec.channelRefs` already carry
//    the Coordinator's own declared channel PLUS the addressing surface, with
//    NO wait for `escalate` — and a reply landing before anything escalates
//    is delivered as an ordinary input (`DeliverInputs`'s removed escalation
//    fence).
//
// 2. Escalation still works, and no longer (re)binds anything
//    (coordination-escalation, superseded by coordinator-unconditional-
//    channels): the digest posts into the thread ALREADY open — no new
//    Telegram topic — `status.escalatedAt` is stamped, and a member result
//    delivered AFTER escalate still reaches the thread normally.
func TestCoordinatorAddressedChatBindsChannelsAtCreationAndEscalates(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	const coordName = "e2e-coord-chat"
	const domainCap = "e2e-coord-chat-domain"

	mustCreate(t, e.K, agentCapability(domainCap, ProfileStub))
	co := coordinatorObjWithChannels(coordName, ProfileStub, nil, []string{ChannelConsole},
		[]agentopsv1alpha1.CoordinatorAgentEntry{capabilityEntry("domain", domainCap, "handles domain tasks")})
	mustCreate(t, e.K, co)
	if err := waitCoordinatorReady(ctx, e.K, coordName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	start := time.Now().Add(-5 * time.Second)
	task := "echo coordchat-" + stamp
	// The chat lane, driven directly against /signal/inbound exactly as
	// TestUnclaimedSourceDrops's chat sub-case does — real manager routing and
	// real delivery through the real channel adapters, with the Telegram BOT
	// ITSELF the only double (test/fakebotapi, already load-bearing
	// elsewhere). SourceTelegram is already Ready-served by PipelineChat, so
	// the chat lane's "is anything wired to this source" gate passes — the
	// addressed command below reaches the Coordinator BY NAME regardless of
	// whether the Coordinator itself claims the source (discoverable
	// addressing: reached, never named by a claim).
	e.PostSignal(t, SourceTelegram, map[string]any{
		"fingerprint": "e2e-coord-chat-" + stamp, "kind": "chat",
		"payload": "/" + coordName + " " + task,
		"labels":  map[string]string{"agentops.dev/channel": ChannelTelegram, "agentops.dev/sender": "e2e-operator"},
	})

	// waitForCoordinatorRoot locates the object by its generated name — there
	// is no console search-by-criteria surface, so this one lookup is
	// necessarily a Kubernetes read. Everything asserted ABOUT it from here
	// on goes through the console's own HTTP API: that is the actual
	// interface chat-shaped-conversations is for, and a check against the
	// raw CRD proves the manager's wiring without proving the console shows
	// it — exactly the gap that let AttributeCoordinator ship broken (it
	// silently dropped a conversation's coordinator attribution the moment
	// its Coordinator CR was deleted, and no test caught it because every
	// channel-binding assertion here read the CRD directly instead of the
	// console response a person actually sees).
	root := waitForCoordinatorRoot(ctx, t, e, coordName, start, 2*time.Minute)
	if root.Spec.CausedBy != nil {
		t.Fatalf("an addressed coordinator command must open a ROOT with no causedBy, got %+v", root.Spec.CausedBy)
	}
	// The console's own cache is an eventually-consistent watch over the CR,
	// so the attribution it serves can lag the write that just created the
	// root by a beat — poll it rather than asserting on the first read.
	var attr consoleAttribution
	waitFor(t, "the console attributes the root to its coordinator with both channels bound", 30*time.Second, func() (bool, error) {
		attr = consoleConversationAttribution(t, e, root.Name)
		return attr.Coordinator == coordName && boundToConsoleAndTelegram(attr), nil
	})
	if attr.Coordinator != coordName {
		t.Fatalf("the console must attribute the root to its coordinator by name, got %q want %q", attr.Coordinator, coordName)
	}
	if !boundToConsoleAndTelegram(attr) {
		t.Fatalf("the console must show the root bound to the Coordinator's own declared channel (%s) PLUS the "+
			"addressing channel (%s) at CREATION, with no wait for escalate — got %+v",
			ChannelConsole, ChannelTelegram, attr.Threads)
	}

	root = e.WaitRun(t, root.Name, 1, 3*time.Minute)
	if got := root.Status.Runs[0].Result; !strings.Contains(got, "[stub] coordchat-"+stamp) {
		t.Fatalf("first run result: %q", got)
	}

	waitFor(t, "both threads bound (console + the addressing surface)", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, root.Name)
		return err == nil && len(c.Status.Threads) == 2, err
	})
	waitFor(t, "the answer in the console thread, with no escalate call", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, root.Name), "coordchat-"+stamp), nil
	})
	waitFor(t, "the answer sent to Telegram, with no escalate call", 2*time.Minute, func() (bool, error) {
		return botSentContaining(t, e, "coordchat-"+stamp), nil
	})

	// The other half of the same fix: a reply landing BEFORE anything
	// escalates is delivered as an ordinary input — not dropped, not fenced
	// on an escalation moment that no longer exists for this channel.
	if code, out := e.ConsoleSend(t, root.Name, "echo preescalate-"+stamp); code/100 != 2 {
		t.Fatalf("send: %d %s", code, out)
	}
	root = e.WaitRun(t, root.Name, 2, 3*time.Minute)
	waitFor(t, "the pre-escalate reply delivered to the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, root.Name), "preescalate-"+stamp), nil
	})

	topicsBeforeEscalate := len(e.BotCalls(t, "createForumTopic"))

	// 2. Escalate: posts into the thread ALREADY open, stamps
	// status.escalatedAt, binds nothing new.
	token := coordinatorToken(e, coordName, root.Name)
	code, out := e.coordinateCall(t, "/coordinate/escalate", token,
		map[string]any{"conversation": root.Name, "message": "escalating-" + stamp})
	if code != 200 {
		t.Fatalf("escalate: %d %v", code, out)
	}
	waitFor(t, "status.escalatedAt stamped", time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, root.Name)
		if err != nil {
			return false, err
		}
		return c.Status.EscalatedAt != nil, nil
	})
	waitFor(t, "the escalate digest delivered into the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, root.Name), "escalating-"+stamp), nil
	})
	waitFor(t, "the escalate digest delivered to Telegram too — the SAME thread set", 2*time.Minute, func() (bool, error) {
		return botSentContaining(t, e, "escalating-"+stamp), nil
	})
	if got := len(e.BotCalls(t, "createForumTopic")); got != topicsBeforeEscalate {
		t.Fatalf("escalate must post into the EXISTING Telegram topic, never open a new one: "+
			"topics before=%d after=%d", topicsBeforeEscalate, got)
	}

	// A member result delivered AFTER escalate still reaches the thread
	// normally — no regression in ordinary delivery once escalated.
	member := e.invokeMember(t, coordName, root.Name, "domain", "echo postescalate-"+stamp)
	e.WaitRun(t, member, 1, 3*time.Minute)
	root = e.WaitRun(t, root.Name, 3, 3*time.Minute)
	if !runsRecordInput(root, "postescalate-"+stamp) {
		t.Fatalf("a member result delivered AFTER escalate must still reach the root as an "+
			"ordinary routed input, root runs: %+v", root.Status.Runs)
	}
	waitFor(t, "the post-escalate member result delivered to the console thread", 2*time.Minute, func() (bool, error) {
		return strings.Contains(e.ConsoleTranscript(t, root.Name), "postescalate-"+stamp), nil
	})
}
