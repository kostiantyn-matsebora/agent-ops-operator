package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MCPConfigSpec is a reusable, shareable set of MCP server definitions, or —
// as an escape hatch — a reference to a hand-written mcp.json.
type MCPConfigSpec struct {
	// Servers defined inline. Mutually exclusive with the raw forms below.
	// +optional
	Servers map[string]MCPServer `json:"servers,omitempty"`
	// ConfigMapRef / SecretRef mount a complete hand-written mcp.json (key
	// mcp.json) instead of compiling one. Such a config is EXCLUSIVE: a
	// document the operator maintains by hand is opaque to us, so binding it
	// alongside any other config is an error rather than a partial result.
	// +optional
	ConfigMapRef *ObjectRef `json:"configMapRef,omitempty"`
	// +optional
	SecretRef *ObjectRef `json:"secretRef,omitempty"`
}

// IsRaw reports whether this config mounts a hand-written mcp.json rather than
// compiling one from Servers.
func (s *MCPConfigSpec) IsRaw() bool { return s.ConfigMapRef != nil || s.SecretRef != nil }

// MCPConfigStatus reports validation state.
type MCPConfigStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mcpc
// +kubebuilder:printcolumn:name="Servers",type=integer,JSONPath=`.status.serverCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MCPConfig is a named MCP server set referenced by AgentProfiles.
type MCPConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPConfigSpec   `json:"spec,omitempty"`
	Status MCPConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MCPConfigList contains a list of MCPConfig.
type MCPConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPConfig{}, &MCPConfigList{})
}
