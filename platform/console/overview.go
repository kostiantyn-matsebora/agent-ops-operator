package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

// The installation at a glance, and what is wrong with it.
//
// The page's real job is the LAST section: every non-True condition across every
// kind, plus pod-level failure. The answer to "is my install healthy" should not
// require reading four other pages, and it must not require reading kubectl —
// which is also why nothing here asserts health of its own. Every problem row
// quotes a condition a reconciler wrote or a state the kubelet reported.

// podSpecView is the console's read of a pod/deployment spec: images, and
// nothing else. Image DIGESTS come from pod status, not spec, because the spec
// carries whatever tag was written and the status carries what actually ran.
type podSpecView struct {
	Containers []struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	} `json:"containers"`
	NodeName string `json:"nodeName,omitempty"`
}

type podStatusView struct {
	Phase             string `json:"phase,omitempty"`
	Reason            string `json:"reason,omitempty"`
	Message           string `json:"message,omitempty"`
	ContainerStatuses []struct {
		Name         string `json:"name"`
		Ready        bool   `json:"ready"`
		RestartCount int    `json:"restartCount"`
		ImageID      string `json:"imageID,omitempty"`
		State        map[string]struct {
			Reason  string `json:"reason,omitempty"`
			Message string `json:"message,omitempty"`
		} `json:"state,omitempty"`
	} `json:"containerStatuses,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

type deploySpecView struct {
	Replicas *int32 `json:"replicas,omitempty"`
	Template struct {
		Spec podSpecView `json:"spec"`
	} `json:"template"`
}

type deployStatusView struct {
	Replicas          int32 `json:"replicas,omitempty"`
	ReadyReplicas     int32 `json:"readyReplicas,omitempty"`
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
}

// WorkloadInfo is one Deployment as the overview renders it.
type WorkloadInfo struct {
	Name     string `json:"name"`
	Image    string `json:"image,omitempty"`
	Digest   string `json:"digest,omitempty"`
	Desired  int32  `json:"desired"`
	Ready    int32  `json:"ready"`
	Restarts int    `json:"restarts"`
	// Problem is the pod-level reason when one exists — CrashLoopBackOff,
	// ImagePullBackOff, Unschedulable. This is the fact that exists in no CR and
	// is the reason the console takes a pod read grant at all.
	Problem string `json:"problem,omitempty"`
}

// Problem is one thing wrong right now.
type Problem struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	Since   string `json:"since,omitempty"`
	// Source distinguishes a condition a RECONCILER wrote from a finding the
	// CONSOLE derived by cross-referencing. The two carry different authority
	// and must never be presented as the same thing.
	Source string `json:"source"`
}

// Problem sources.
const (
	SourceReported = "reported" // a condition on the object
	SourceDerived  = "derived"  // the console's own cross-reference
	SourcePod      = "pod"      // kubelet-reported workload state
)

// Overview is GET /api/overview.
type Overview struct {
	Namespace string `json:"namespace"`
	// Manager is what the manager reports about itself — version, leader, and
	// its capacity ceiling. Absent (with Error set) when /status is unreachable,
	// which is itself worth showing.
	Manager      *ManagerStatus `json:"manager,omitempty"`
	ManagerError string         `json:"managerError,omitempty"`
	Stream       StreamHealth   `json:"stream"`

	Workloads []WorkloadInfo `json:"workloads"`
	Runtimes  []RuntimeInfo  `json:"runtimes"`
	Adapters  []AdapterInfo  `json:"adapters"`

	Counts   map[string]int  `json:"counts"`
	Synced   map[string]bool `json:"synced"`
	Problems []Problem       `json:"problems"`
}

// RuntimeInfo is one AgentRuntime's image — the version an agent actually runs.
type RuntimeInfo struct {
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
}

// AdapterInfo is one adapter CR, either kind.
type AdapterInfo struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Image is empty for an EXTERNALLY SERVED signal adapter — it owns no
	// workload, which is a configuration rather than a missing field.
	Image    string `json:"image,omitempty"`
	ServedBy string `json:"servedBy,omitempty"`
	Health   Health `json:"health"`
	Reason   string `json:"reason,omitempty"`
	Serves   int    `json:"serves"`
}

func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	out := Overview{
		Namespace: a.namespace,
		Stream:    a.activity.StreamHealth(),
		Counts:    map[string]int{},
		Synced:    map[string]bool{},
		Workloads: []WorkloadInfo{},
		Runtimes:  []RuntimeInfo{},
		Adapters:  []AdapterInfo{},
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if st, err := a.mgr.Status(ctx); err == nil {
		out.Manager = st
	} else {
		out.ManagerError = err.Error()
	}

	for _, k := range append(append([]string{}, Kinds...), InstallKinds...) {
		out.Counts[k] = len(a.cache.List(k))
		out.Synced[k] = a.cache.Synced(k)
	}
	out.Workloads = a.workloads()
	for _, rt := range a.cache.List("agentruntimes") {
		var spec struct {
			Image string `json:"image,omitempty"`
		}
		_ = json.Unmarshal(rt.Spec, &spec)
		out.Runtimes = append(out.Runtimes, RuntimeInfo{Name: rt.Metadata.Name, Image: spec.Image})
	}
	out.Adapters = a.adapters()
	out.Problems = a.problems()
	writeJSON(w, http.StatusOK, out)
}

// workloads joins Deployments with their pods so a rollout that is "1 desired,
// 0 ready" carries the kubelet's reason rather than just a number.
func (a *API) workloads() []WorkloadInfo {
	podsByPrefix := map[string][]*Object{}
	for _, p := range a.cache.List("pods") {
		podsByPrefix[p.Metadata.Name] = append(podsByPrefix[p.Metadata.Name], p)
	}
	out := []WorkloadInfo{}
	for _, d := range a.cache.List("deployments") {
		spec := decodeSpec[deploySpecView](d.Spec)
		status := decodeSpec[deployStatusView](d.Status)
		info := WorkloadInfo{Name: d.Metadata.Name, Ready: status.ReadyReplicas}
		if spec.Replicas != nil {
			info.Desired = *spec.Replicas
		}
		if len(spec.Template.Spec.Containers) > 0 {
			info.Image = spec.Template.Spec.Containers[0].Image
		}
		// pods are named <deployment>-<replicaset hash>-<suffix>; prefix match is
		// enough here because these are all chart-owned names in one namespace
		for name, pods := range podsByPrefix {
			if !strings.HasPrefix(name, d.Metadata.Name+"-") {
				continue
			}
			for _, p := range pods {
				ps := decodeSpec[podStatusView](p.Status)
				for _, cs := range ps.ContainerStatuses {
					info.Restarts += cs.RestartCount
					if info.Digest == "" && cs.ImageID != "" {
						info.Digest = cs.ImageID
					}
				}
				if reason := podProblem(ps); reason != "" && info.Problem == "" {
					info.Problem = reason
				}
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// podProblem names why a pod is not serving, or "" when it is fine. Waiting and
// terminated states both matter: ImagePullBackOff and CrashLoopBackOff are the
// two an operator most needs, and they live in different halves of the state.
func podProblem(ps podStatusView) string {
	if ps.Phase == "Failed" || ps.Phase == "Unknown" {
		return strings.TrimSpace(ps.Phase + " " + ps.Reason + " " + ps.Message)
	}
	for _, cs := range ps.ContainerStatuses {
		for state, detail := range cs.State {
			if state == "running" {
				continue
			}
			if detail.Reason != "" && detail.Reason != "Completed" {
				return cs.Name + ": " + detail.Reason
			}
		}
	}
	if ps.Phase == "Pending" {
		for _, c := range ps.Conditions {
			if c.Type == "PodScheduled" && c.Status == "False" {
				return "unschedulable: " + c.Reason
			}
		}
	}
	return ""
}

func (a *API) adapters() []AdapterInfo {
	out := []AdapterInfo{}
	for _, kind := range []string{"channeladapters", "signaladapters"} {
		served, servedKind := "channels", "channels"
		if kind == "signaladapters" {
			served, servedKind = "signalsources", "signalsources"
		}
		for _, obj := range a.cache.List(kind) {
			var spec struct {
				Image    string `json:"image,omitempty"`
				ServedBy *struct {
					Kind string `json:"kind"`
					Name string `json:"name"`
				} `json:"servedBy,omitempty"`
			}
			_ = json.Unmarshal(obj.Spec, &spec)
			h, reason, _ := health(obj)
			info := AdapterInfo{Kind: kind, Name: obj.Metadata.Name, Image: spec.Image, Health: h, Reason: reason}
			if spec.ServedBy != nil {
				info.ServedBy = spec.ServedBy.Kind + "/" + spec.ServedBy.Name
			}
			for _, s := range a.cache.List(served) {
				if decodeSpec[servedSpec](s.Spec).Adapter == obj.Metadata.Name {
					info.Serves++
				}
			}
			_ = servedKind
			out = append(out, info)
		}
	}
	return out
}

// problems collects every non-True condition across every kind, plus pod
// failures and the console's own cross-object findings — each labelled with
// where it came from.
//
// HIDDEN CLASSES STILL COUNT. This list is built from the cache, never from
// whatever the topology view is currently displaying, so a display filter can
// never conceal a broken component from the rollup.
func (a *API) problems() []Problem {
	var out []Problem
	for _, kind := range Kinds {
		for _, obj := range a.cache.List(kind) {
			for _, c := range obj.Conditions() {
				if c.Status == "True" {
					continue
				}
				out = append(out, Problem{
					Kind: kind, Name: obj.Metadata.Name, Type: c.Type,
					Reason: c.Reason, Message: c.Message, Since: c.LastTransitionTime,
					Source: SourceReported,
				})
			}
		}
	}
	for _, p := range a.cache.List("pods") {
		if reason := podProblem(decodeSpec[podStatusView](p.Status)); reason != "" {
			out = append(out, Problem{
				Kind: "pods", Name: p.Metadata.Name, Type: "PodHealth",
				Reason: reason, Source: SourcePod,
			})
		}
	}
	for _, f := range a.findings() {
		out = append(out, Problem{
			Kind: f.Kind, Name: f.Name, Type: f.Check,
			Reason: f.Reason, Message: f.Message, Source: SourceDerived,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Since != out[j].Since {
			return out[i].Since > out[j].Since // newest first
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}
