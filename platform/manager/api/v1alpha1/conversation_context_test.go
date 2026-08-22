package v1alpha1

import "testing"

// The rename must not do the harm the field exists to prevent. A status field
// that simply moved would strand every in-flight conversation's handle at the
// moment of upgrade, restarting the context of all of them — the exact failure
// this area is about, self-inflicted.

func TestContextIDPrefersTheCurrentField(t *testing.T) {
	c := &Conversation{}
	c.Status.RuntimeContextID = "ctx-new"
	c.Status.SessionID = "ctx-old"

	if got := c.ContextID(); got != "ctx-new" {
		t.Fatalf("ContextID() = %q, want the current field to win", got)
	}
}

func TestContextIDAdoptsTheRetiredField(t *testing.T) {
	// Written by a manager from before the rename: only the old spelling.
	c := &Conversation{}
	c.Status.SessionID = "ctx-old"

	if got := c.ContextID(); got != "ctx-old" {
		t.Fatalf("a conversation from before the rename lost its handle: %q", got)
	}
}

func TestSetContextIDWritesOnlyTheCurrentField(t *testing.T) {
	c := &Conversation{}
	c.Status.SessionID = "ctx-old"

	c.SetContextID("ctx-new")

	if c.Status.RuntimeContextID != "ctx-new" {
		t.Fatalf("RuntimeContextID = %q", c.Status.RuntimeContextID)
	}
	// The adopted conversation must stop carrying both, or the fallback would
	// keep resurrecting a stale handle after the deprecated field is removed.
	if c.Status.SessionID != "" {
		t.Fatalf("the retired field must be cleared on adoption, got %q", c.Status.SessionID)
	}
	if got := c.ContextID(); got != "ctx-new" {
		t.Fatalf("ContextID() after adoption = %q", got)
	}
}

func TestContextIDEmptyWhenNeitherIsSet(t *testing.T) {
	if got := (&Conversation{}).ContextID(); got != "" {
		t.Fatalf("a conversation with no handle must report none, got %q", got)
	}
}
