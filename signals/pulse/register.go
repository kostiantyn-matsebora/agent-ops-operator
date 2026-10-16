package main

// Pulse-side webhook registration — the ONE Pulse-specific thing in this
// module, mirroring signals/alertmanager/register.go's split between a
// vendor-neutral receiving adapter and a vendor-specific sender call.
//
// Pulse sends outgoing webhooks only (design.md Decision 1) — there is no
// receiver-registration concept on THIS adapter's side. "Registering" means
// calling PULSE'S OWN API (POST /api/notifications/webhooks, the Generic
// service) to tell Pulse where to post. Unlike signals/alertmanager, this
// adapter's registration config is read straight from the pod's own
// environment (PULSE_URL, PULSE_API_TOKEN, PULSE_SOURCE_NAME,
// PULSE_SIGNING_SECRET), rendered there by the bundle chart — never from a
// SignalSource (design.md Decision 1, pulse-signal-adapter spec).
//
// Registration runs once at startup and keeps re-running on a fixed
// interval afterward (registerLoop), never crash-looping on failure. A
// failed attempt degrades the source's Ready reason; a later successful
// retry heals it with no pod restart.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// registerInterval is the fixed interval between registration attempts —
// not exponential backoff. Pulse coming back up, or a rotated API token
// taking effect, is the common case this loop exists for, and a modest
// fixed interval heals that well under a minute without hammering Pulse
// during a longer outage.
const registerInterval = 30 * time.Second

// pulseRegistration is this adapter's own registration inputs, read once
// from the pod's environment at startup.
type pulseRegistration struct {
	BaseURL       string // PULSE_URL
	APIToken      string // PULSE_API_TOKEN
	SourceName    string // PULSE_SOURCE_NAME — also the source this adapter's single registration reports status against
	SigningSecret string // PULSE_SIGNING_SECRET, optional — "" means unsigned mode
}

// loadRegistration reads the registration config from the pod's own
// environment. An empty BaseURL or SourceName means "nothing configured to
// register" — the webhook endpoint still serves either way.
func loadRegistration() pulseRegistration {
	return pulseRegistration{
		BaseURL:       strings.TrimSuffix(os.Getenv("PULSE_URL"), "/"),
		APIToken:      os.Getenv("PULSE_API_TOKEN"),
		SourceName:    os.Getenv("PULSE_SOURCE_NAME"),
		SigningSecret: os.Getenv("PULSE_SIGNING_SECRET"),
	}
}

// registerLoop keeps this adapter's Pulse-side webhook registered, forever.
// Every tick re-attempts and re-reports, so a fixed interval is all the
// retry policy needed — no crash-loop, no one-shot startup attempt that
// could leave a pod permanently unregistered (pulse-signal-adapter spec,
// "Pulse unreachable at startup still serves the webhook").
func (a *adapter) registerLoop(ctx context.Context) {
	for ctx.Err() == nil {
		a.reconcileRegistration(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(registerInterval):
		}
	}
}

// reconcileRegistration attempts one registration cycle and reports the
// resulting Ready state. Exposed to tests directly (bypassing the loop's
// ticker) to drive a deterministic failure-then-success sequence.
func (a *adapter) reconcileRegistration(ctx context.Context) {
	if a.reg.SourceName == "" {
		// nothing to register, and no source identity to report status
		// against — a pod with no registration env configured at all.
		return
	}
	if a.reg.BaseURL == "" {
		a.report(ctx, a.reg.SourceName, "RegistrationNotConfigured",
			"served, but PULSE_URL is not set — configure the bundle chart's registration env to register with Pulse")
		return
	}

	url := a.endpoint(a.reg.SourceName)
	client := newPulseClient(a.reg.BaseURL, a.reg.APIToken)
	desired := desiredWebhook(a.reg.SourceName, url, a.reg.SigningSecret)
	if err := client.ensureWebhook(ctx, desired); err != nil {
		a.report(ctx, a.reg.SourceName, "PulseRegistrationFailed",
			fmt.Sprintf("served, but could not register the webhook with Pulse: %v", err))
		return
	}
	a.report(ctx, a.reg.SourceName, "PulseRegistered",
		fmt.Sprintf("served by signal-pulse — Pulse posts alerts to %s", url))
}

// endpoint is this adapter's webhook URL for a source (in-cluster form when
// the reconciler injected the pod namespace; path-only otherwise) — mirrors
// signals/alertmanager's own.
func (a *adapter) endpoint(source string) string {
	if a.podNS == "" {
		return "<service>/webhook/" + source
	}
	port := strings.TrimPrefix(a.listen, ":")
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("http://agentops-signal-%s.%s.svc:%s/webhook/%s", a.sourceType, a.podNS, port, source)
}

// pulseClient is a minimal Pulse REST client, stdlib only (this module stays
// dependency-free). Auth follows the bearer-token shape design.md's Decision
// 3 already assumes for Pulse's scoped API tokens.
type pulseClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newPulseClient(baseURL, token string) *pulseClient {
	return &pulseClient{baseURL: baseURL, token: token, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *pulseClient) do(ctx context.Context, method, path string, in any) (int, []byte, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, b, nil
}

// pulseWebhookConfig is Pulse's own Generic notification-webhook object
// shape, as far as this adapter needs it: enough to create or repair the
// receiver pointing at this adapter's own /webhook/<source> endpoint.
type pulseWebhookConfig struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Type          string `json:"type"` // "generic"
	URL           string `json:"url"`
	Template      string `json:"template"`
	SigningSecret string `json:"signingSecret,omitempty"`
	Enabled       bool   `json:"enabled"`
}

// webhookName is the deterministic identity this adapter owns in Pulse's
// webhook list, mirroring signals/alertmanager's registrationName.
func webhookName(source string) string { return "agentops-" + source }

// desiredWebhook renders the webhook object this adapter wants registered.
func desiredWebhook(source, webhookURL, signingSecret string) pulseWebhookConfig {
	return pulseWebhookConfig{
		Name:          webhookName(source),
		Type:          "generic",
		URL:           webhookURL,
		Template:      pulseWebhookTemplate,
		SigningSecret: signingSecret,
		Enabled:       true,
	}
}

// comparable strips fields Pulse may not echo back on read — the signing
// secret is sensitive and commonly redacted — so idempotence comparison
// never flaps on a field it can never actually observe.
func (c pulseWebhookConfig) comparable() pulseWebhookConfig {
	c.ID = ""
	c.SigningSecret = ""
	return c
}

// ensureWebhook creates or repairs this adapter's Generic webhook in Pulse.
// Idempotent: a webhook already matching the desired body (sans secret) is
// left alone; one that differs is updated in place — matching
// signals/alertmanager's create-or-repair posture for its own sender
// object.
func (c *pulseClient) ensureWebhook(ctx context.Context, desired pulseWebhookConfig) error {
	code, body, err := c.do(ctx, "GET", "/api/notifications/webhooks", nil)
	if err != nil {
		return fmt.Errorf("list webhooks: %v", err)
	}
	if code >= 400 {
		return fmt.Errorf("list webhooks: %d %s", code, bytes.TrimSpace(body))
	}
	var existing []pulseWebhookConfig
	if err := json.Unmarshal(body, &existing); err != nil {
		return fmt.Errorf("list webhooks: unreadable response: %v", err)
	}

	for _, cur := range existing {
		if cur.Name != desired.Name {
			continue
		}
		if specEqual(cur.comparable(), desired.comparable()) {
			return nil
		}
		desired.ID = cur.ID
		code, body, err = c.do(ctx, "PUT", "/api/notifications/webhooks/"+cur.ID, desired)
		if err != nil {
			return fmt.Errorf("update webhook %s: %v", cur.ID, err)
		}
		if code >= 400 {
			return fmt.Errorf("update webhook %s: %d %s", cur.ID, code, bytes.TrimSpace(body))
		}
		return nil
	}

	code, body, err = c.do(ctx, "POST", "/api/notifications/webhooks", desired)
	if err != nil {
		return fmt.Errorf("create webhook: %v", err)
	}
	if code >= 400 {
		return fmt.Errorf("create webhook: %d %s", code, bytes.TrimSpace(body))
	}
	return nil
}

// specEqual compares two values by their JSON encoding (map keys sort
// deterministically through the encoder), mirroring
// signals/alertmanager's own specEqual.
func specEqual(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(ja, jb)
}
