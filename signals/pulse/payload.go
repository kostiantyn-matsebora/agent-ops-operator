package main

// The Pulse webhook payload: the Go-template JSON body this adapter
// registers with Pulse's Generic webhook service (register.go), and the
// parser that decodes it on receipt (main.go's handleWebhook). Both halves
// live in this one file on purpose — see design.md's "one module owns both
// halves" risk mitigation and the pulse-signal-adapter spec's "A pinned
// fixture would catch drift" scenario: letting the template drift from the
// parser's expected shape would fail silently at the first malformed
// webhook, rather than at build time.
//
// Pulse's exact webhook schema is not in this repository (design.md,
// Context — Pulse sends outgoing webhooks only, with a caller-supplied
// payload template). This is a deliberate, documented choice consistent
// with the "firing"/"resolved" semantics the adapter contract already uses:
// an "event" field carrying exactly "alert" (firing) or "resolved", plus the
// resource/alert identity fields a fingerprint needs.
//
// CONFIRMED against a real Pulse instance (v6.5.0) on 2026-10-17, via its own
// `GET /api/notifications/webhook-templates` response for the "generic"
// service: the lifecycle field is `.Event`, not `.Type` — `.Type` is the
// resource's metric type (cpu, diskRead, ...), a separate real field from
// `.ResourceType` (the virtualization object kind). The first version of
// this template used `.Type` for "event", which silently dropped every real
// alert (normalize() never saw "alert") — design.md's own Risks section had
// flagged this exact confirmation as still open.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// pulseWebhookTemplate is the Go-template JSON body registered verbatim as
// this webhook's payload template with Pulse's Generic notification
// service (register.go's desiredWebhook). Every field here has a matching
// json tag on pulseEvent below — TestWebhookTemplateMatchesParser pins that
// agreement.
const pulseWebhookTemplate = `{
  "event": "{{ .Event }}",
  "id": "{{ .ID }}",
  "resource": "{{ .ResourceName }}",
  "resourceType": "{{ .ResourceType }}",
  "node": "{{ .Node }}",
  "severity": "{{ .Level }}",
  "message": "{{ .Message }}",
  "value": "{{ .Value }}",
  "startsAt": "{{ .Timestamp }}"
}`

// pulseTemplateContext documents the fields pulseWebhookTemplate assumes
// Pulse's own notification context exposes when rendering a Generic webhook
// body (Event carrying "alert"/"resolved", the rest naming the resource and
// alert identity). Used only by this module's own pinned fixture test —
// Pulse renders the real thing with its own template engine.
type pulseTemplateContext struct {
	Event        string
	ID           string
	ResourceName string
	ResourceType string
	Node         string
	Level        string
	Message      string
	Value        string
	Timestamp    string
}

// pulseEvent is the parsed shape of an inbound Pulse webhook body, matching
// pulseWebhookTemplate field for field.
type pulseEvent struct {
	Event        string `json:"event"`
	ID           string `json:"id"`
	Resource     string `json:"resource"`
	ResourceType string `json:"resourceType"`
	Node         string `json:"node"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Value        string `json:"value"`
	StartsAt     string `json:"startsAt"`
}

// parsePulseEvent decodes one inbound webhook body against pulseEvent's
// shape. Event and Resource are the two fields every downstream use
// (firing/resolved routing, fingerprinting) depends on — a body missing
// either is not a Pulse webhook this adapter understands, so it is rejected
// here rather than normalized into a signal with a blank identity.
func parsePulseEvent(body []byte) (pulseEvent, error) {
	var e pulseEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return pulseEvent{}, err
	}
	if e.Event == "" {
		return pulseEvent{}, fmt.Errorf("missing event field")
	}
	if e.Resource == "" {
		return pulseEvent{}, fmt.Errorf("missing resource field")
	}
	return e, nil
}

// initiationID derives this signal's correlation id from the underlying
// Pulse alert occurrence: the resource plus Pulse's own alert id when the
// sender provides one, falling back to a hash of the
// resource/type/severity/message tuple when it does not (mirroring
// signals/alertmanager's groupLabelsFallbackID). startsAt and value are
// excluded on purpose, so a firing event and its later resolved pair for the
// same occurrence yield the same id, and a retried delivery of either does
// too — and two unrelated alerts (different resource/id) never collide.
func (e pulseEvent) initiationID() string {
	if e.ID != "" {
		return "pulse:" + e.Resource + ":" + e.ID
	}
	h := sha256.Sum256([]byte(strings.Join([]string{e.Resource, e.ResourceType, e.Severity, e.Message}, "|")))
	return "pulse:" + hex.EncodeToString(h[:])[:16]
}
