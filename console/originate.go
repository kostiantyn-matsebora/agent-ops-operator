package main

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Origination: the console starts a conversation THE WAY EVERYTHING ELSE DOES —
// by emitting a chat signal from a claimed SignalSource.
//
// Not through POST /task. Four things follow from that, and each is the reason
// this file is not a thin wrapper around a simpler endpoint:
//
//  1. The invariant holds literally. "Conversations originate only from claimed
//     signal sources" stops being a rule with a side door.
//  2. WHO ANSWERS IS DECLARED, NOT CHOSEN BY THE CALLER. /task takes a pipeline
//     name in the body — the caller picks the agent. A claimed source means the
//     wiring decides, which is the rule's actual point. The console cannot reach
//     an agent no wiring points at.
//  3. Origination becomes visible traffic. A /task conversation materializes
//     from nowhere, leaving a hole in the graph exactly where the operator
//     acted; a console source is a node with an edge, so pressing "start" lights
//     up the graph the console is rendering.
//  4. Self-started conversations join themselves. The claiming pipeline's
//     channel set includes the console Channel, so the conversation arrives with
//     a console thread — no pipeline edit, no copy-paste patch.
//
// The failure mode is diagnosable with machinery that already exists: an
// unclaimed console source sits at Wired=False, which reads exactly as "this
// console cannot start conversations yet".

// Reserved labels a chat signal carries. Without the channel label the reply
// has nowhere to go, and the manager refuses the signal — so the console never
// posts one without it.
const (
	labelChatChannel = "agentops.dev/channel"
	labelChatSender  = "agentops.dev/sender"
)

// Originator posts chat signals using the console's SIGNAL identity — a second
// token in the same pod, never the channel one. The two surfaces validate
// against different CRD lists, so using the wrong token here fails with 401
// rather than silently working.
type Originator struct {
	mgr    *Manager
	source string
}

// NewOriginator builds the origination client, or nil when the console holds no
// signal identity. A nil Originator is the honest representation of "this
// console cannot start conversations" and the API reports it with that reason.
func NewOriginator(managerURL, signalToken, source string) *Originator {
	if signalToken == "" || source == "" {
		return nil
	}
	return &Originator{mgr: NewManager(managerURL, signalToken), source: source}
}

// Source names the SignalSource this console originates from.
func (o *Originator) Source() string {
	if o == nil {
		return ""
	}
	return o.source
}

// signalResponse is the manager's /signal/inbound answer. `reason` carries the
// drop explanation — the Wired=False text for an unclaimed source — which is
// exactly what the UI must show instead of a generic error.
type signalResponse struct {
	Queued        int    `json:"queued"`
	Conversations int    `json:"conversations"`
	Reason        string `json:"reason,omitempty"`
}

// Start emits one chat signal. It returns the manager's drop reason when the
// source is not claimed, rather than an error: nothing failed — the system is
// telling the operator what is missing.
func (o *Originator) Start(ctx context.Context, channel, sender, task string) (reason string, err error) {
	if o == nil {
		return "", fmt.Errorf("this console holds no signal identity: declare a SignalAdapter with " +
			"servedBy pointing at this ChannelAdapter, and set SIGNAL_SOURCE_NAME")
	}
	if channel == "" {
		return "", fmt.Errorf("no console Channel is served, so a reply would have nowhere to go")
	}
	var out signalResponse
	// The fingerprint must be unique per message: cooldown is OFF for chat by
	// default, but a repeated fingerprint would still be deduped, and a person
	// asking the same thing twice means it twice.
	fingerprint := "console-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	_, err = o.mgr.do(ctx, "POST", "/signal/inbound", map[string]any{
		"source": o.source,
		"signals": []map[string]any{{
			"fingerprint": fingerprint,
			"kind":        "chat",
			"payload":     task,
			"labels": map[string]string{
				labelChatChannel: channel,
				labelChatSender:  sender,
			},
		}},
	}, &out)
	if err != nil {
		return "", err
	}
	return out.Reason, nil
}
