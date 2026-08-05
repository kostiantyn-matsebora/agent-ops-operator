package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MCPConfigSpec is a reusable, shareable set of MCP server definitions.
type MCPConfigSpec struct {
	Servers map[string]MCPServer `json:"servers"`
}

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
