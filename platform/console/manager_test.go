package main

import (
	"strings"
	"testing"
)

// A SIGNAL CARD IS FOUR THINGS, NOT ONE PARAGRAPH.
//
// It used to join title, attribution, labels and the payload with single
// newlines and no fence. Markdown reflows that into one running line with a raw
// JSON document embedded mid-sentence — which is what an operator actually saw.
func TestSignalCardIsStructured(t *testing.T) {
	m := &OpMessage{
		Kind: "signal", Title: "Unhealthy: share-manager", Source: "cluster-events",
		Pipeline: "k8s-ops",
		Labels: map[string]string{
			"namespace": "longhorn-system", "severity": "Warning",
			"alertname": "Unhealthy",      // already the title
			"source":    "cluster-events", // already the source line
		},
		Body:     "{\n  \"reason\": \"Unhealthy\"\n}",
		InputRef: "conv-in-1",
	}
	got := m.Render()

	// WHERE IT CAME FROM is labelled and on its own line. It is the first thing
	// asked of an alert, and it used to be a scrap of italic text swallowed by
	// the chip run after it.
	if !strings.Contains(got, "**Source** `cluster-events`") {
		t.Errorf("the signal source is not stated plainly:\n%s", got)
	}
	if !strings.Contains(got, "**Pipeline** `k8s-ops`") {
		t.Errorf("the route is not stated plainly:\n%s", got)
	}

	// LABELS ARE A TABLE, one row each — not a run of k=v on one line.
	if !strings.Contains(got, "| label | value |") {
		t.Errorf("labels are not a table:\n%s", got)
	}
	if !strings.Contains(got, "| `namespace` | longhorn-system |") {
		t.Errorf("label row missing:\n%s", got)
	}

	// LABELS THE CARD ALREADY STATED ARE DROPPED. Suppressed by value, so an
	// adapter with different label NAMES gets the same treatment.
	if strings.Contains(got, "| `alertname` |") {
		t.Error("alertname repeats the title and must be dropped")
	}
	if strings.Contains(got, "| `source` |") {
		t.Error("source repeats the source line and must be dropped")
	}

	// THE PAYLOAD IS NOT IN THE CARD. It travels apart so the browser can put
	// it behind a disclosure control, the way Telegram folds it into an
	// expandable quote.
	if strings.Contains(got, "reason") {
		t.Errorf("the payload is still inline in the card:\n%s", got)
	}
	if p := m.SignalPayload(); !strings.Contains(p, "\"reason\": \"Unhealthy\"") {
		t.Errorf("the payload did not travel separately: %q", p)
	}
	// Blank lines are what make these separate blocks rather than one sentence.
	if n := strings.Count(got, "\n\n"); n < 3 {
		t.Errorf("card has %d block breaks, want the parts separated:\n%s", n, got)
	}
}

// Only a SIGNAL has a payload to fold. An answer is prose to read.
func TestOnlySignalsCarryAPayload(t *testing.T) {
	for _, kind := range []string{"answer", "relay", "notice"} {
		m := &OpMessage{Kind: kind, Body: "some text"}
		if p := m.SignalPayload(); p != "" {
			t.Errorf("%s offered a payload to fold: %q", kind, p)
		}
	}
}

// A payload carrying its own fence must not close ours early.
func TestFenceNests(t *testing.T) {
	got := fence("see ```go\nx := 1\n```")
	if !strings.HasPrefix(got, "````") {
		t.Fatalf("fence did not lengthen to contain the payload:\n%s", got)
	}
}

// A ONE-LINE PAYLOAD IS NOT A DOCUMENT.
//
// A posted task is a sentence somebody wrote. Fenced, it renders as prose in a
// monospaced box that scrolls sideways — the machine-document treatment applied
// to something that is not one.
func TestOneLineProseIsNotFenced(t *testing.T) {
	if got := fence("Report on the longhorn-system namespace, please."); strings.Contains(got, "```") {
		t.Fatalf("a one-line prose payload was fenced: %q", got)
	}
	// A multi-line one still is: it has a shape worth preserving.
	if got := fence("line one\nline two"); !strings.Contains(got, "```") {
		t.Fatalf("a multi-line payload lost its fence: %q", got)
	}
}
