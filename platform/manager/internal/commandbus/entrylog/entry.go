// Package entrylog is the append-only model both commandbus backends share
// (design D-2 of openspec/changes/async-command-bus/): a queue is a log of
// immutable entries, and a task's state is folded from the entries that name
// it, the last one deciding. Nothing here rewrites or removes an entry.
//
// A backend owns the transport and nothing else. It asks a Log for a plan (an
// Append: the entry to add and the sequence the key's subject must still end
// at), writes the entry with that condition, and Applies what the log hands
// back. commandbus/memory assigns the sequence itself under a mutex;
// commandbus/nats takes it from JetStream and feeds the Log from a watch.
// Because the planning is the same code, one conformance suite covers both.
//
// Order is a log position and nothing else: a task's position is the sequence
// its message entry was given on append, so FIFO is the log's own and a retry
// keeps its place without a field to maintain.
package entrylog

import (
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Type names what an entry records.
type Type string

const (
	// TypeMsg is the message itself, appended once at Submit.
	TypeMsg Type = "msg"
	// TypeClaimed starts one attempt under a lease.
	TypeClaimed Type = "claimed"
	// TypeFailed ends an attempt without success. It records the outcome the
	// writer decided (retry with a backoff, or dead), so a reader never
	// re-derives a jittered backoff.
	TypeFailed Type = "failed"
	// TypeSucceeded ends the task.
	TypeSucceeded Type = "succeeded"
)

// Entry is one immutable record of the log. Every entry names its task and
// carries the descriptors a Feed event needs, so a consumer resuming
// mid-log never has to find the message entry first.
type Entry struct {
	Type         Type                 `json:"type"`
	UID          string               `json:"uid"`
	CommandID    commandbus.CommandID `json:"commandId"`
	Queue        commandbus.Queue     `json:"queue"`
	Kind         commandbus.Kind      `json:"kind"`
	Key          commandbus.Key       `json:"key"`
	InvocationID string               `json:"invocationId,omitempty"`
	At           time.Time            `json:"at"`

	// TypeMsg.
	Payload     []byte `json:"payload,omitempty"`
	MaxAttempts int    `json:"maxAttempts,omitempty"`

	// TypeClaimed, TypeFailed and TypeSucceeded carry the attempt they end or
	// begin.
	Attempt int `json:"attempt,omitempty"`

	// TypeClaimed.
	Token  string    `json:"token,omitempty"`
	Holder string    `json:"holder,omitempty"`
	Until  time.Time `json:"until,omitzero"`

	// TypeFailed.
	Code        commandbus.ErrorCode `json:"code,omitempty"`
	Detail      string               `json:"detail,omitempty"`
	Dead        bool                 `json:"dead,omitempty"`
	NotBefore   time.Time            `json:"notBefore,omitzero"`
	ParkedUntil time.Time            `json:"parkedUntil,omitzero"`
}

// State is the task state the entry leaves its task in, as a Feed event
// reports it.
func (e Entry) State() commandbus.TaskState {
	switch e.Type {
	case TypeClaimed:
		return commandbus.StateRunning
	case TypeSucceeded:
		return commandbus.StateSucceeded
	case TypeFailed:
		if e.Dead {
			return commandbus.StateDead
		}
		return commandbus.StateRetrying
	default:
		return commandbus.StateQueued
	}
}

// Event is the Feed event for e, appended at sequence seq.
func (e Entry) Event(seq uint64) commandbus.TaskEvent {
	return commandbus.TaskEvent{
		Sequence: commandbus.Sequence(seq), UID: e.UID, Queue: e.Queue, Kind: e.Kind, Key: e.Key,
		CommandID: e.CommandID, InvocationID: e.InvocationID,
		State: e.State(), Attempt: e.Attempt, Code: e.Code,
	}
}

// Append is what a backend writes: the entry, and the sequence the entry's
// subject must still end at for the write to land. Subject is the key's
// message subject for a TypeMsg and the key's event subject for every other
// entry, so two writers racing on one key cannot both land.
type Append struct {
	Entry Entry
	// ExpectLast is the last sequence the subject held when the plan was
	// made. Zero means the subject was empty.
	ExpectLast uint64
}
