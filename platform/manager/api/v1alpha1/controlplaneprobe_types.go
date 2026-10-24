package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ControlPlaneProbeSpec is a trivial key/value pair with no real-world
// meaning, carried only to exercise create/read through /control-plane/*.
type ControlPlaneProbeSpec struct {
	// Key is an opaque, operator-chosen label for this probe object. It
	// carries no meaning the manager interprets.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`

	// Value is an opaque payload paired with Key. It carries no meaning the
	// manager interprets.
	Value string `json:"value,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=cpprobe
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ControlPlaneProbe is disposable scaffolding, to be replaced, not extended,
// once a real first resident of /control-plane/* is proposed. It exists
// solely to prove the token-auth → handler → object-CRUD round trip that
// surface exists for, and carries no business meaning of its own. It has no
// status: there is nothing to resolve.
type ControlPlaneProbe struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ControlPlaneProbeSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ControlPlaneProbeList contains a list of ControlPlaneProbe.
type ControlPlaneProbeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ControlPlaneProbe `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ControlPlaneProbe{}, &ControlPlaneProbeList{})
}
