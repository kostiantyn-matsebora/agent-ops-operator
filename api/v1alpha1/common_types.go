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

// MCPSpec is the tri-form MCP configuration:
// inline servers, refs to MCPConfig CRs, or a raw ConfigMap/Secret holding mcp.json.
// Forms may be combined; merge order: configMap/secret raw < configRefs (in order) < inline.
type MCPSpec struct {
	// Inline server definitions.
	// +optional
	Servers map[string]MCPServer `json:"servers,omitempty"`
	// References to MCPConfig objects, merged in order.
	// +optional
	ConfigRefs []ObjectRef `json:"configRefs,omitempty"`
	// Raw complete mcp.json in a ConfigMap (key mcp.json).
	// +optional
	ConfigMapRef *ObjectRef `json:"configMapRef,omitempty"`
	// Raw complete mcp.json in a Secret (key mcp.json).
	// +optional
	SecretRef *ObjectRef `json:"secretRef,omitempty"`
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
