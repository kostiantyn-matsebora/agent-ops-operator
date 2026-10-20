package controlplane

import (
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// ProbeHandler serves the ControlPlaneProbe round trip: create-or-update by
// name, read by name, 404 on an unknown name. ControlPlaneProbe is
// disposable scaffolding (see its own type doc comment in api/v1alpha1) —
// this handler exists to prove the surface works end to end, and is
// deleted or renamed, never grown in place, once a real first resident of
// /control-plane/* is proposed.
type ProbeHandler struct {
	// Writer performs Create/Update. The manager's own cached client is fine
	// here — writes always reach the API server regardless of cache state.
	Writer client.Writer
	// Reader answers every Get, strongly consistent. A handler's own write
	// must never be missed by a cache that has not caught up — the same
	// reason httpapi.Server keeps a separate APIReader (Reader) beside its
	// cached Client.
	Reader client.Reader
	// Namespace is the manager's own operating namespace — the same one
	// every other kind it owns is read and written in.
	Namespace string
}

// probeBody is the wire shape for both the PUT request and the GET
// response.
type probeBody struct {
	// Name is OPTIONAL on PUT — the route's {name} is authoritative — but
	// when present it MUST agree with it. A caller whose body claims a
	// different name than the route gets refused with nothing stored,
	// rather than either one being silently preferred over the other.
	Name string `json:"name,omitempty"`
	// Key is required: the CRD's own validation says so, and this handler
	// enforces it explicitly rather than depend on the cluster having that
	// validation installed (CRD YAML, not every test fixture, enforces it).
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// Put creates or updates the named ControlPlaneProbe.
func (h *ProbeHandler) Put(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing name"})
		return
	}
	var in probeBody
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if in.Name != "" && in.Name != name {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("body name %q does not match route name %q", in.Name, name),
		})
		return
	}
	if in.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing key"})
		return
	}
	ctx := r.Context()
	var existing agentopsv1alpha1.ControlPlaneProbe
	err := h.Reader.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: name}, &existing)
	switch {
	case err == nil:
		existing.Spec.Key, existing.Spec.Value = in.Key, in.Value
		if err := h.Writer.Update(ctx, &existing); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	case apierrors.IsNotFound(err):
		probe := &agentopsv1alpha1.ControlPlaneProbe{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.Namespace},
			Spec:       agentopsv1alpha1.ControlPlaneProbeSpec{Key: in.Key, Value: in.Value},
		}
		if err := h.Writer.Create(ctx, probe); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Get reads the named ControlPlaneProbe, 404 on an unknown name.
func (h *ProbeHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing name"})
		return
	}
	var probe agentopsv1alpha1.ControlPlaneProbe
	if err := h.Reader.Get(r.Context(), types.NamespacedName{Namespace: h.Namespace, Name: name}, &probe); err != nil {
		if apierrors.IsNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, probeBody{Name: probe.Name, Key: probe.Spec.Key, Value: probe.Spec.Value})
}
