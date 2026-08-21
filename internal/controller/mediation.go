package controller

import (
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/mcpcompile"
)

// ConditionMediated reports whether every MCP server this conversation binds is
// actually behind the enforcing proxy.
//
// It exists because the dangerous answer is the SILENT one. A conversation with
// mediation enabled and one endpoint the proxy cannot parse is not "mostly
// mediated" — it has an unenforced path, and an operator who enabled the
// feature has every reason to believe otherwise. See design decision D7.
const ConditionMediated = "EgressMediated"

const (
	// ReasonAllEndpointsMediated: every bound server is enforced.
	ReasonAllEndpointsMediated = "AllEndpointsMediated"
	// ReasonEndpointNotEnforceable: a bound server's transport cannot be read,
	// so its tool calls pass unexamined.
	ReasonEndpointNotEnforceable = "EndpointNotEnforceable"
	// ReasonOpaqueConfig: a hand-written mcp.json. Its servers are unknown to
	// the operator by construction, so nothing can be claimed about them.
	ReasonOpaqueConfig = "OpaqueConfig"
)

// mediationCondition judges what mediation actually covers for one conversation.
//
// Detection is MANAGER-SIDE and needs no report from the proxy: the manager
// compiled the endpoints, so it already knows their transports. A reporting
// channel from the pod would need a credential and an endpoint, to carry a fact
// the manager holds first.
func mediationCondition(mcp mcpcompile.Result,
	med *agentopsv1alpha1.EgressMediation) *metav1.Condition {

	if med == nil {
		return nil // not mediated, nothing to claim either way
	}
	if mcp.RawConfigMap != "" || mcp.RawSecret != "" {
		return &metav1.Condition{
			Type:   ConditionMediated,
			Status: metav1.ConditionFalse,
			Reason: ReasonOpaqueConfig,
			Message: "a raw mcp.json is bound, so its servers are opaque to the manager " +
				"and cannot be confirmed mediated",
		}
	}
	var unenforceable []string
	for key, url := range mcp.Endpoints {
		if !enforceableEndpoint(url) {
			unenforceable = append(unenforceable, fmt.Sprintf("%s (%s)", key, schemeOf(url)))
		}
	}
	if len(unenforceable) > 0 {
		sort.Strings(unenforceable)
		return &metav1.Condition{
			Type:   ConditionMediated,
			Status: metav1.ConditionFalse,
			Reason: ReasonEndpointNotEnforceable,
			Message: fmt.Sprintf("tool calls to %s are forwarded WITHOUT enforcement: "+
				"the proxy reads MCP over plain HTTP and does not terminate TLS",
				strings.Join(unenforceable, ", ")),
		}
	}
	return &metav1.Condition{
		Type:    ConditionMediated,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonAllEndpointsMediated,
		Message: fmt.Sprintf("%d bound MCP server(s) enforced by the egress proxy", len(mcp.Endpoints)),
	}
}

// enforceableEndpoint reports whether the proxy can read this transport.
//
// Plain HTTP only. Enforcing an https endpoint would mean terminating TLS
// inside the pod that runs untrusted model output, which is a bigger hole than
// the one being closed — so it is REPORTED instead, deliberately.
func enforceableEndpoint(url string) bool {
	return schemeOf(url) == "http"
}

func schemeOf(url string) string {
	if i := strings.Index(url, "://"); i >= 0 {
		return strings.ToLower(url[:i])
	}
	return ""
}

// applyMediationCondition sets or clears the condition on a conversation.
func applyMediationCondition(conv *agentopsv1alpha1.Conversation, cond *metav1.Condition) bool {
	if cond == nil {
		// A conversation that is not mediated carries no claim. An install that
		// never enabled the feature must not accumulate a permanent condition
		// about it.
		if apimeta.FindStatusCondition(conv.Status.Conditions, ConditionMediated) == nil {
			return false
		}
		apimeta.RemoveStatusCondition(&conv.Status.Conditions, ConditionMediated)
		return true
	}
	return apimeta.SetStatusCondition(&conv.Status.Conditions, *cond)
}
