package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TelegramChannel configures a Telegram supergroup (Topics enabled) as chat surface.
type TelegramChannel struct {
	// BotTokenSecretRef selects the bot token.
	BotTokenSecretRef corev1.SecretKeySelector `json:"botTokenSecretRef"`
	// ChatID of the supergroup (-100…).
	ChatID string `json:"chatId"`
	// FeedThreadID: topic for raw notification passthrough (optional).
	// +optional
	FeedThreadID *int64 `json:"feedThreadId,omitempty"`
	// Approvers: Telegram user ids allowed to approve actions. Empty = anyone
	// in the (private) group.
	// +optional
	Approvers []int64 `json:"approvers,omitempty"`
	// PollingEnabled starts the getUpdates loop for this channel. Keep false
	// while another system polls the same bot token (getUpdates conflicts).
	// +kubebuilder:default=false
	// +optional
	PollingEnabled bool `json:"pollingEnabled,omitempty"`
}

// ChannelSpec configures one chat surface.
type ChannelSpec struct {
	// +optional
	Telegram *TelegramChannel `json:"telegram,omitempty"`
	// DefaultProfileRef handles bare messages (no /profile prefix).
	// +optional
	DefaultProfileRef *ObjectRef `json:"defaultProfileRef,omitempty"`
}

// ChannelStatus reports connectivity.
type ChannelStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	Polling bool `json:"polling,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Polling",type=boolean,JSONPath=`.status.polling`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Channel is a chat surface (v1: Telegram supergroup with Topics).
type Channel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChannelSpec   `json:"spec,omitempty"`
	Status ChannelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChannelList contains a list of Channel.
type ChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Channel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Channel{}, &ChannelList{})
}
