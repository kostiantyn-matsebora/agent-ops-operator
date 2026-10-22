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
	"crypto/sha256"
	"encoding/hex"
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
	// WebhookURL is the URL registered WITH Pulse as the delivery target.
	// This adapter is a dumb transport — it never derives a URL of its own
	// (same posture as channels/telegram and signals/ha: no connectivity
	// information lives on the adapter, only on the source), so this is
	// required for registration exactly like BaseURL is: a source carrying
	// one but not the other still serves, reporting
	// RegistrationNotConfigured.
	//
	// The bundle chart computes this for the one source it ships (the
	// in-cluster Service DNS name, http://agentops-signal-<adapter>.<ns>
	// .svc:<port>/webhook/<source>) — see chart/charts/pulse/templates
	// /signal.yaml. A hand-authored source must set it explicitly.
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
	// WebhookHeaders are registered as the Generic webhook's own Headers
	// field (confirmed real against Pulse's own
	// internal/notifications/notifications.go WebhookConfig.Headers) —
	// static headers PULSE attaches to every delivery to this adapter.
	//
	// HMAC verification needs no header Pulse's own source doesn't already
	// send unconditionally. This exists for infra reasons HMAC does not
	// reach — a WAF or gateway in front of this adapter's Ingress requiring
	// its own shared-secret header, for instance. Pulse's own signing
	// headers win on a name collision.
	//
	// Pulse masks every header VALUE on read, so create-or-repair compares
	// these by KEY only (comparable). A changed value under an unchanged key
	// is never seen as drift and needs a manual re-registration in Pulse.
	WebhookHeaders map[string]string `json:"webhookHeaders,omitempty"`
	// CorrelationStrategy selects how initiationID (payload.go) folds Pulse's
	// alert id into this source's signals — "id" or "id+startTime". Resolved
	// once per registryLoop poll tick via normalizeCorrelationStrategy below,
	// same cadence as the signing secret, never re-derived per webhook
	// request.
	CorrelationStrategy string `json:"correlationStrategy,omitempty"`
}

// normalizeCorrelationStrategy resolves a served source's raw
// correlationStrategy config value to one of the two strategies
// pulseEvent.initiationID understands. This is the one place the default
// lives: the explicit opt-in "id" is passed through verbatim, and
// everything else — absent, empty, or any unrecognized value — degrades to
// "id+startTime" rather than rejecting. There is no HTTP request to return
// a 4xx against here (the value is read from a polled SignalSource), so a
// typo degrades to the safer, corrected default instead of silently
// reinstating the old collapsing behavior or crashing the adapter
// (design.md's "Unrecognized or empty value defaults rather than rejects").
func normalizeCorrelationStrategy(raw string) string {
	if raw == correlationStrategyID {
		return correlationStrategyID
	}
	return correlationStrategyIDStartTime
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
// both keys of the one Secret credentialsSecretRef names. One documented
// shape: `token` and `signingSecret`, exactly as the chart renders them.
func pulseCredential(info SourceInfo) (token, signingSecret string) {
	if info.CredentialEnvPrefix == "" {
		return "", ""
	}
	return os.Getenv(info.CredentialEnvPrefix + "token"), os.Getenv(info.CredentialEnvPrefix + "signingSecret")
}

// reconcileRegistration attempts one registration cycle for a served source
// and reports the resulting Ready state. Returns the resolved signing secret
// (possibly "") and the resolved correlationStrategy (always one of
// normalizeCorrelationStrategy's two canonical values) so the caller can
// cache both for the duration of one refresh cycle — signing secret for
// inbound signature verification, correlationStrategy for initiationID
// derivation — neither ever re-derived per webhook request. Exposed to
// tests directly to drive a deterministic failure-then-success sequence
// without waiting on registryLoop's ticker.
//
// The signing secret is resolved FIRST and returned on every path:
// verification follows the secret, never the registration
// (pulse-signal-adapter spec). A source whose config is absent, incomplete
// or invalid still enforces its configured secret, so a hand-registered
// Pulse presenting that secret verifies exactly as an adapter-registered
// one does, and an unsigned post is refused rather than accepted.
func (a *adapter) reconcileRegistration(ctx context.Context, info SourceInfo) (signingSecret, correlationStrategy string) {
	token, signingSecret := pulseCredential(info)
	cfg, err := parseRegisterConfig(info.Config)
	if err != nil {
		a.report(ctx, info.Name, "InvalidConfig", err.Error())
		return signingSecret, normalizeCorrelationStrategy("")
	}
	raw := ""
	if cfg != nil {
		raw = cfg.CorrelationStrategy
	}
	correlationStrategy = normalizeCorrelationStrategy(raw)
	if cfg == nil || cfg.BaseURL == "" || cfg.WebhookURL == "" {
		a.report(ctx, info.Name, "RegistrationNotConfigured",
			"served by signal-pulse — set spec.config.baseUrl and spec.config.webhookUrl (and a credentialsSecretRef "+
				"holding a Pulse API token) to register this webhook with Pulse automatically")
		return signingSecret, correlationStrategy
	}
	client := newPulseClient(cfg.BaseURL, token)
	desired := desiredWebhook(info.Name, cfg.WebhookURL, signingSecret, cfg.WebhookHeaders)
	if err := client.ensureWebhook(ctx, desired); err != nil {
		a.report(ctx, info.Name, "PulseRegistrationFailed",
			fmt.Sprintf("served, but could not register the webhook with Pulse: %v", err))
		return signingSecret, correlationStrategy
	}
	a.report(ctx, info.Name, "PulseRegistered",
		fmt.Sprintf("served by signal-pulse — Pulse posts alerts to %s", cfg.WebhookURL))
	return signingSecret, correlationStrategy
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

// pulseWebhookConfig is Pulse's own WebhookConfig object shape
// (internal/notifications/notifications.go in Pulse's own source), as far
// as this adapter needs it. Fields this adapter never sets — Method
// (defaults to POST), Service (platform-specific formatting for
// discord/slack/teams, irrelevant to a custom Template), CustomFields,
// TagFilter and friends — are left off the struct entirely rather than
// guessed at.
type pulseWebhookConfig struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Template string `json:"template"`
	// Headers are attached by PULSE to every delivery to this adapter —
	// e.g. a static header a WAF or gateway in front of the adapter
	// requires, independent of the HMAC signing below. Never how a caller
	// authenticates INTO Pulse; this is the opposite direction.
	Headers       map[string]string `json:"headers,omitempty"`
	SigningSecret string            `json:"signingSecret,omitempty"`
	Enabled       bool              `json:"enabled"`
}

// webhookName is the deterministic identity this adapter owns in Pulse's
// webhook list, mirroring signals/alertmanager's registrationName.
func webhookName(source string) string { return "agentops-" + source }

// secretDigestParam is the query parameter on the registered webhook URL
// carrying a one-way digest of the signing secret. It is what lets the
// create-or-repair comparison see a rotated secret at all.
//
// It lives on the URL because the URL is the one field that both carries
// data and round-trips unmasked. Pulse's GetWebhooks → maskedWebhookResponse
// masks the stored signingSecret AND every registered header value as
// "***REDACTED***" on read, so a digest header would read back as constant
// drift. Name is the lookup key and Template is the notification body, so
// neither can carry it. The inbound mux routes on method and path only
// (newMux), so the parameter never changes how a delivery is served.
//
// Internal to this adapter, never part of the published contract.
const secretDigestParam = "agentops_secret_digest"

// redactedHeaderValue stands in for every header value on both sides of the
// comparison, matching what Pulse itself returns on read.
const redactedHeaderValue = "***REDACTED***"

// secretDigest renders secretDigestParam's value: "sha256:<hex>" of the
// signing secret. One-way, and sent only to the Pulse instance that already
// holds the secret itself.
func secretDigest(signingSecret string) string {
	sum := sha256.Sum256([]byte(signingSecret))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// desiredWebhook renders the webhook object this adapter wants registered.
func desiredWebhook(source, webhookURL, signingSecret string, headers map[string]string) pulseWebhookConfig {
	return pulseWebhookConfig{
		Name:          webhookName(source),
		URL:           signedWebhookURL(webhookURL, signingSecret),
		Template:      pulseWebhookTemplate,
		Headers:       headers,
		SigningSecret: signingSecret,
		Enabled:       true,
	}
}

// signedWebhookURL is the URL registered with Pulse: webhookURL with
// secretDigestParam set beside any query it already carries. No secret, no
// parameter, so dropping the secret is itself drift the comparison repairs.
// An unparseable URL is returned unchanged — parseRegisterConfig already
// refused it, so this path is unreachable from a served source.
func signedWebhookURL(webhookURL, signingSecret string) string {
	if signingSecret == "" {
		return webhookURL
	}
	u, err := url.Parse(webhookURL)
	if err != nil {
		return webhookURL
	}
	q := u.Query()
	q.Set(secretDigestParam, secretDigest(signingSecret))
	u.RawQuery = q.Encode()
	return u.String()
}

// comparable reduces a webhook to what Pulse actually echoes back on read,
// so idempotence comparison never flaps on a field it can never observe.
//   - The signing secret is dropped: Pulse masks it.
//   - Header values are replaced by Pulse's own mask: Pulse masks every
//     value, keeping only the keys. A header added or removed in
//     webhookHeaders still shows as drift.
//   - The URL is kept verbatim: it carries secretDigestParam, the field
//     through which a rotated secret shows as drift.
func (c pulseWebhookConfig) comparable() pulseWebhookConfig {
	c.ID = ""
	c.SigningSecret = ""
	if len(c.Headers) > 0 {
		keys := make(map[string]string, len(c.Headers))
		for k := range c.Headers {
			keys[k] = redactedHeaderValue
		}
		c.Headers = keys
	}
	return c
}

// ensureWebhook creates or repairs this adapter's Generic webhook in Pulse.
// Idempotent: a webhook already matching the desired body (sans secret and
// header values, which Pulse masks, but with the secret's digest on the URL)
// is left alone. One that differs is updated in place — matching
// signals/alertmanager's create-or-repair posture for its own sender object.
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
