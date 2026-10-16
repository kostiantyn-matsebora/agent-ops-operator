package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"text/template"
)

// jsonTagsOf returns the json tag names of a struct's fields, used here to
// compare the parser's expected field set against the template's rendered
// one without hand-maintaining a second copy of either list.
func jsonTagsOf(v any) []string {
	t := reflect.TypeOf(v)
	tags := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// TestWebhookTemplateMatchesParser is the pinned fixture the
// pulse-signal-adapter spec's "A pinned fixture would catch drift" scenario
// requires: it renders pulseWebhookTemplate with a sample context and
// asserts the rendered JSON's field set is EXACTLY pulseEvent's json tag
// set — no field the template renders that the parser does not expect, and
// no field the parser expects that the template does not render. A future
// edit to either half without the other fails here, at build time, rather
// than at the first malformed webhook.
func TestWebhookTemplateMatchesParser(t *testing.T) {
	tmpl, err := template.New("pulse-webhook").Parse(pulseWebhookTemplate)
	if err != nil {
		t.Fatalf("pulseWebhookTemplate does not parse: %v", err)
	}
	ctx := pulseTemplateContext{
		Type: "alert", ID: "abc123", ResourceName: "vm-101", ResourceType: "vm",
		Node: "pve1", Level: "critical", Message: "CPU high", Value: "97",
		Timestamp: "2026-01-01T00:00:00Z",
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		t.Fatalf("pulseWebhookTemplate failed to render: %v", err)
	}

	var rendered map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &rendered); err != nil {
		t.Fatalf("rendered template is not valid JSON: %v\nrendered body:\n%s", err, buf.String())
	}
	renderedFields := make([]string, 0, len(rendered))
	for k := range rendered {
		renderedFields = append(renderedFields, k)
	}
	sort.Strings(renderedFields)

	expectedFields := jsonTagsOf(pulseEvent{})
	if !reflect.DeepEqual(renderedFields, expectedFields) {
		t.Fatalf("template field set and parser field set have drifted apart:\n  template renders: %v\n  parser expects:   %v",
			renderedFields, expectedFields)
	}

	// The rendered body must also actually decode into pulseEvent with every
	// value round-tripping — the field-set comparison above could pass while
	// a type mismatch (e.g. a field template renders as a JSON number where
	// the parser wants a string) still broke decoding silently.
	var parsed pulseEvent
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("rendered template does not decode into pulseEvent: %v", err)
	}
	want := pulseEvent{
		Event: "alert", ID: "abc123", Resource: "vm-101", ResourceType: "vm",
		Node: "pve1", Severity: "critical", Message: "CPU high", Value: "97",
		StartsAt: "2026-01-01T00:00:00Z",
	}
	if parsed != want {
		t.Fatalf("rendered-then-parsed value mismatch:\n  got:  %+v\n  want: %+v", parsed, want)
	}
}

func TestParsePulseEventRejectsMissingFields(t *testing.T) {
	if _, err := parsePulseEvent([]byte(`{"resource":"vm-1"}`)); err == nil {
		t.Fatal("a body with no event field must be rejected")
	}
	if _, err := parsePulseEvent([]byte(`{"event":"alert"}`)); err == nil {
		t.Fatal("a body with no resource field must be rejected")
	}
	if _, err := parsePulseEvent([]byte(`not json`)); err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
}

func TestParsePulseEventAcceptsWellFormedBody(t *testing.T) {
	e, err := parsePulseEvent([]byte(firingBody("x")))
	if err != nil {
		t.Fatalf("well-formed firing body should parse: %v", err)
	}
	if e.Event != "alert" || e.Resource != "vm-101" {
		t.Fatalf("parsed fields: %+v", e)
	}
}
