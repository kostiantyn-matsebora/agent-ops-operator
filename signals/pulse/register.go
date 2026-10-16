package main

// Pulse-side webhook registration — the ONE Pulse-specific thing in this
// module, mirroring signals/alertmanager/register.go's split between a
// vendor-neutral receiving adapter and a vendor-specific sender call.
//
// Pulse sends outgoing webhooks only (design.md Decision 1) — there is no
// receiver-registration concept on THIS adapter's side. "Registering" means
// calling PULSE'S OWN API (POST /api/notifications/webhooks, the Generic
// service) to tell Pulse where to post.
//
// Registration config comes from the SERVED SOURCE itself, not a pod-wide
// env var: the SignalAdapter CRD has no generic env/envFrom passthrough
// (platform/manager/api/v1alpha1/signaladapter_types.go — image, servedBy,
// port, serviceAccountName, singleton, resources, configSchema,
// credentialKeys, externals, and nothing else), so a literal PULSE_URL/
// PULSE_API_TOKEN the bundle chart "renders onto the pod" has no delivery
// path. The one mechanism that DOES reach this pod is the one every other
// signal adapter already uses: SignalSource.spec.config (schema-less JSON,
// polled via GET /signal/sources?adapter=) for the non-sensitive base URL,
// and SignalSource.spec.credentialsSecretRef (kubelet-resolved envFrom,
// never read by the manager) for the API token and the optional signing
// secret — exactly signals/ha's `credentialToken` pattern. This bundle
// still ships exactly one default SignalSource (design.md Decision 1's
// actual goal — no generalized, hand-authored-source registration path),
// it just reads that one source's own config/credential rather than a pod
// env var nothing could set.
//
// Registration runs on every source-list poll tick (main.go's
// registryLoop), never crash-looping on failure. A failed attempt degrades
// the source's Ready reason; a later successful retry heals it with no pod
// restart.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// registerConfig is the served SignalSource's spec.config, interpreted by
// this adapter alone (schema-less on the CRD; signals/alertmanager's
// registerSpec is the precedent for a signal type owning its own config
// shape).
type registerConfig struct {
	// BaseURL is Pulse's own base URL, e.g. https://pulse.example.org.
	BaseURL string `json:"baseUrl"`
	// WebhookURL overrides the URL registered WITH Pulse as the delivery
	// target. Empty derives the in-cluster Service DNS name
	// (http://agentops-signal-<adapter>.<ns>.svc:<port>/webhook/<source>),
	// which only resolves for a Pulse instance reachable from inside this
	// cluster's network.
	//
	// Pulse is commonly NOT in-cluster — it watches Proxmox/Docker hosts
	// that may sit entirely outside Kubernetes — so a Pulse that cannot
	// resolve cluster-internal DNS needs an externally reachable URL
	// instead: a LoadBalancer Service, a NodePort, an Ingress, or a tunnel
	// the operator already runs, pointed at this adapter's own
	// /webhook/<source> path. Exposing that is OUTSIDE this chart's
	// scope, same posture as every other operator-supplied endpoint here
	// (prometheus's query backend is never derived either) — this field
	// only says what to tell Pulse once the exposure exists.
	WebhookURL string `json:"webhookUrl,omitempty"`
}

// parseRegisterConfig extracts this adapter's config block. No config at
// all (nil/empty) is a valid, common state — the bundle's default source
// ships inert until the install sets baseUrl — and is reported as "not
// configured" rather than an error.
func parseRegisterConfig(raw json.RawMessage) (*registerConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var cfg registerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid spec.config: %v", err)
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	cfg.WebhookURL = strings.TrimSuffix(cfg.WebhookURL, "/")
	if cfg.BaseURL != "" {
		// the token is sent to this URL, so only a plain http(s) origin is accepted
		if err := validHTTPURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid spec.config: baseUrl %v", err)
		}
	}
	if cfg.WebhookURL != "" {
		// registered WITH Pulse verbatim, so the same shape check applies
		if err := validHTTPURL(cfg.WebhookURL); err != nil {
			return nil, fmt.Errorf("invalid spec.config: webhookUrl %v", err)
		}
	}
	return &cfg, nil
}

// validHTTPURL rejects anything that is not a plain http(s) origin with a
// host — no embedded userinfo, no scheme-less or opaque value a sender
// could misinterpret.
func validHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return fmt.Errorf("must be an http(s) URL with a host")
	}
	return nil
}

// pulseCredential resolves this source's projected credential: the Pulse
// API token (required to register) and the optional HMAC signing secret,
// both keys of the one Secret credentialsSecretRef names. Both casings are
// accepted, mirroring signals/ha's credentialToken — Secrets written by
// hand commonly use either.
func pulseCredential(info SourceInfo) (token, signingSecret string) {
	if info.CredentialEnvPrefix == "" {
		return "", ""
	}
	for _, key := range []string{"token", "TOKEN"} {
		if v := os.Getenv(info.CredentialEnvPrefix + key); v != "" {
			token = v
			break
		}
	}
	for _, key := range []string{"signingSecret", "SIGNING_SECRET"} {
		if v := os.Getenv(info.CredentialEnvPrefix + key); v != "" {
			signingSecret = v
			break
		}
	}
	return token, signingSecret
}

// reconcileRegistration attempts one registration cycle for a served source
// and reports the resulting Ready state. Returns the resolved signing
// secret (possibly "") so the caller can cache it for inbound signature
// verification — this adapter never stores it longer than one refresh
// cycle. Exposed to tests directly to drive a deterministic
// failure-then-success sequence without waiting on registryLoop's ticker.
func (a *adapter) reconcileRegistration(ctx context.Context, info SourceInfo) string {
	cfg, err := parseRegisterConfig(info.Config)
	if err != nil {
		a.report(ctx, info.Name, "InvalidConfig", err.Error())
		return ""
	}
	if cfg == nil || cfg.BaseURL == "" {
		a.report(ctx, info.Name, "RegistrationNotConfigured",
			fmt.Sprintf("served by signal-pulse — POST %s; set spec.config.baseUrl (and a credentialsSecretRef "+
				"holding a Pulse API token) to register this webhook with Pulse automatically", a.endpoint(info.Name)))
		return ""
	}
	token, signingSecret := pulseCredential(info)
	url := cfg.WebhookURL
	if url == "" {
		// No override: derive the in-cluster Service URL, which only
		// resolves for a Pulse reachable from inside this cluster's
		// network. An external Pulse needs spec.config.webhookUrl set to
		// whatever the operator exposed this adapter's /webhook/<source>
		// path as — see registerConfig's doc comment.
		url = a.endpoint(info.Name)
	}
	client := newPulseClient(cfg.BaseURL, token)
	desired := desiredWebhook(info.Name, url, signingSecret)
	if err := client.ensureWebhook(ctx, desired); err != nil {
		a.report(ctx, info.Name, "PulseRegistrationFailed",
			fmt.Sprintf("served, but could not register the webhook with Pulse: %v", err))
		return signingSecret
	}
	a.report(ctx, info.Name, "PulseRegistered",
		fmt.Sprintf("served by signal-pulse — Pulse posts alerts to %s", url))
	return signingSecret
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
		code, body, err = c.do(ctx, "PUT", "/api/notifications/webhooks/"+url.PathEscape(cur.ID), desired)
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
