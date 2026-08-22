// The agentops manager: reconciles Conversations / AgentProfiles / Channels /
// SignalSources and serves the worker + ingest HTTP API.
package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/metrics"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/storagebreaker"
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

// envBool reads an explicit opt-in. Anything that is not a recognised true
// value is false: both retention flags are destructive in their own way, so a
// typo must decline them rather than enable them.
func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// envDuration reads a Go duration ("720h", "30m"). An unparseable or
// non-positive value yields zero, which every caller reads as "off".
func envDuration(key string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// envDurationOr is envDuration for settings whose "off" is not zero. A start
// deadline of zero would reap every pod the instant it was created, so an
// unset or unparseable value must fall back to the default rather than to nil
// behaviour.
func envDurationOr(key string, def time.Duration) time.Duration {
	if d := envDuration(key); d > 0 {
		return d
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
	// ONE breaker, both edges. The HTTP API feeds it runs that could not reach
	// their context; the reconciler feeds it pods that could not attach their
	// volume. Two instances would be two judgements about whether storage is
	// down, disagreeing at the worst possible moment.
	breaker := storagebreaker.New()
	reconciler := &controller.ConversationReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		MaxActiveConversations: maxActive,
		// A runtime pod that never STARTS holds its slot until it is reaped.
		// Generous by default: it must clear a cold pull of a large runtime
		// image, because reaping a pod that was merely still pulling turns a
		// slow start into a restart loop that never completes one.
		RuntimeStartDeadline: envDurationOr("RUNTIME_START_DEADLINE", controller.DefaultRuntimeStartDeadline),
		// Events REPORT; nothing reads them back. The reconciler holds the
		// recorder so a pod that cannot start says so where an operator running
		// `kubectl describe conversation` is already looking.
		Recorder:       mgr.GetEventRecorderFor("agentops-conversation"),
		StorageBreaker: breaker,
		// The same log every other emission site feeds, so the runtime-start hop
		// lands in the console's sequence beside the ones around it.
		Activity: acts,
		// Paired with the chart's rbac.drainAware: the behaviour and the
		// ClusterRole that makes it possible ship together, or enabling it
		// alone would only produce forbidden loops in the log.
		DrainAware: envBool("DRAIN_AWARE"),
		Ops:      ops,
		// The timer closes through the SAME path /close does, which is why the
		// reconciler holds the router at all.
		Router: router,
		// Both OFF by default and independent: autoclose with autodelete off —
		// a lane that tidies itself and keeps its record — is the common
		// configuration, so enabling one must never imply the other.
		AutoCloseEnabled:    envBool("CONVERSATION_AUTOCLOSE_ENABLED"),
		AutoCloseIdleAge:    envDuration("CONVERSATION_AUTOCLOSE_IDLE_AGE"),
		AutoDeleteEnabled:   envBool("CONVERSATION_AUTODELETE_ENABLED"),
		AutoDeleteClosedAge: envDuration("CONVERSATION_AUTODELETE_CLOSED_AGE"),
		Runtime: runtimepod.Config{
			Image:          env("RUNTIME_IMAGE", ""),
			ServiceAccount: env("RUNTIME_SA", "agentops-runtime"),
			ControlURL:     env("CONTROL_URL", "http://agentops-manager."+namespace+".svc.cluster.local:8080"),
			IdleTTLMinutes: envInt("RUNTIME_IDLE_TTL_M", 1),
			HomePVC:        os.Getenv("HOME_PVC"),
			WorkspacePVC:   os.Getenv("WORKSPACE_PVC"),
			NodeSelector:   map[string]string{"node-role/app": "true"},
			Command:        commandFromEnv(),
			// The sidecar that keeps context durable when a runtime declares
			// contextSync. Release-wide, like the manager's own image: it
			// implements a contract rather than being a backend choice. Empty
			// disables sidecar mode outright, so a chart that has not been
			// upgraded cannot half-apply it.
			ContextSyncImage:     env("CONTEXT_SYNC_IMAGE", ""),
			// Empty disables mediation outright, the same way an empty
			// CONTEXT_SYNC_IMAGE disables the sidecar: a redirect with no proxy
			// to answer it is a pod that reaches nothing.
			EgressProxyImage: env("EGRESS_PROXY_IMAGE", ""),
			EgressInitImage:  env("EGRESS_INIT_IMAGE", ""),
			ContextLiveSizeLimit: env("CONTEXT_LIVE_SIZE_LIMIT", "4Gi"),
		},
	}
	// Assigned rather than passed at construction: the router is built before the
	// reconciler that owns this value, and copying the literal into both would be
	// two spellings of one fact. /exit uses it for exactly one question — whether
	// a released conversation keeps its context.
	router.Runtime = reconciler.Runtime
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
		StorageBreaker:         breaker,
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
