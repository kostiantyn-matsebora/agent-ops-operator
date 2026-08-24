package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// SignalAdapterSpec declares a signal-type IMPLEMENTATION — nothing more.
// The CR's NAME is the routing key: SignalSources whose spec.adapter equals it
// are served by this adapter (one adapter per implementation, by
// construction). No configuration lives here: per-source settings are on the
// served SignalSources (config, credentialsSecretRef — projected into the
// pod by the reconciler, kubelet-resolved, never read through the API).
// +kubebuilder:validation:XValidation:rule="has(self.image) != has(self.servedBy)",message="set exactly one of image (this adapter runs its own workload) or servedBy (another adapter's pod serves this identity)"
type SignalAdapterSpec struct {
	// Image implementing the signal adapter contract. Required UNLESS servedBy
	// names the workload that already serves this identity.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Image string `json:"image,omitempty"`
	// ServedBy declares this adapter EXTERNALLY SERVED: another adapter's pod
	// already holds the process, and this CR exists only to give it a signal
	// identity. When set the reconciler creates NO Deployment, Service or
	// ServiceAccount and reports Ready=True with reason ServedBy; the named
	// ChannelAdapter's reconciler injects SIGNAL_ADAPTER_TOKEN into its pod.
	//
	// Why this exists: a chat transport is inherently a SURFACE and an
	// ORIGINATOR — it carries conversations on threads and starts them from the
	// general surface. Without this, declaring both identities produces two
	// Deployments, one of which is an idle pod existing only to make a source
	// Served. This repo has paid for that shape once already (gateway-telegram
	// was an adapter with a signal-free SignalSource purely to carry a
	// credential, which then sat at Wired=False). The difference here is the one
	// that matters: an externally-served source originates real conversations
	// for a Pipeline that claims it.
	// +optional
	ServedBy *AdapterRef `json:"servedBy,omitempty"`
	// Port the image's own HTTP surface listens on (webhook-receiving
	// implementations). When set, the reconciler owns a Service
	// agentops-signal-<name> targeting it and injects LISTEN_ADDR — enabling
	// the adapter is a complete appliance. Unset = no inbound surface (e.g.
	// cron).
	// +optional
	Port *int32 `json:"port,omitempty"`
	// ServiceAccountName is the identity this adapter's workload runs as, and
	// its token IS mounted — naming an account whose token is never mounted
	// would grant nothing, since the pod would never present that identity.
	//
	// A REFERENCE, never a creation. No reconciler creates this account, none
	// validates that it exists, and none binds anything to it: a SignalAdapter
	// is an ordinary namespaced object, so an operator that could grant one
	// would make CR-edit rights a privilege escalation. The chart that grants an
	// adapter its permissions renders the account beside that grant.
	//
	// EMPTY MEANS THE RELEASE'S FLOOR ACCOUNT — created always by the chart,
	// bound to nothing, refused as a binding target. Not the namespace default,
	// which carries whatever the cluster gave it; and not "no account", because
	// a pod holding no token cannot be told apart from one whose grant was
	// forgotten.
	//
	// It must never be a runtime identity. What an agent may do is model
	// output's reach; what an adapter may do is this project's own code's reach,
	// and the two are opposite grants.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Singleton runs the workload as replicas 1 + strategy Recreate so no
	// rollout ever runs two instances side by side (pollers and schedulers
	// must not double-fire).
	// +kubebuilder:default=true
	// +optional
	Singleton *bool `json:"singleton,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// ConfigSchema is a JSON Schema (draft 2020-12) describing spec.config on
	// the Channels/SignalSources this adapter serves. OPTIONAL — declaring
	// nothing behaves exactly as before. This is interface metadata, not
	// configuration: it holds no config values, connectivity, or credentials,
	// so the CR stays pure implementation. Because it lives on the spec it is
	// readable by any cluster client (kubectl, docs tooling) the moment the CR
	// is applied — no registration step, and the adapter binary plays no part.
	// Authoring rule: bump the schema in the same diff as `image`.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ConfigSchema *runtime.RawExtension `json:"configSchema,omitempty"`
	// CredentialKeys documents the Secret keys the implementation expects in a
	// served CR's credentialsSecretRef. Documentation ONLY — the manager reads
	// no Secrets, so it can never verify these.
	// +optional
	CredentialKeys []CredentialKeyDoc `json:"credentialKeys,omitempty"`
}

// SignalAdapterStatus reports workload and serving state.
type SignalAdapterStatus struct {
	// Conditions: Deployed (workload rendered), Ready (workload available).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ServedSources counts SignalSources naming this adapter in spec.adapter.
	// +optional
	ServedSources int32 `json:"servedSources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Sources",type=integer,JSONPath=`.status.servedSources`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SignalAdapter plugs a signal implementation into the operator: apply one
// naming an image and every SignalSource whose spec.adapter equals THIS CR's
// NAME is served by it — no operator or chart change. The reconciler owns the
// adapter Deployment (zero-RBAC ServiceAccount, no SA token automount), the
// Service when port is declared, and injects the manager URL, a per-adapter
// derived auth token, and each served source's projected credentials.
type SignalAdapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SignalAdapterSpec   `json:"spec,omitempty"`
	Status SignalAdapterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SignalAdapterList contains a list of SignalAdapter.
type SignalAdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SignalAdapter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SignalAdapter{}, &SignalAdapterList{})
}
