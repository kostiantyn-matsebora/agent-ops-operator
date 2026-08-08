package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// ObjectRef references another object by name (same namespace).
type ObjectRef struct {
	// Name of the referenced object.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ToolingBinding binds capabilities to a wiring: an ordered set of refs.
// Content lives entirely in the referenced CRs (MCPToolset / MCPConfig) — the
// binding carries refs only.
//
// There is deliberately no mode. Capabilities live ONLY on the Pipeline, so
// there is no profile-side tooling to merge with or overwrite; a mode field
// would be a control whose two values did the same thing.
type ToolingBinding struct {
	// Refs are applied in order: tool lists concatenate with dedup, MCP server
	// keys are overlaid with the later ref winning a collision.
	// +kubebuilder:validation:MinItems=1
	Refs []ObjectRef `json:"refs"`
}

// NamedValue is a name/value pair where the value may come from a Secret or
// ConfigMap key (the env valueFrom idiom). Used for MCP server headers etc.
type NamedValue struct {
	Name string `json:"name"`
	// +optional
	Value string `json:"value,omitempty"`
	// +optional
	ValueFrom *corev1.EnvVarSource `json:"valueFrom,omitempty"`
}

// MCPServer describes a single MCP server a worker connects to.
type MCPServer struct {
	// Type of transport: sse, http or stdio.
	// +kubebuilder:validation:Enum=sse;http;stdio
	Type string `json:"type"`
	// URL for sse/http transports.
	// +optional
	URL string `json:"url,omitempty"`
	// Command and Args for stdio transport.
	// +optional
	Command string `json:"command,omitempty"`
	// +optional
	Args []string `json:"args,omitempty"`
	// Headers to send (sse/http). Values may reference Secrets — the manager
	// compiles them to env-var placeholders resolved in the runtime pod.
	// +optional
	Headers []NamedValue `json:"headers,omitempty"`
	// Env for stdio servers.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// CredentialKeyDoc documents one Secret key an adapter implementation expects
// in a served CR's credentialsSecretRef. It is DOCUMENTATION ONLY: the manager
// reads no Secrets, so it can neither verify the key exists nor read its value
// — it only makes the expectation discoverable from the adapter CR.
type CredentialKeyDoc struct {
	// Key is the Secret key (projected as env <credentialEnvPrefix><KEY>).
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
	// Required marks a key the implementation cannot work without.
	// +optional
	Required bool `json:"required,omitempty"`
	// +optional
	Description string `json:"description,omitempty"`
}
