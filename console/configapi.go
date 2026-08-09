package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

// The configuration surface: every agentops.dev CR, listable and inspectable,
// plus the cross-object checks that currently require assembling seven kinds by
// hand.
//
// Read-only, and that is a design position rather than a missing feature.
// Pipelines are THE wiring, the wiring is GitOps-managed, and a console that
// edits them competes with helmfile. There is no write path to the Kubernetes
// API anywhere in this module.
//
// FINDINGS ARE MARKED BY SOURCE. A condition a reconciler wrote and a
// cross-reference the console derived carry different authority; presenting
// them identically would let the console appear to speak for the cluster.

// Finding is one cross-object problem the console derived itself.
type Finding struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Check   string `json:"check"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	// Ref names the object that failed to resolve, when there is one.
	Ref string `json:"ref,omitempty"`
}

// Cross-object check names.
const (
	CheckDanglingRef     = "DanglingRef"
	CheckUnclaimedSource = "UnclaimedSource"
	CheckUnservedChannel = "UnservedChannel"
	CheckProfileRuntime  = "ProfileWithoutRuntime"
	CheckConfigSchema    = "ConfigSchemaViolation"
)

// findings runs every cross-object check over the cache.
//
// These are ADVISORY. Where a reconciler already reports the same thing as a
// condition, the condition wins and appears as a `reported` problem; these exist
// for the relationships no single reconciler owns.
func (a *API) findings() []Finding {
	var out []Finding
	exists := func(kind, name string) bool { return a.cache.Get(kind, name) != nil }

	for _, p := range a.cache.List("pipelines") {
		spec := decodeSpec[pipelineSpec](p.Spec)
		check := func(kind, name string) {
			if name == "" || exists(kind, name) {
				return
			}
			out = append(out, Finding{
				Kind: "pipelines", Name: p.Metadata.Name, Check: CheckDanglingRef,
				Reason: "MissingReference", Ref: Singular[kind] + "/" + name,
				Message: "references " + Singular[kind] + " " + name + ", which does not exist",
			})
		}
		check("agentprofiles", spec.ProfileRef.Name)
		for _, ref := range spec.SignalSourceRefs {
			check("signalsources", ref.Name)
		}
		for _, ref := range spec.ChannelRefs {
			check("channels", ref.Name)
		}
		if spec.Toolsets != nil {
			for _, ref := range spec.Toolsets.Refs {
				check("mcptoolsets", ref.Name)
			}
		}
		if spec.MCPConfigs != nil {
			for _, ref := range spec.MCPConfigs.Refs {
				check("mcpconfigs", ref.Name)
			}
		}
	}

	// An unclaimed source silently DROPS signals. The reconciler reports it on
	// the Wired condition, so this finding exists only when that condition has
	// not been written yet — otherwise the same problem would be listed twice.
	claimed := a.claimedSources()
	for _, s := range a.cache.List("signalsources") {
		if claimed[s.Metadata.Name] || s.Condition("Wired") != nil {
			continue
		}
		out = append(out, Finding{
			Kind: "signalsources", Name: s.Metadata.Name, Check: CheckUnclaimedSource,
			Reason:  "NoPipelineClaim",
			Message: "no Pipeline references this source — its signals are dropped until one does",
		})
	}

	for _, ch := range a.cache.List("channels") {
		adapter := decodeSpec[servedSpec](ch.Spec).Adapter
		if adapter == "" || exists("channeladapters", adapter) {
			continue
		}
		out = append(out, Finding{
			Kind: "channels", Name: ch.Metadata.Name, Check: CheckUnservedChannel,
			Reason: "NoServingAdapter", Ref: "ChannelAdapter/" + adapter,
			Message: "names adapter " + adapter + ", for which no ChannelAdapter exists",
		})
	}
	for _, src := range a.cache.List("signalsources") {
		adapter := decodeSpec[servedSpec](src.Spec).Adapter
		if adapter == "" || exists("signaladapters", adapter) {
			continue
		}
		out = append(out, Finding{
			Kind: "signalsources", Name: src.Metadata.Name, Check: CheckUnservedChannel,
			Reason: "NoServingAdapter", Ref: "SignalAdapter/" + adapter,
			Message: "names adapter " + adapter + ", for which no SignalAdapter exists",
		})
	}

	// A profile whose runtimeRef resolves to nothing falls back to "default",
	// then to bootstrap config — so this is a warning, not a failure, and says so.
	for _, prof := range a.cache.List("agentprofiles") {
		ref := decodeSpec[profileSpec](prof.Spec).RuntimeRef.Name
		if ref == "" {
			if !exists("agentruntimes", "default") {
				out = append(out, Finding{
					Kind: "agentprofiles", Name: prof.Metadata.Name, Check: CheckProfileRuntime,
					Reason: "NoDefaultRuntime",
					Message: "declares no runtimeRef and no AgentRuntime named \"default\" exists — " +
						"runs fall back to the manager's bootstrap configuration",
				})
			}
			continue
		}
		if !exists("agentruntimes", ref) {
			out = append(out, Finding{
				Kind: "agentprofiles", Name: prof.Metadata.Name, Check: CheckProfileRuntime,
				Reason: "MissingRuntime", Ref: "AgentRuntime/" + ref,
				Message: "runtimeRef names AgentRuntime " + ref + ", which does not exist",
			})
		}
	}

	// configSchema violations are reported by the manager on the served CR's
	// ConfigValid condition (advisory there too). Surfacing them here as a
	// derived finding would duplicate a reported one, so they are collected from
	// the condition instead — this is the one check the console does NOT redo.
	for _, kind := range []string{"channels", "signalsources"} {
		for _, obj := range a.cache.List(kind) {
			if c := obj.Condition("ConfigValid"); c != nil && c.Status == "False" {
				out = append(out, Finding{
					Kind: kind, Name: obj.Metadata.Name, Check: CheckConfigSchema,
					Reason: c.Reason, Message: c.Message,
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (a *API) claimedSources() map[string]bool {
	claimed := map[string]bool{}
	for _, p := range a.cache.List("pipelines") {
		for _, ref := range decodeSpec[pipelineSpec](p.Spec).SignalSourceRefs {
			claimed[ref.Name] = true
		}
	}
	return claimed
}

// InboundRef is one object that references this one — the "used by these 2
// pipelines" answer, which is what tells an operator whether deleting something
// is safe.
type InboundRef struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Field string `json:"field"`
}

// inboundRefs finds everything pointing at (kind, name).
func (a *API) inboundRefs(kind, name string) []InboundRef {
	var out []InboundRef
	add := func(k, n, field string) { out = append(out, InboundRef{Kind: k, Name: n, Field: field}) }

	for _, p := range a.cache.List("pipelines") {
		spec := decodeSpec[pipelineSpec](p.Spec)
		switch kind {
		case "agentprofiles":
			if spec.ProfileRef.Name == name {
				add("pipelines", p.Metadata.Name, "profileRef")
			}
		case "signalsources":
			for _, ref := range spec.SignalSourceRefs {
				if ref.Name == name {
					add("pipelines", p.Metadata.Name, "signalSourceRefs")
				}
			}
		case "channels":
			for _, ref := range spec.ChannelRefs {
				if ref.Name == name {
					add("pipelines", p.Metadata.Name, "channelRefs")
				}
			}
		case "mcptoolsets":
			if spec.Toolsets != nil {
				for _, ref := range spec.Toolsets.Refs {
					if ref.Name == name {
						add("pipelines", p.Metadata.Name, "toolsets.refs")
					}
				}
			}
		case "mcpconfigs":
			if spec.MCPConfigs != nil {
				for _, ref := range spec.MCPConfigs.Refs {
					if ref.Name == name {
						add("pipelines", p.Metadata.Name, "mcpConfigs.refs")
					}
				}
			}
		}
	}
	switch kind {
	case "channeladapters", "signaladapters":
		served := "channels"
		if kind == "signaladapters" {
			served = "signalsources"
		}
		for _, obj := range a.cache.List(served) {
			if decodeSpec[servedSpec](obj.Spec).Adapter == name {
				add(served, obj.Metadata.Name, "adapter")
			}
		}
		if kind == "channeladapters" {
			// an externally-served SignalAdapter points AT a ChannelAdapter
			for _, sa := range a.cache.List("signaladapters") {
				var spec struct {
					ServedBy *struct{ Name string } `json:"servedBy,omitempty"`
				}
				_ = json.Unmarshal(sa.Spec, &spec)
				if spec.ServedBy != nil && spec.ServedBy.Name == name {
					add("signaladapters", sa.Metadata.Name, "servedBy")
				}
			}
		}
	case "agentruntimes":
		for _, prof := range a.cache.List("agentprofiles") {
			if decodeSpec[profileSpec](prof.Spec).RuntimeRef.Name == name {
				add("agentprofiles", prof.Metadata.Name, "runtimeRef")
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// KindInfo is one entry of the configuration index.
type KindInfo struct {
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Count  int    `json:"count"`
	Synced bool   `json:"synced"`
}

func (a *API) handleKinds(w http.ResponseWriter, r *http.Request) {
	out := make([]KindInfo, 0, len(Kinds))
	for _, k := range Kinds {
		out = append(out, KindInfo{Kind: k, Title: Singular[k], Count: len(a.cache.List(k)), Synced: a.cache.Synced(k)})
	}
	writeJSON(w, http.StatusOK, out)
}

// InventoryRow is one CR in a per-kind listing.
type InventoryRow struct {
	Name string `json:"name"`
	// UID/Created/Labels/Annotations come along because a list is where an
	// operator scans, and leaving them out sends them to kubectl for facts the
	// watch cache already holds.
	UID         string            `json:"uid,omitempty"`
	Created     string            `json:"created,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Health      Health            `json:"health"`
	Conditions  []Condition       `json:"conditions,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	// Columns are the kind-specific facts a list of THIS kind should show — a
	// Pipeline row is not a Channel row, and a generic table would make both
	// useless.
	Columns map[string]string `json:"columns,omitempty"`
	// Findings counts the console's own cross-object problems for this object,
	// so a list flags what a detail view explains.
	Findings int `json:"findings"`
}

func (a *API) handleInventory(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownKind(kind) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown kind " + kind})
		return
	}
	byObject := map[string]int{}
	for _, f := range a.findings() {
		if f.Kind == kind {
			byObject[f.Name]++
		}
	}
	objs := a.cache.List(kind)
	rows := make([]InventoryRow, 0, len(objs))
	for _, o := range objs {
		h, _, _ := health(o)
		rows = append(rows, InventoryRow{
			Name: o.Metadata.Name, UID: o.Metadata.UID,
			Created: o.Metadata.CreationTimestamp,
			Labels:  o.Metadata.Labels, Annotations: o.Metadata.Annotations,
			Health: h, Conditions: o.Conditions(), Summary: summaryLine(o),
			Columns: kindColumns(o), Findings: byObject[o.Metadata.Name],
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

// kindColumns is the per-kind column set. Purpose-built on purpose: a Pipeline
// row shows profile/sources/channels; a Channel row shows adapter and served
// state.
func kindColumns(o *Object) map[string]string {
	cols := map[string]string{}
	switch o.Kind {
	case "pipelines":
		spec := decodeSpec[pipelineSpec](o.Spec)
		cols["profile"] = spec.ProfileRef.Name
		cols["sources"] = joinRefs(spec.SignalSourceRefs)
		cols["channels"] = joinRefs(spec.ChannelRefs)
		if spec.Toolsets != nil {
			cols["toolsets"] = joinRefs(spec.Toolsets.Refs)
			cols["toolsMode"] = spec.Toolsets.Mode
		}
		if spec.MCPConfigs != nil {
			cols["mcpConfigs"] = joinRefs(spec.MCPConfigs.Refs)
		}
	case "channels", "signalsources":
		cols["adapter"] = decodeSpec[servedSpec](o.Spec).Adapter
		if c := o.Condition("Served"); c != nil {
			cols["served"] = c.Status
		}
		if c := o.Condition("Wired"); c != nil {
			cols["wired"] = c.Status
		}
	case "agentprofiles":
		spec := decodeSpec[profileSpec](o.Spec)
		cols["runtime"] = spec.RuntimeRef.Name
		if spec.Repository != nil {
			cols["repository"] = spec.Repository.URL
		}
	case "channeladapters", "signaladapters":
		var spec struct {
			Image    string `json:"image,omitempty"`
			ServedBy *struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"servedBy,omitempty"`
		}
		_ = json.Unmarshal(o.Spec, &spec)
		cols["image"] = spec.Image
		if spec.ServedBy != nil {
			cols["servedBy"] = spec.ServedBy.Kind + "/" + spec.ServedBy.Name
		}
	case "agentruntimes":
		var spec struct {
			Image string `json:"image,omitempty"`
		}
		_ = json.Unmarshal(o.Spec, &spec)
		cols["image"] = spec.Image
	case "mcptoolsets":
		var spec struct {
			Tools []string `json:"tools,omitempty"`
		}
		_ = json.Unmarshal(o.Spec, &spec)
		cols["tools"] = strings.Join(spec.Tools, ", ")
	case "mcpconfigs":
		var spec struct {
			Servers map[string]json.RawMessage `json:"servers,omitempty"`
		}
		_ = json.Unmarshal(o.Spec, &spec)
		keys := make([]string, 0, len(spec.Servers))
		for k := range spec.Servers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		cols["servers"] = strings.Join(keys, ", ")
	case "conversations":
		v := conversationView(o)
		cols["phase"] = v.Status.Phase
		cols["profile"] = v.Spec.ProfileRef.Name
	}
	for k, v := range cols {
		if v == "" {
			delete(cols, k)
		}
	}
	return cols
}

func joinRefs(refs []Ref) string {
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.Name)
	}
	return strings.Join(names, ", ")
}

// Detail is one CR with everything the detail view needs.
type Detail struct {
	Object     *Object      `json:"object"`
	Health     Health       `json:"health"`
	Conditions []Condition  `json:"conditions,omitempty"`
	YAML       string       `json:"yaml"`
	UsedBy     []InboundRef `json:"usedBy"`
	Findings   []Finding    `json:"findings"`
	// Resolved is present on Pipeline detail ONLY, fetched from the manager and
	// rendered verbatim. The console must not recompute capability composition —
	// see manager.go.
	Resolved      *ResolvedCapabilities `json:"resolved,omitempty"`
	ResolvedError string                `json:"resolvedError,omitempty"`
}

func (a *API) handleDetail(w http.ResponseWriter, r *http.Request) {
	kind, name := r.PathValue("kind"), r.PathValue("name")
	if !knownKind(kind) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown kind " + kind})
		return
	}
	obj := a.cache.Get(kind, name)
	if obj == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Singular[kind] + " " + name + " not found"})
		return
	}
	h, _, _ := health(obj)
	out := Detail{
		Object: obj, Health: h, Conditions: obj.Conditions(),
		YAML: objectYAML(obj), UsedBy: a.inboundRefs(kind, name), Findings: []Finding{},
	}
	for _, f := range a.findings() {
		if f.Kind == kind && f.Name == name {
			out.Findings = append(out.Findings, f)
		}
	}
	if kind == "pipelines" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if res, err := a.mgr.Resolved(ctx, name); err == nil {
			out.Resolved = res
		} else {
			out.ResolvedError = err.Error()
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// objectYAML renders the object as YAML, matching what `kubectl get -o yaml`
// shows for the fields the console holds. Reconstructed rather than passed
// through: the cache stores parsed metadata plus untouched spec/status, and
// re-serializing keeps a stable field order.
func objectYAML(o *Object) string {
	apiVersion := APIGroup + "/" + APIVersion
	if gv, ok := groupVersion[o.Kind]; ok {
		apiVersion = strings.TrimPrefix(gv.Group+"/", "/") + gv.Version
	}
	doc := map[string]any{
		"apiVersion": apiVersion,
		"kind":       Singular[o.Kind],
	}
	// round-trip metadata through JSON so the encoder sees plain values
	if b, err := json.Marshal(o.Metadata); err == nil {
		var v any
		if json.Unmarshal(b, &v) == nil {
			doc["metadata"] = v
		}
	}
	for key, raw := range map[string]json.RawMessage{"spec": o.Spec, "status": o.Status} {
		if len(raw) == 0 {
			continue
		}
		var v any
		if json.Unmarshal(raw, &v) == nil {
			doc[key] = v
		}
	}
	return encodeYAML(doc)
}

// summaryLine is the one key fact per kind worth showing in a list.
func summaryLine(o *Object) string {
	switch o.Kind {
	case "pipelines":
		spec := decodeSpec[pipelineSpec](o.Spec)
		parts := []string{"profile " + spec.ProfileRef.Name}
		if n := len(spec.SignalSourceRefs); n > 0 {
			parts = append(parts, plural(n, "source"))
		}
		if n := len(spec.ChannelRefs); n > 0 {
			parts = append(parts, plural(n, "channel"))
		}
		return strings.Join(parts, ", ")
	case "channels", "signalsources":
		if adapter := decodeSpec[servedSpec](o.Spec).Adapter; adapter != "" {
			return "adapter " + adapter
		}
	case "agentprofiles":
		spec := decodeSpec[profileSpec](o.Spec)
		if spec.RuntimeRef.Name != "" {
			return "runtime " + spec.RuntimeRef.Name
		}
	case "conversations":
		v := conversationView(o)
		if v.Status.Phase != "" {
			return strings.ToLower(v.Status.Phase)
		}
	}
	return ""
}

func knownKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
