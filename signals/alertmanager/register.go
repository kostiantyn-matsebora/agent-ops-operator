package main

// Sender self-registration, and the ONE VictoriaMetrics-specific thing in this
// module.
//
// The adapter itself is vendor-neutral: it receives the standard Alertmanager
// webhook payload, which vanilla Alertmanager and VictoriaMetrics both send.
// This file is the exception, and it is exceptional because REGISTRATION is not
// standardised — VictoriaMetrics' operator takes a VMAlertmanagerConfig OBJECT,
// while vanilla Alertmanager's config is a FILE with nothing to write to. So the
// names below are VictoriaMetrics API names on purpose, and stay that way while
// the rest of the module carries none.
//
// A served SignalSource may carry a `register`
// block in its opaque config naming an in-cluster VMAlertmanager. The
// adapter then owns a VMAlertmanagerConfig (webhook receiver + continue
// route pointing at its own /webhook/<source>) in that namespace, so the
// sender is configured by the adapter — no manual alertmanager edits. The
// Kubernetes access is plain REST with the mounted SA token (stdlib only;
// the module stays dependency-free). Everything degrades to instructions:
// callers turn any error into a Ready message telling the operator exactly
// what to do by hand.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
)

// registerSpec is the `register` block of a source's opaque config: the whole
// sender-side routing decision for this source, so a SignalSource can fully
// replace a hand-written receiver + route.
//
// There is no endpoint here on purpose. Alertmanager has no API for adding a
// receiver — its config is assembled from VMAlertmanagerConfig objects by
// vm-operator — so registering means WRITING an object, not calling an
// address. The only endpoint involved points the other way (where alerts are
// pushed) and the adapter derives it from its own identity.
type registerSpec struct {
	// Namespace to write the VMAlertmanagerConfig into; empty = the adapter's
	// own namespace, which needs no cross-namespace permissions. Set it only
	// when the target VMAlertmanager restricts which namespaces it selects
	// configs from (`configNamespaceSelector`); with the common
	// `selectAllByDefault: true` any namespace works.
	Namespace string `json:"namespace,omitempty"`
	// Matchers select which alerts reach this source (Alertmanager matcher
	// syntax). Empty = everything that reaches the route.
	Matchers []string `json:"matchers,omitempty"`
	// Route timing, passed through verbatim (Alertmanager duration strings).
	GroupWait      string `json:"groupWait,omitempty"`
	GroupInterval  string `json:"groupInterval,omitempty"`
	RepeatInterval string `json:"repeatInterval,omitempty"`
	// MaxAlerts caps alerts per webhook post (0 = Alertmanager's default).
	MaxAlerts int `json:"maxAlerts,omitempty"`
	// SendResolved is a pointer so "unset" stays distinguishable from false:
	// unset omits the key and the sender's own default governs, like every
	// other knob in this block.
	SendResolved *bool `json:"sendResolved,omitempty"`
}

// parseRegister extracts the register block. An empty config, or one with no
// (or a null) `register` key, is ABSENT: (nil, nil). A block that is present
// but does not parse — a mistyped value, an unknown key, a non-object — is an
// error, never treated as absent: a typo'd key would otherwise silently
// change routing, or skip registration altogether.
func parseRegister(config json.RawMessage) (*registerSpec, error) {
	if len(config) == 0 {
		return nil, nil
	}
	var wrap struct {
		Register json.RawMessage `json:"register"`
	}
	if err := json.Unmarshal(config, &wrap); err != nil {
		return nil, fmt.Errorf("source config is not a JSON object: %w", err)
	}
	if len(wrap.Register) == 0 || string(wrap.Register) == "null" {
		return nil, nil
	}
	if err := requireExactRegisterKeys(wrap.Register); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(wrap.Register))
	dec.DisallowUnknownFields()
	var reg registerSpec
	if err := dec.Decode(&reg); err != nil {
		return nil, fmt.Errorf("malformed register block: %w", err)
	}
	return &reg, nil
}

// registerKeys is the exact key set the register block accepts, read off
// registerSpec's own json tags so the two cannot drift.
var registerKeys = func() map[string]bool {
	keys := map[string]bool{}
	t := reflect.TypeOf(registerSpec{})
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		keys[name] = true
	}
	return keys
}()

// requireExactRegisterKeys refuses a key that only case-folds onto a known
// one. encoding/json matches field names case-insensitively, even under
// DisallowUnknownFields, so `SendResolved` or a second `NAMESPACE` would be
// applied (the last one winning) while the chart's case-sensitive configSchema
// reports the same block invalid.
func requireExactRegisterKeys(block json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(block, &fields); err != nil {
		return fmt.Errorf("malformed register block: %w", err)
	}
	for key := range fields {
		if !registerKeys[key] {
			return fmt.Errorf("malformed register block: unknown field %q", key)
		}
	}
	return nil
}

// kubeClient is a minimal in-cluster Kubernetes REST client.
type kubeClient struct {
	base  string
	token string
	http  *http.Client
}

const saMount = "/var/run/secrets/kubernetes.io/serviceaccount"

// registrationGrant is the permission self-registration needs, in the
// target namespace — the same rules the chart's registration Role carries.
const registrationGrant = "get/list/create/update/patch on vmalertmanagerconfigs.operator.victoriametrics.com"

// errForbidden marks an API refusal (401/403): the remedy is a grant bound to
// the adapter's account, which only the caller can name.
var errForbidden = errors.New("forbidden")

// accountRemediation tells the operator where the adapter's identity comes
// from, naming only fields the current SignalAdapter CRD carries: the token is
// mounted only for the account spec.serviceAccountName references, and the
// prometheus chart renders that account (agentops-signal-<adapter>) beside its
// registration Role when alertmanager.registration.enabled is set.
func accountRemediation(adapterName string) string {
	return fmt.Sprintf(
		"set SignalAdapter %s spec.serviceAccountName to an account bound to that grant — "+
			"the prometheus chart renders agentops-signal-%s with it under alertmanager.registration.enabled",
		adapterName, adapterName)
}

// newInClusterClient builds a client from the mounted SA. A nil client (with
// the returned reason) means the token is not available — registration then
// degrades to instructions.
func newInClusterClient(adapterName string) (*kubeClient, string) {
	tok, err := os.ReadFile(saMount + "/token")
	if err != nil {
		return nil, "ServiceAccount token not mounted (registration needs " + registrationGrant + "; " +
			accountRemediation(adapterName) + ")"
	}
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" {
		return nil, "not running in a cluster (KUBERNETES_SERVICE_HOST unset)"
	}
	pool := x509.NewCertPool()
	if ca, err := os.ReadFile(saMount + "/ca.crt"); err == nil {
		pool.AppendCertsFromPEM(ca)
	}
	return &kubeClient{
		base:  "https://" + host + ":" + port,
		token: strings.TrimSpace(string(tok)),
		http: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		},
	}, ""
}

// do performs one API call; 4xx/5xx come back as errors carrying the status.
func (k *kubeClient) do(ctx context.Context, method, path string, in any) (int, []byte, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, k.base+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+k.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := k.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, b, nil
}

const vmacPath = "/apis/operator.victoriametrics.com/v1beta1/namespaces/%s/vmalertmanagerconfigs"

// registrationName is the deterministic VMAlertmanagerConfig identity for a
// source.
func registrationName(source string) string { return "agentops-" + source }

// desiredVMAC renders the registration object.
func desiredVMAC(source, namespace, webhookURL string, reg *registerSpec) map[string]any {
	name := registrationName(source)
	route := map[string]any{"receiver": name, "continue": true}
	if len(reg.Matchers) > 0 {
		route["matchers"] = reg.Matchers
	}
	for key, val := range map[string]string{
		"group_wait":      reg.GroupWait,
		"group_interval":  reg.GroupInterval,
		"repeat_interval": reg.RepeatInterval,
	} {
		if val != "" {
			route[key] = val
		}
	}
	webhook := map[string]any{"url": webhookURL}
	if reg.SendResolved != nil {
		webhook["send_resolved"] = *reg.SendResolved
	}
	if reg.MaxAlerts > 0 {
		webhook["max_alerts"] = reg.MaxAlerts
	}
	return map[string]any{
		"apiVersion": "operator.victoriametrics.com/v1beta1",
		"kind":       "VMAlertmanagerConfig",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "agentops-signal-alertmanager",
				"agentops.dev/source":          source,
			},
		},
		"spec": map[string]any{
			"route": route,
			"receivers": []any{map[string]any{
				"name":            name,
				"webhook_configs": []any{webhook},
			}},
		},
	}
}

// ensureRegistration creates or repairs the VMAlertmanagerConfig for a
// source. Returns the object reference on success; errors are pre-shaped for
// the instruction path.
func (k *kubeClient) ensureRegistration(ctx context.Context, source, ns, webhookURL string, reg *registerSpec) (string, error) {
	name := registrationName(source)
	ref := ns + "/" + name
	desired := desiredVMAC(source, ns, webhookURL, reg)

	collection := fmt.Sprintf(vmacPath, ns)
	code, body, err := k.do(ctx, "GET", collection+"/"+name, nil)
	if err != nil {
		return "", fmt.Errorf("API unreachable: %v", err)
	}
	switch code {
	case 200:
		var current map[string]any
		if err := json.Unmarshal(body, &current); err != nil {
			return "", fmt.Errorf("unreadable current object %s: %v", ref, err)
		}
		if specEqual(current["spec"], desired["spec"]) {
			return ref, nil
		}
		// drift: repair, preserving resourceVersion for a clean update
		if meta, ok := current["metadata"].(map[string]any); ok {
			desired["metadata"].(map[string]any)["resourceVersion"] = meta["resourceVersion"]
		}
		code, body, err = k.do(ctx, "PUT", collection+"/"+name, desired)
		if err != nil || code >= 400 {
			return "", apiFailure("update", ref, code, body, err)
		}
		return ref, nil
	case 404:
		// object (or CRD/namespace) absent — try to create; a 404 on create
		// means the CRD itself is missing
		code, body, err = k.do(ctx, "POST", collection, desired)
		if err != nil || code >= 400 {
			return "", apiFailure("create", ref, code, body, err)
		}
		return ref, nil
	default:
		return "", apiFailure("read", ref, code, body, nil)
	}
}

// specEqual compares a decoded spec with a freshly built one. The two sides
// differ in Go types ([]string vs []any) though they mean the same JSON, so
// both are normalized through the encoder (which sorts map keys) first.
func specEqual(current, desired any) bool {
	a, err1 := json.Marshal(current)
	b, err2 := json.Marshal(desired)
	return err1 == nil && err2 == nil && bytes.Equal(a, b)
}

func apiFailure(verb, ref string, code int, body []byte, err error) error {
	if err != nil {
		return fmt.Errorf("%s %s: %v", verb, ref, err)
	}
	switch code {
	case 401, 403:
		return fmt.Errorf("%s %s %w (grant %s in %s)", verb, ref, errForbidden, registrationGrant, strings.SplitN(ref, "/", 2)[0])
	case 404:
		return fmt.Errorf("%s %s: VMAlertmanagerConfig CRD or namespace not found (is the VictoriaMetrics operator installed?)", verb, ref)
	default:
		return fmt.Errorf("%s %s: %d %s", verb, ref, code, bytes.TrimSpace(body))
	}
}

// webhookURL derives this adapter's own endpoint for a source from its
// reconciler-injected identity (Service name = agentops-signal-<adapter>,
// namespace from the downward API, port from LISTEN_ADDR).
func webhookURL(adapterName, podNamespace, listenAddr, source string) string {
	port := strings.TrimPrefix(listenAddr, ":")
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("http://agentops-signal-%s.%s.svc:%s/webhook/%s", adapterName, podNamespace, port, source)
}
