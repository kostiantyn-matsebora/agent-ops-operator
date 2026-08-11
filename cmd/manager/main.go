// The agentops manager: reconciles Conversations / AgentProfiles / Channels /
// SignalSources and serves the worker + ingest HTTP API.
package main

import (
	"encoding/json"
	"os"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/metrics"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentopsv1alpha1.AddToScheme(scheme))
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

// maxActiveConversations resolves the cap on simultaneously ACTIVE
// conversations (one holding a runtime pod), reporting whether the deprecated
// MAX_RUNTIMES spelling supplied it. The rename is deliberate: the number is
// read by people who think in conversations, and `maxRuntimes` next to
// AgentRuntime CRs reads as a limit on runtime KINDS. MAX_RUNTIMES is honored
// for one release when the new name is unset.
func maxActiveConversations() (n int, deprecated bool) {
	if v, err := strconv.Atoi(os.Getenv("MAX_ACTIVE_CONVERSATIONS")); err == nil && v > 0 {
		return v, false
	}
	if v, err := strconv.Atoi(os.Getenv("MAX_RUNTIMES")); err == nil && v > 0 {
		return v, true
	}
	return 5, false
}

// commandFromEnv parses RUNTIME_COMMAND_JSON (e.g. ["sh","-c","..."]) — used to
// run a stub worker during shadow verification.
func commandFromEnv() []string {
	raw := os.Getenv("RUNTIME_COMMAND_JSON")
	if raw == "" {
		return nil
	}
	var cmd []string
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		return nil
	}
	return cmd
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	setupLog := ctrl.Log.WithName("setup")

	namespace := env("NAMESPACE", "agent-ops")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{namespace: {}},
		},
		Metrics:                 metricsserver.Options{BindAddress: env("METRICS_ADDR", ":9090")},
		HealthProbeBindAddress:  env("PROBE_ADDR", ":8081"),
		LeaderElection:          true,
		LeaderElectionID:        "agentops-manager.agentops.dev",
		LeaderElectionNamespace: namespace,
	})
	if err != nil {
		setupLog.Error(err, "manager")
		os.Exit(1)
	}

	// Channel plumbing: built-in channel types register in-process providers
	// here; every other type is served by an external adapter consuming the op
	// queue over /channel/*. The manager reads no secrets — adapter auth
	// arrives via env (ADAPTER_TOKEN), transport credentials live adapter-side.
	// Per-hop telemetry: bounded, in-memory, lossy by design. Every emission
	// site feeds THIS log, and the log fans out to the Prometheus registry — one
	// instrumentation pass, two consumers, so the console's stream and the
	// cluster's charts cannot drift apart.
	acts := activity.New(envInt("ACTIVITY_BUFFER", activity.DefaultSize))

	registry := chat.NewRegistry()
	ops := &chat.OpQueue{Client: mgr.GetClient(), Namespace: namespace, Registry: registry, Activity: acts}
	router := &chat.Router{Client: mgr.GetClient(), Reader: mgr.GetAPIReader(), Namespace: namespace, Ops: ops, Activity: acts}
	controlURL := env("CONTROL_URL", "http://agentops-manager."+namespace+".svc.cluster.local:8080")

	// ChannelAdapter lifecycle: adapters are plugged in as CRs; the reconciler
	// owns their Deployments (zero-RBAC SA, derived contract token, projected
	// per-channel credentials — all kubelet-resolved, zero Secret API reads).
	if err := (&controller.ChannelAdapterReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ManagerURL:  controlURL,
		MasterToken: os.Getenv("ADAPTER_TOKEN"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "channeladapter controller")
		os.Exit(1)
	}
	if err := (&controller.ChannelReconciler{
		Client:   mgr.GetClient(),
		Registry: registry,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "channel controller")
		os.Exit(1)
	}

	// SignalAdapter lifecycle: the signal sibling of the channel adapter stack
	// (inbound-only contract; same workload machinery and security posture).
	if err := (&controller.SignalAdapterReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ManagerURL:  controlURL,
		MasterToken: os.Getenv("ADAPTER_TOKEN"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "signaladapter controller")
		os.Exit(1)
	}
	if err := (&controller.SignalSourceReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "signalsource controller")
		os.Exit(1)
	}

	// Pipeline wiring: validation-only reconciler; routing reads Ready
	// pipelines at decision time (pipeline-first, source-level fallback).
	if err := (&controller.PipelineReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "pipeline controller")
		os.Exit(1)
	}

	maxActive, deprecatedCap := maxActiveConversations()
	if deprecatedCap {
		setupLog.Info("MAX_RUNTIMES is deprecated and is removed after one release — "+
			"set MAX_ACTIVE_CONVERSATIONS (chart: maxActiveConversations) instead", "cap", maxActive)
	}
	reconciler := &controller.ConversationReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		MaxActiveConversations: maxActive,
		Ops:                    ops,
		Runtime: runtimepod.Config{
			Image:          env("RUNTIME_IMAGE", ""),
			ServiceAccount: env("RUNTIME_SA", "agentops-runtime"),
			ControlURL:     env("CONTROL_URL", "http://agentops-manager."+namespace+".svc.cluster.local:8080"),
			IdleTTLMinutes: envInt("RUNTIME_IDLE_TTL_M", 1),
			HomePVC:        os.Getenv("HOME_PVC"),
			WorkspacePVC:   os.Getenv("WORKSPACE_PVC"),
			NodeSelector:   map[string]string{"node-role/app": "true"},
			Command:        commandFromEnv(),
		},
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "conversation controller")
		os.Exit(1)
	}

	api := &httpapi.Server{
		Client:    mgr.GetClient(),
		Reader:    mgr.GetAPIReader(),
		Namespace: namespace,
		Addr:      env("API_ADDR", ":8080"),
		Ops:       ops,
		Router:    router,
		Activity:  acts,
		Version:   env("VERSION", "dev"),
		// The same bootstrap config the reconciler builds pods from: dispatch
		// asks it whether this deployment can carry conversation context.
		Runtime: reconciler.Runtime,
		// The backlog bound: the one capacity check that must live in ingest,
		// because the point is not to create the object at all.
		MaxQueuedConversations: envInt("MAX_QUEUED_CONVERSATIONS", 50),
		MaxActiveConversations: maxActive,
		AdapterToken:           os.Getenv("ADAPTER_TOKEN"),
	}
	if err := mgr.Add(api); err != nil {
		setupLog.Error(err, "http api")
		os.Exit(1)
	}

	// The metric set observes the activity log and samples the same in-memory
	// state GET /status reports, into the registry already serving :9090. The
	// counters exist so alerting never depends on anyone having a browser open.
	collector := metrics.New(api.MetricsSample)
	collector.MustRegister()
	acts.AddObserver(collector)

	utilruntime.Must(mgr.AddHealthzCheck("healthz", healthz.Ping))
	utilruntime.Must(mgr.AddReadyzCheck("readyz", healthz.Ping))

	setupLog.Info("starting agentops manager", "namespace", namespace)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "run")
		os.Exit(1)
	}
}
