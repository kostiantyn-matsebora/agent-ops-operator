// Package v1alpha1 contains API Schema definitions for the agentops v1alpha1 API group.
//
// Agent Ops Operator: agents you can address — signals (alerts, cron, k8s events)
// and direct tasks become Conversations pinned to chat topics, executed by
// per-conversation runtime pods running an agent defined by an AgentProfile
// (repository + agent + MCP + credentials).
//
// +kubebuilder:object:generate=true
// +groupName=agentops.dev
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects.
	// Provisional group — final domain decided at public extraction (pre-1.0
	// rename is cheap; check naming collisions, e.g. agentops.ai company).
	GroupVersion = schema.GroupVersion{Group: "agentops.dev", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
