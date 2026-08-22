package main

import (
	"encoding/json"
	"testing"
)

func rec(logger, message string) *logRecord {
	return &logRecord{
		Name:    logger,
		Message: []string{message},
		Level:   "ERROR",
		Source:  []json.RawMessage{json.RawMessage(`"custom_components/x/__init__.py"`), json.RawMessage(`1`)},
	}
}

// Mechanism 1 needs no read, so it must hold on a NIL excluder — before any
// session has authenticated, which is exactly when a burst is most likely to be
// in flight. The failure this guards against must not be reachable by
// forgetting to wire the excluder up.
func TestMarkerHoldsWithNoSession(t *testing.T) {
	var cold *selfExcluder
	for _, r := range []*logRecord{
		rec("custom_components.agentops.sensor", "boom"),
		rec("homeassistant.components.script", "Error running script agent-ops nightly"),
	} {
		if excluded, why := cold.Excludes(r); !excluded {
			t.Fatalf("a nil excluder must still apply the marker rule (%q): why=%q", r.Name, why)
		}
	}
}

// Mechanism 2 is the one that actually breaks the loop: a failed agent call is
// logged by the surface the agent called through, and reporting it wakes the
// agent that made it.
func TestAgentSurfaceIsExcludedWithNoSession(t *testing.T) {
	var cold *selfExcluder
	for _, logger := range []string{
		"homeassistant.components.mcp_server",
		"homeassistant.components.websocket_api.http",
		"homeassistant.components.api",
	} {
		if excluded, _ := cold.Excludes(rec(logger, "Error handling request")); !excluded {
			t.Fatalf("%q is an agent surface and must be excluded", logger)
		}
	}
}

// Home Assistant's own voice assistant is used by PEOPLE. Silencing it would
// cost real errors to buy nothing.
func TestConversationIsNotAnAgentSurface(t *testing.T) {
	var cold *selfExcluder
	if excluded, why := cold.Excludes(rec("homeassistant.components.conversation", "intent failed")); excluded {
		t.Fatalf("conversation must not be excluded: %s", why)
	}
}

// Mechanism 3 is precise and needs one read. Before that read it is simply off
// — which is why it is not the only mechanism.
func TestOwnUserNeedsTheRead(t *testing.T) {
	s := newSelfExcluder()
	r := rec("homeassistant.components.http.ban",
		"Login attempt or request with invalid authentication from agentops-bot")
	if excluded, _ := s.Excludes(rec("homeassistant.components.zwave_js", "Login failed for ops-bot")); excluded {
		t.Fatal("nothing should be excluded by user before the read")
	}
	s.learnUser(currentUser{ID: "abc123", Name: "ops-bot"})
	if excluded, _ := s.Excludes(rec("homeassistant.components.zwave_js", "Login failed for ops-bot")); !excluded {
		t.Fatal("a record naming this adapter's own user must be excluded once learned")
	}
	// The marker rule already covered the record above, independently.
	if excluded, _ := s.Excludes(r); !excluded {
		t.Fatal("marker rule regression")
	}
}

func TestOrdinaryRecordsPass(t *testing.T) {
	s := newSelfExcluder()
	s.learnUser(currentUser{ID: "abc123", Name: "ops-bot"})
	if excluded, why := s.Excludes(rec("homeassistant.components.zwave_js", "Failed to set temperature")); excluded {
		t.Fatalf("an ordinary record must pass: %s", why)
	}
}

func TestLearnUserIgnoresEmpty(t *testing.T) {
	s := newSelfExcluder()
	s.learnUser(currentUser{})
	if excluded, _ := s.Excludes(rec("homeassistant.components.hue", "")); excluded {
		t.Fatal("an empty user must not exclude everything")
	}
}
