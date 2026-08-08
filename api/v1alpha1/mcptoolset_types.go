package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MCPToolsetSpec is a named, reusable list of tool patterns. It carries NO
// server definitions — those belong exclusively to MCPConfig CRs.
type MCPToolsetSpec struct {
	// Tools this toolset grants: MCP namespaces ("mcp__victorialogs__*") or
	// built-in tool names ("Bash"). Any allowlist entry the runtime accepts is
	// legal; the patterns are opaque to the manager, which passes them through
	// exactly like the profile's allowedTools.
	// +kubebuilder:validation:MinItems=1
	Tools []string `json:"tools"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=mcpts
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MCPToolset is a named tool allowlist referenced by Pipelines. It has no
// status: there is nothing to resolve — the patterns are opaque strings handed
// to the runtime.
type MCPToolset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MCPToolsetSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// MCPToolsetList contains a list of MCPToolset.
type MCPToolsetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPToolset `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPToolset{}, &MCPToolsetList{})
}
