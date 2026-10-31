package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// This file is the record's own vocabulary: the lookups and delivery rules the
// manager used to read off api/v1alpha1.Conversation, re-stated on the record
// so no caller needs the Kubernetes type to ask them.

// ContextID returns the runtime's context handle, or "". The deprecated
// SessionID spelling is read as the fallback so a record written before the
// rename keeps its handle across the upgrade.
func (r *ConversationRecord) ContextID() string {
	if r.RuntimeContextID != "" {
		return r.RuntimeContextID
	}
	return r.SessionID
}

// SetContextID records the runtime's handle, writing only the current field
// and retiring the deprecated one.
func (r *ConversationRecord) SetContextID(id string) {
	r.RuntimeContextID = id
	r.SessionID = ""
}

// Thread returns the binding on a channel, or nil when the channel holds no
// thread here. The pointer aliases the record's own slice.
func (r *ConversationRecord) Thread(channel string) *ThreadBinding {
	for i := range r.Threads {
		if r.Threads[i].Channel == channel {
			return &r.Threads[i]
		}
	}
	return nil
}

// ThreadFor returns the thread id bound for a channel, or nil. An empty
// ThreadID is treated as no binding: a binding may exist carrying only a
// Claim while its topic is still being created.
func (r *ConversationRecord) ThreadFor(channel string) *string {
	for i := range r.Threads {
		if r.Threads[i].Channel == channel && r.Threads[i].ThreadID != "" {
			return &r.Threads[i].ThreadID
		}
	}
	return nil
}

// BoundTo reports whether the record references a channel.
func (r *ConversationRecord) BoundTo(channel string) bool {
	for _, c := range r.ChannelRefs {
		if c == channel {
			return true
		}
	}
	return false
}

// ThreadArchived reports whether the thread on a channel was already archived.
func (r *ConversationRecord) ThreadArchived(channel string) bool {
	for _, c := range r.ThreadsArchived {
		if c == channel {
			return true
		}
	}
	return false
}

// UnreadFor reports whether a channel's thread has activity the given reader
// has not been reported read up to. A channel with no binding is never unread.
func (r *ConversationRecord) UnreadFor(channel, reader string) bool {
	t := r.Thread(channel)
	if t == nil {
		return false
	}
	return t.Unread(r.LastActivity, reader)
}

// SameOrigin reports whether r and other were originated by the same Pipeline
// or the same Coordinator. A record naming neither never matches.
func (r *ConversationRecord) SameOrigin(other *ConversationRecord) bool {
	switch {
	case r.PipelineRef != "":
		return other.PipelineRef == r.PipelineRef
	case r.CoordinatorRef != "":
		return other.CoordinatorRef == r.CoordinatorRef
	default:
		return false
	}
}

// Watermark returns the reader's own mark when it has one, and the
// channel-wide mark otherwise.
func (t *ThreadBinding) Watermark(reader string) *string {
	if reader != "" {
		for i := range t.Readers {
			if t.Readers[i].Key == reader {
				return t.Readers[i].ReadAt
			}
		}
	}
	return t.ReadAt
}

// ParseWatermark parses a ReadAt string into a comparable time. An empty or
// unparsable string reports !ok.
func ParseWatermark(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// Unread reports whether the thread has activity newer than its watermark. A
// binding without ReadTracked predates the mechanism and is read. A tracked
// binding with no usable watermark is unread.
func (t *ThreadBinding) Unread(lastActivity *time.Time, reader string) bool {
	if !t.ReadTracked {
		return false
	}
	at := t.Watermark(reader)
	if at == nil {
		return true
	}
	parsed, ok := ParseWatermark(*at)
	if !ok {
		return true
	}
	if lastActivity == nil {
		return false
	}
	return lastActivity.After(parsed)
}

// OriginSurface names the channel an input was typed on, or "" when no surface
// displayed it. labels are the input's blob labels.
func (i *InputItem) OriginSurface(labels map[string]string) string {
	if i.Origin == nil {
		return ""
	}
	switch i.Origin.Kind {
	case OriginChannel:
		return i.Origin.Name
	case OriginSignal:
		return labels[LabelChatChannel]
	}
	return ""
}

// OriginSender is the identity that typed an input, from whichever lane
// carried it, or "".
func (i *InputItem) OriginSender(labels map[string]string) string {
	if i.Origin != nil && i.Origin.Sender != "" {
		return i.Origin.Sender
	}
	return labels[LabelChatSender]
}

// TypedByAPerson reports whether somebody wrote the input, as opposed to an
// event that opened it.
func (i *InputItem) TypedByAPerson() bool {
	if i.Origin == nil {
		return false
	}
	return i.Origin.Kind == OriginChannel || i.Origin.SignalKind == SignalKindChat
}

// DeliverTo answers, per destination, whether a bound channel still owes its
// readers this message. An input with no origin is delivered nowhere.
func (i *InputItem) DeliverTo(channel, originSurface string, originEchoes bool) bool {
	if i.Origin == nil {
		return false
	}
	return !(channel == originSurface && originEchoes)
}

// Record renders one queued input as the entry a run keeps, applying the
// inline cap in the one place it is applied. text is the resolved message and
// labels are the input blob's.
func (i *InputItem) Record(text string, labels map[string]string) RecordedInput {
	rec := RecordedInput{
		ID: i.ID, Type: i.Type, PayloadRef: i.PayloadRef,
		Surface: i.OriginSurface(labels), Sender: i.OriginSender(labels),
	}
	if i.Origin != nil {
		rec.Origin = i.Origin.Kind
	}
	if !i.ReceivedAt.IsZero() {
		at := i.ReceivedAt
		rec.ReceivedAt = &at
	}
	// Rune-safe: a byte cut would halve a multi-byte character.
	if runes := []rune(text); len(runes) > MaxRecordedInputText {
		rec.Text, rec.Truncated = string(runes[:MaxRecordedInputText]), true
	} else {
		rec.Text = text
	}
	return rec
}

// DeliveredTo reports whether the run's reply already reached a channel.
func (r *Run) DeliveredTo(channel string) bool {
	for _, c := range r.Delivered {
		if c == channel {
			return true
		}
	}
	return false
}

// MutateMaxAttempts bounds Mutate's retry on ErrConflict.
const MutateMaxAttempts = 10

// Mutate is the get-modify-Update closure every status or metadata write goes
// through: read the record, let fn change it, write it back with the revision
// it was read at, and read again when another writer got there first. It
// replaces a merge patch, which never conflicted — fn therefore runs again on
// the fresh record and must derive its change from the record it is handed,
// not from a snapshot it captured earlier.
//
// fn leaving the record unchanged writes nothing, as a merge patch with no
// difference did. An error from fn stops the loop and is returned as is. The
// returned record is the one written (or, when nothing changed, the one read).
func Mutate(ctx context.Context, s Store, key Key, fn func(*ConversationRecord) error) (ConversationRecord, error) {
	var lastErr error
	for attempt := 0; attempt < MutateMaxAttempts; attempt++ {
		rec, rev, err := s.Get(ctx, key)
		if err != nil {
			return ConversationRecord{}, err
		}
		before, err := json.Marshal(rec)
		if err != nil {
			return ConversationRecord{}, fmt.Errorf("state: mutate %q: snapshot: %w", key, err)
		}
		if err := fn(&rec); err != nil {
			return ConversationRecord{}, err
		}
		after, err := json.Marshal(rec)
		if err != nil {
			return ConversationRecord{}, fmt.Errorf("state: mutate %q: snapshot: %w", key, err)
		}
		if bytes.Equal(before, after) {
			return rec, nil
		}
		if _, err = s.Update(ctx, key, rev, rec); err == nil {
			return rec, nil
		}
		if !errors.Is(err, ErrConflict) {
			return ConversationRecord{}, err
		}
		lastErr = err
	}
	return ConversationRecord{}, fmt.Errorf("state: mutate %q: exhausted %d attempts: %w", key, MutateMaxAttempts, lastErr)
}

// Condition statuses, the values metav1.ConditionStatus carries.
const (
	ConditionTrue    = "True"
	ConditionFalse   = "False"
	ConditionUnknown = "Unknown"
)

// FindCondition returns the condition of a type, or nil. The pointer aliases
// the record's own slice.
func (r *ConversationRecord) FindCondition(condType string) *Condition {
	for i := range r.Conditions {
		if r.Conditions[i].Type == condType {
			return &r.Conditions[i]
		}
	}
	return nil
}

// SetCondition adds or updates a condition with apimeta.SetStatusCondition's
// semantics: LastTransitionTime moves only when the status does (stamped now
// when the caller left it zero), the rest is overwritten. It reports whether
// anything changed.
func (r *ConversationRecord) SetCondition(c Condition) bool {
	existing := r.FindCondition(c.Type)
	if existing == nil {
		if c.LastTransitionTime.IsZero() {
			c.LastTransitionTime = time.Now().UTC()
		}
		r.Conditions = append(r.Conditions, c)
		return true
	}
	changed := false
	if existing.Status != c.Status {
		existing.Status = c.Status
		if c.LastTransitionTime.IsZero() {
			c.LastTransitionTime = time.Now().UTC()
		}
		existing.LastTransitionTime = c.LastTransitionTime
		changed = true
	}
	if existing.Reason != c.Reason {
		existing.Reason, changed = c.Reason, true
	}
	if existing.Message != c.Message {
		existing.Message, changed = c.Message, true
	}
	if existing.ObservedGeneration != c.ObservedGeneration {
		existing.ObservedGeneration, changed = c.ObservedGeneration, true
	}
	return changed
}

// RemoveCondition drops a condition of a type, reporting whether one was there.
func (r *ConversationRecord) RemoveCondition(condType string) bool {
	for i := range r.Conditions {
		if r.Conditions[i].Type == condType {
			r.Conditions = append(r.Conditions[:i], r.Conditions[i+1:]...)
			return true
		}
	}
	return false
}

// Patch is Mutate for a caller holding the record: fn is applied to a fresh
// read of rec.Name, and rec is replaced with what was written, so the caller's
// copy reads the way it did after a merge patch — as the stored state.
func Patch(ctx context.Context, s Store, rec *ConversationRecord, fn func(*ConversationRecord)) error {
	updated, err := Mutate(ctx, s, Key(rec.Name), func(r *ConversationRecord) error {
		fn(r)
		return nil
	})
	if err != nil {
		return err
	}
	*rec = updated
	return nil
}

// IgnoreNotFound turns ErrNotFound into nil, the way client.IgnoreNotFound
// does for the apiserver's own.
func IgnoreNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
