package main

import (
	"net/url"
	"sort"
	"strings"
)

// The facts the Components and Infrastructure views are drawn from.
//
// The console still asserts nothing the cluster does not say. A component is
// a Deployment, a CronJob, an adapter CR, a runtime image a pod or a runtime
// declares, an MCP server a config names, a repository a profile names, or a
// system an adapter declares it faces. The views are functions of these facts
// computed in the browser, where the hop mapping lives.

// Component roles. Where the activity vocabulary names a node kind the role IS
// that kind, so a hop's endpoint is a component id as it stands.
const (
	RoleManager        = "manager"
	RoleSignalAdapter  = "signal-adapter"
	RoleChannelAdapter = "channel-adapter"
	RoleRuntimeImage   = "runtime-image"
	RoleModel          = "model"
	RoleMCPServer      = "mcp-server"
	RoleExternal       = "external"
	RoleGateway        = "gateway"
	RoleContextSync    = "context-sync"
	RoleEgressProxy    = "egress-proxy"
	RoleHousekeeping   = "housekeeping"
	RoleRepository     = "repository"
	// RoleWorkload is a Deployment or CronJob the console recognises as none
	// of the above. Drawn rather than dropped: it runs in the namespace.
	RoleWorkload = "workload"
)

// Component is one node of the Components view.
type Component struct {
	// ID is `<role>/<name>`.
	ID   string `json:"id"`
	Role string `json:"role"`
	Name string `json:"name"`
	// Image is what the component runs, where one is known.
	Image string `json:"image,omitempty"`
	// Count is the pods running it now: a runtime image in three
	// conversation pods is ONE component with a count of three.
	Count int `json:"count"`
	// Health, Reason and Message come from the adapter CR's own condition for
	// an adapter, and are `none` for everything else — a component's pods
	// carry their own health, and the console adds no judgement of its own.
	Health  Health `json:"health"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	Bundle  string `json:"bundle,omitempty"`
	// Implements lists the Model node ids this component carries: an
	// adapter's CR, the runtimes on an image, the MCPConfigs naming a server,
	// the profiles checking out a repository.
	Implements []string `json:"implements,omitempty"`
	// Workload is the Deployment or CronJob it runs as, as `<kind>/<name>`.
	Workload string `json:"workload,omitempty"`
	// ServedBy is the component whose process serves this one.
	ServedBy string `json:"servedBy,omitempty"`
	// ExternalKind is an external's declared kind: sender, api or kubernetes.
	ExternalKind string `json:"externalKind,omitempty"`
	// Harness and Vendor are a runtime image's, where this repository built it.
	Harness string `json:"harness,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
	// URL is an MCP server's or a repository's address, as declared.
	URL string `json:"url,omitempty"`
}

// Pod is one node of the Infrastructure view.
type Pod struct {
	ID   string `json:"id"` // pods/<name>
	Name string `json:"name"`
	// Component is the component id this pod runs.
	Component string `json:"component,omitempty"`
	// ClusterNode is spec.nodeName. Empty while unscheduled.
	ClusterNode string `json:"clusterNode,omitempty"`
	Phase       string `json:"phase,omitempty"`
	// Health is from pod phase and container state alone.
	Health Health `json:"health"`
	Reason string `json:"reason,omitempty"`
	// Conversation and Pipeline are a runtime pod's, as panel attributes.
	Conversation string      `json:"conversation,omitempty"`
	Pipeline     string      `json:"pipeline,omitempty"`
	Containers   []Container `json:"containers"`
}

// Container is one container of a pod, so a conversation pod can open into
// its agent and sidecars.
type Container struct {
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
	// Role is the component role this container is, on a runtime pod:
	// runtime-image, context-sync or egress-proxy.
	Role     string `json:"role,omitempty"`
	Ready    bool   `json:"ready"`
	Restarts int    `json:"restarts"`
}

// Names the manager and the chart give things, each the one place it is
// spelled on this side.
const (
	managerDeployment      = "agentops-manager"
	housekeepingCronJob    = "agentops-housekeeping"
	channelAdapterPrefix   = "agentops-adapter-"
	signalAdapterPrefix    = "agentops-signal-"
	gatewayPrefix          = "agentops-gateway-"
	runtimePodAppLabel     = "app.kubernetes.io/name"
	runtimePodAppValue     = "agentops-runtime"
	runtimePodConversation = "agentops.dev/conversation"
	runtimeWorkerContainer = "worker"
	contextSyncImageEnv    = "CONTEXT_SYNC_IMAGE"
	egressProxyImageEnv    = "EGRESS_PROXY_IMAGE"
)

type cronJobSpecView struct {
	JobTemplate struct {
		Spec struct {
			Template struct {
				Spec podSpecView `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	} `json:"jobTemplate"`
}

type mcpConfigSpecView struct {
	Servers map[string]struct {
		URL string `json:"url,omitempty"`
	} `json:"servers,omitempty"`
}

func componentID(role, name string) string { return role + "/" + name }

type componentSet struct {
	byID  map[string]*Component
	edges []Edge
}

func (s *componentSet) ensure(role, name string) *Component {
	id := componentID(role, name)
	if c := s.byID[id]; c != nil {
		return c
	}
	c := &Component{ID: id, Role: role, Name: name, Health: HealthNone}
	s.byID[id] = c
	return c
}

func (c *Component) implement(id string) {
	for _, have := range c.Implements {
		if have == id {
			return
		}
	}
	c.Implements = append(c.Implements, id)
}

// buildComponents derives the Components and Infrastructure facts.
func buildComponents(c *Cache, t *Topology) {
	set := &componentSet{byID: map[string]*Component{}}
	workloads := map[string]string{} // "<kind>/<name>" -> component id

	manager := set.ensure(RoleManager, RoleManager)
	if d := c.Get("deployments", managerDeployment); d != nil {
		manager.Workload = nodeID("deployments", managerDeployment)
		manager.Image = firstImage(decodeSpec[deploySpecView](d.Spec).Template.Spec)
		workloads[manager.Workload] = manager.ID
		addSidecars(set, decodeSpec[deploySpecView](d.Spec).Template.Spec)
	}

	addAdapters(c, set, workloads)
	addRuntimeImages(c, set)
	addMCPServers(c, set, workloads)
	addRepositories(c, set)

	for _, d := range c.List("deployments") {
		key := nodeID("deployments", d.Metadata.Name)
		if _, claimed := workloads[key]; claimed {
			continue
		}
		role, name := RoleWorkload, d.Metadata.Name
		if g, ok := strings.CutPrefix(d.Metadata.Name, gatewayPrefix); ok && g != "" {
			role, name = RoleGateway, g
		}
		comp := set.ensure(role, name)
		comp.Workload, comp.Image = key, firstImage(decodeSpec[deploySpecView](d.Spec).Template.Spec)
		workloads[key] = comp.ID
	}
	for _, cj := range c.List("cronjobs") {
		role, name := RoleWorkload, cj.Metadata.Name
		if cj.Metadata.Name == housekeepingCronJob {
			role, name = RoleHousekeeping, RoleHousekeeping
		}
		comp := set.ensure(role, name)
		comp.Workload = nodeID("cronjobs", cj.Metadata.Name)
		comp.Image = firstImage(decodeSpec[cronJobSpecView](cj.Spec).JobTemplate.Spec.Template.Spec)
		workloads[comp.Workload] = comp.ID
	}

	t.Pods = buildPods(c, set, workloads)
	t.Components = set.sorted()
	sortEdges(set.edges)
	t.ComponentEdges = set.edges
	if t.ComponentEdges == nil {
		t.ComponentEdges = []Edge{}
	}
}

func (s *componentSet) sorted() []Component {
	out := make([]Component, 0, len(s.byID))
	for _, c := range s.byID {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func firstImage(spec podSpecView) string {
	if len(spec.Containers) == 0 {
		return ""
	}
	return spec.Containers[0].Image
}

// addSidecars reads the sidecar images the manager puts into runtime pods
// from its own environment, so they are components before any pod runs.
func addSidecars(set *componentSet, spec podSpecView) {
	for _, ctr := range spec.Containers {
		for _, e := range ctr.Env {
			switch {
			case e.Name == contextSyncImageEnv && e.Value != "":
				set.ensure(RoleContextSync, RoleContextSync).Image = e.Value
			case e.Name == egressProxyImageEnv && e.Value != "":
				set.ensure(RoleEgressProxy, RoleEgressProxy).Image = e.Value
			}
		}
	}
}

// addAdapters makes each adapter CR a component, with its workload, and each
// declared external a component joined to it.
func addAdapters(c *Cache, set *componentSet, workloads map[string]string) {
	for _, kind := range []string{"channeladapters", "signaladapters"} {
		role, prefix := RoleChannelAdapter, channelAdapterPrefix
		if kind == "signaladapters" {
			role, prefix = RoleSignalAdapter, signalAdapterPrefix
		}
		for _, obj := range c.List(kind) {
			spec := decodeSpec[adapterSpec](obj.Spec)
			comp := set.ensure(role, obj.Metadata.Name)
			comp.Health, comp.Reason, comp.Message = health(obj)
			comp.Image, comp.Bundle = spec.Image, bundleOf(obj)
			comp.implement(nodeID(kind, obj.Metadata.Name))
			if spec.ServedBy != nil && spec.ServedBy.Name != "" {
				comp.ServedBy = componentID(RoleChannelAdapter, spec.ServedBy.Name)
			} else if c.Get("deployments", prefix+obj.Metadata.Name) != nil {
				comp.Workload = nodeID("deployments", prefix+obj.Metadata.Name)
				workloads[comp.Workload] = comp.ID
			}
			for _, ext := range spec.Externals {
				if ext.Name == "" {
					continue
				}
				e := set.ensure(RoleExternal, ext.Name)
				if e.ExternalKind == "" {
					e.ExternalKind = ext.Kind
				}
				// A sender pushes to the adapter. Everything else is called by it.
				edge := Edge{From: comp.ID, To: e.ID, Kind: "calls"}
				if ext.Kind == "sender" {
					edge = Edge{From: e.ID, To: comp.ID, Kind: "sends"}
				}
				set.edges = append(set.edges, edge)
			}
		}
	}
}

// addRuntimeImages makes each declared runtime's image a component. Two
// runtimes on one image are one component implementing both.
func addRuntimeImages(c *Cache, set *componentSet) {
	for _, rt := range c.List("agentruntimes") {
		image := decodeSpec[runtimeSpec](rt.Spec).Image
		if image == "" {
			continue
		}
		comp := set.ensure(RoleRuntimeImage, image)
		comp.Image = image
		comp.Harness, comp.Vendor = runtimeFacts(image)
		comp.implement(nodeID("agentruntimes", rt.Metadata.Name))
	}
}

// addMCPServers makes each server an MCPConfig names a component, named by
// its key — the name a tool call's hop carries. A server whose URL host is a
// Deployment in this namespace runs as that Deployment.
func addMCPServers(c *Cache, set *componentSet, workloads map[string]string) {
	for _, cfg := range c.List("mcpconfigs") {
		for key, srv := range decodeSpec[mcpConfigSpecView](cfg.Spec).Servers {
			comp := set.ensure(RoleMCPServer, key)
			comp.implement(nodeID("mcpconfigs", cfg.Metadata.Name))
			if comp.URL == "" {
				comp.URL = srv.URL
			}
			if host := serviceOf(srv.URL); host != "" && comp.Workload == "" {
				if d := c.Get("deployments", host); d != nil {
					comp.Workload = nodeID("deployments", host)
					comp.Image = firstImage(decodeSpec[deploySpecView](d.Spec).Template.Spec)
					workloads[comp.Workload] = comp.ID
				}
			}
		}
	}
}

// serviceOf is the in-cluster service a URL names: the first label of its
// host. `http://agentops-mcp-k8s.agent-ops.svc:8080/mcp` names
// `agentops-mcp-k8s`.
func serviceOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host, _, _ := strings.Cut(u.Hostname(), ".")
	return host
}

func addRepositories(c *Cache, set *componentSet) {
	for _, p := range c.List("agentprofiles") {
		repo := decodeSpec[profileSpec](p.Spec).Repository
		if repo == nil || repo.URL == "" {
			continue
		}
		comp := set.ensure(RoleRepository, repo.URL)
		comp.URL = repo.URL
		comp.implement(nodeID("agentprofiles", p.Metadata.Name))
	}
}

// buildPods reads every pod and joins it to the component it runs. A runtime
// pod runs the image of its worker container, and its sidecars are counted
// on their own components.
func buildPods(c *Cache, set *componentSet, workloads map[string]string) []Pod {
	pipelines := c.List("pipelines")
	out := []Pod{}
	for _, obj := range c.List("pods") {
		spec := decodeSpec[podSpecView](obj.Spec)
		status := decodeSpec[podStatusView](obj.Status)
		pod := Pod{
			ID: nodeID("pods", obj.Metadata.Name), Name: obj.Metadata.Name,
			ClusterNode: spec.NodeName, Phase: status.Phase,
			Health: podHealth(obj), Reason: podProblem(status), Containers: []Container{},
		}
		runtimePod := obj.Metadata.Labels[runtimePodAppLabel] == runtimePodAppValue
		live := status.Phase != "Succeeded" && status.Phase != "Failed"
		for _, ctr := range spec.Containers {
			cont := Container{Name: ctr.Name, Image: ctr.Image}
			for _, cs := range status.ContainerStatuses {
				if cs.Name == ctr.Name {
					cont.Ready, cont.Restarts = cs.Ready, cs.RestartCount
				}
			}
			if runtimePod {
				cont.Role = runtimeContainerRole(ctr.Name)
				if comp := runtimeContainerComponent(set, cont); comp != nil {
					if cont.Role == RoleRuntimeImage {
						pod.Component = comp.ID
					}
					if live {
						comp.Count++
					}
				}
			}
			pod.Containers = append(pod.Containers, cont)
		}
		if runtimePod {
			if conv := obj.Metadata.Labels[runtimePodConversation]; conv != "" {
				pod.Conversation = conv
				if o := c.Get("conversations", conv); o != nil {
					pod.Pipeline = AttributePipeline(o, pipelines)
				}
			}
		} else if id := ownerComponent(obj.Metadata.Name, workloads); id != "" {
			pod.Component = id
			if comp := set.byID[id]; comp != nil && live {
				comp.Count++
			}
		}
		out = append(out, pod)
	}
	return out
}

func runtimeContainerRole(name string) string {
	switch name {
	case runtimeWorkerContainer:
		return RoleRuntimeImage
	case RoleContextSync:
		return RoleContextSync
	case RoleEgressProxy:
		return RoleEgressProxy
	}
	return ""
}

func runtimeContainerComponent(set *componentSet, cont Container) *Component {
	switch cont.Role {
	case RoleRuntimeImage:
		if cont.Image == "" {
			return nil
		}
		comp := set.ensure(RoleRuntimeImage, cont.Image)
		comp.Image = cont.Image
		if comp.Harness == "" {
			comp.Harness, comp.Vendor = runtimeFacts(cont.Image)
		}
		return comp
	case RoleContextSync, RoleEgressProxy:
		comp := set.ensure(cont.Role, cont.Role)
		if comp.Image == "" {
			comp.Image = cont.Image
		}
		return comp
	}
	return nil
}

// ownerComponent finds the workload a pod belongs to by its generated name —
// `<deployment>-<hash>-<suffix>` or `<cronjob>-<hash>-<suffix>`. The LONGEST
// matching workload wins, so `agentops-mcp-k8s-admin-…` is never credited to
// `agentops-mcp-k8s`.
func ownerComponent(pod string, workloads map[string]string) string {
	best, id := "", ""
	for key, comp := range workloads {
		_, name, _ := strings.Cut(key, "/")
		if strings.HasPrefix(pod, name+"-") && len(name) > len(best) {
			best, id = name, comp
		}
	}
	return id
}

// addObservedComponents adds the components only a hop names: a model is
// known by the calls made to it, and nothing declares one ahead of time.
func addObservedComponents(t *Topology, stats []EdgeStat) {
	have := map[string]bool{}
	for _, c := range t.Components {
		have[c.ID] = true
	}
	for _, s := range stats {
		for _, ref := range []NodeRef{s.From, s.To} {
			switch ref.Kind {
			case RoleModel, RoleMCPServer, RoleRuntimeImage, RoleExternal:
			default:
				continue
			}
			id := componentID(ref.Kind, ref.Name)
			if ref.Name == "" || have[id] {
				continue
			}
			have[id] = true
			t.Components = append(t.Components, Component{ID: id, Role: ref.Kind, Name: ref.Name, Health: HealthNone})
		}
	}
	sort.Slice(t.Components, func(i, j int) bool { return t.Components[i].ID < t.Components[j].ID })
}
