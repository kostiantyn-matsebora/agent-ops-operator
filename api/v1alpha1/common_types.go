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

// ToolingBinding binds MCP configs to a wiring: an ordered set of refs.
// Content lives entirely in the referenced CRs (MCPConfig) — the binding
// carries refs only.
//
// There is deliberately no mode here. MCP SERVERS reach a run only through the
// compiled mcp.json, and an agent definition has no field that declares one —
// so there is nothing on the other side for a mode to compose against, and its
// two values would do the same thing. (Tools are different: see
// ToolsetBinding.)
type ToolingBinding struct {
	// Refs are applied in order: MCP server keys are overlaid with the later
	// ref winning a collision.
	// +kubebuilder:validation:MinItems=1
	Refs []ObjectRef `json:"refs"`
}

// ToolsetBinding binds MCPToolset CRs to a wiring, plus the MODE that says how
// their tools compose with the ones the AGENT'S OWN DEFINITION declares — the
// `tools:` frontmatter of .claude/agents/<agent>.md in the profile's
// repository. The counterpart is the agent definition, never the profile: the
// profile carries no capabilities at all, and mistaking it for the counterpart
// is what deleted this field once already.
//
// The composition happens in the RUNTIME, which is the only component with the
// repository checked out. What the manager computes from Refs is the wiring's
// CONTRIBUTION, not the final allowlist.
type ToolsetBinding struct {
	// Mode composes this binding's tools with the agent definition's:
	// merge unions them (the agent keeps what it declared, the wiring adds),
	// overwrite passes the wiring's alone (the agent's declaration does not
	// apply to this route). Built-ins included — name them in the toolset.
	// +kubebuilder:validation:Enum=merge;overwrite
	// +kubebuilder:default=merge
	// +optional
	Mode string `json:"mode,omitempty"`
	// Refs are applied in order: tool lists concatenate with dedup, the first
	// occurrence keeping its position.
	// +kubebuilder:validation:MinItems=1
	Refs []ObjectRef `json:"refs"`
}

// Tooling composition modes for ToolsetBinding.Mode.
const (
	// ToolsModeMerge unions the wiring's tools with the agent definition's.
	ToolsModeMerge = "merge"
	// ToolsModeOverwrite passes the wiring's tools alone.
	ToolsModeOverwrite = "overwrite"
)

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
