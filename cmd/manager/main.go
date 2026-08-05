// The agentops manager: reconciles Conversations / AgentProfiles / Channels /
// SignalSources and serves the worker + ingest HTTP API.
package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
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

	// ChatFactory: resolves a Channel to a provider; the bot token is the
	// manager's own credential (scoped secret read in its namespace).
	chatFactory := func(ctx context.Context, ch *agentopsv1alpha1.Channel) (chat.Provider, error) {
		if ch.Spec.Telegram == nil {
			return nil, nil
		}
		var sec corev1.Secret
		if err := mgr.GetAPIReader().Get(ctx, types.NamespacedName{
			Namespace: ch.Namespace, Name: ch.Spec.Telegram.BotTokenSecretRef.Name,
		}, &sec); err != nil {
			return nil, err
		}
		token := string(sec.Data[ch.Spec.Telegram.BotTokenSecretRef.Key])
		return chat.NewTelegram(token, ch.Spec.Telegram.ChatID), nil
	}

	reconciler := &controller.ConversationReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		MaxRuntimes: envInt("MAX_RUNTIMES", 8),
		ChatFactory: chatFactory,
		Runtime: runtimepod.Config{
			Image:          env("RUNTIME_IMAGE", ""),
			ServiceAccount: env("RUNTIME_SA", "agentops-runtime"),
			ControlURL:     env("CONTROL_URL", "http://agentops-manager."+namespace+".svc.cluster.local:8080"),
			IdleTTLMinutes: envInt("RUNTIME_IDLE_TTL_M", 10),
			HomePVC:        os.Getenv("HOME_PVC"),
			NodeSelector:   map[string]string{"node-role/app": "true"},
			Command:        commandFromEnv(),
		},
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "conversation controller")
		os.Exit(1)
	}

	if err := mgr.Add(&httpapi.Server{
		Client:    mgr.GetClient(),
		Reader:    mgr.GetAPIReader(),
		Namespace: namespace,
		Addr:      env("API_ADDR", ":8080"),
	}); err != nil {
		setupLog.Error(err, "http api")
		os.Exit(1)
	}

	// Telegram poller (leader-only): serves Channels with pollingEnabled.
	if err := mgr.Add(&chat.Poller{
		Client:    mgr.GetClient(),
		Reader:    mgr.GetAPIReader(),
		Namespace: namespace,
		Token: func(ctx context.Context, ch *agentopsv1alpha1.Channel) (string, error) {
			var sec corev1.Secret
			if err := mgr.GetAPIReader().Get(ctx, types.NamespacedName{
				Namespace: ch.Namespace, Name: ch.Spec.Telegram.BotTokenSecretRef.Name,
			}, &sec); err != nil {
				return "", err
			}
			return string(sec.Data[ch.Spec.Telegram.BotTokenSecretRef.Key]), nil
		},
	}); err != nil {
		setupLog.Error(err, "telegram poller")
		os.Exit(1)
	}

	utilruntime.Must(mgr.AddHealthzCheck("healthz", healthz.Ping))
	utilruntime.Must(mgr.AddReadyzCheck("readyz", healthz.Ping))

	setupLog.Info("starting agentops manager", "namespace", namespace)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "run")
		os.Exit(1)
	}
}
