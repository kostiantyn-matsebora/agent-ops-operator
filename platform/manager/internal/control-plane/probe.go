package controlplane

import (
	"context"
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
	if err := h.createOrUpdate(r.Context(), name, in.Key, in.Value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// createOrUpdate re-reads and retries its own read-modify-write cycle on a
// conflict, the same shape httpapi.Server's setPhaseUnlessClosed uses for an
// optimistically-locked update. Without this, two concurrent PUTs to a name
// that does not exist yet both see NotFound and both Create — the loser gets
// IsAlreadyExists — and two concurrent PUTs to an existing name both Get the
// same resourceVersion and the loser's Update gets IsConflict. Either would
// otherwise surface as a flat 500 for a request that, semantically, should
// have converged to the same end state.
func (h *ProbeHandler) createOrUpdate(ctx context.Context, name, key, value string) error {
	for range 5 {
		var existing agentopsv1alpha1.ControlPlaneProbe
		err := h.Reader.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: name}, &existing)
		switch {
		case err == nil:
			existing.Spec.Key, existing.Spec.Value = key, value
			if err := h.Writer.Update(ctx, &existing); err != nil {
				if apierrors.IsConflict(err) {
					continue
				}
				return err
			}
			return nil
		case apierrors.IsNotFound(err):
			probe := &agentopsv1alpha1.ControlPlaneProbe{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.Namespace},
				Spec:       agentopsv1alpha1.ControlPlaneProbeSpec{Key: key, Value: value},
			}
			if err := h.Writer.Create(ctx, probe); err != nil {
				if apierrors.IsAlreadyExists(err) {
					continue
				}
				return err
			}
			return nil
		default:
			return err
		}
	}
	return fmt.Errorf("conflict writing probe %q", name)
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
			writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("probe %q not found", name)})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, probeBody{Name: probe.Name, Key: probe.Spec.Key, Value: probe.Spec.Value})
}
