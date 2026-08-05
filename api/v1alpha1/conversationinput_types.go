package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConversationInputSpec carries a large out-of-line work-unit payload.
type ConversationInputSpec struct {
	ConversationRef ObjectRef `json:"conversationRef"`
	Type            InputType `json:"type"`
	Payload         string    `json:"payload"`
}

// ConversationInputStatus tracks consumption.
type ConversationInputStatus struct {
	// +optional
	Consumed bool `json:"consumed,omitempty"`
	// +optional
	ConsumedAt *metav1.Time `json:"consumedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=convin
// +kubebuilder:printcolumn:name="Conversation",type=string,JSONPath=`.spec.conversationRef.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Consumed",type=boolean,JSONPath=`.status.consumed`

// ConversationInput is an out-of-line payload for a Conversation input item.
type ConversationInput struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConversationInputSpec   `json:"spec,omitempty"`
	Status ConversationInputStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConversationInputList contains a list of ConversationInput.
type ConversationInputList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConversationInput `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConversationInput{}, &ConversationInputList{})
}
