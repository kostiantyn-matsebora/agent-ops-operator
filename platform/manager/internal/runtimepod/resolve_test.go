package runtimepod

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Resolution is TWO chains, and the whole point of pinning them here is that a
// reader can see both orders at once. Design D3:
//
//	runtime:  conversation -> profile (DEPRECATED) -> default -> bootstrap
//	identity: conversation -> runtime -> bootstrap (the chart's default)
//
// The Pipeline appears in NEITHER, on purpose: its fields were resolved into
// the conversation at creation. A test that resolved through a Pipeline here
// would be pinning the privilege bug the snapshot exists to prevent.

const testNS = "agent-ops"

func resolveScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return s
}

func rt(name, image, sa string) *agentopsv1alpha1.AgentRuntime {
	r := &agentopsv1alpha1.AgentRuntime{}
	r.Namespace, r.Name = testNS, name
	r.Spec.Image = image
	r.Spec.ServiceAccountName = sa
	return r
}

func prof(name string, runtimeRef string) *agentopsv1alpha1.AgentProfile {
	p := &agentopsv1alpha1.AgentProfile{}
	p.Namespace, p.Name = testNS, name
	if runtimeRef != "" {
		p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: runtimeRef}
	}
	return p
}

func conv(profile, runtimeRef, sa string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Namespace, c.Name = testNS, "conv-1"
	c.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	if runtimeRef != "" {
		c.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: runtimeRef}
	}
	c.Spec.ServiceAccountName = sa
	return c
}

func TestResolveForWalksBothChains(t *testing.T) {
	bootstrap := Config{Image: "bootstrap-image", ServiceAccount: "agentops-runtime"}

	objs := []client.Object{
		rt("default", "default-image", "agentops-runtime"),
		rt("acting", "acting-image", "agentops-runtime-acting"),
		rt("saless", "saless-image", ""),
		prof("plain", ""),
		prof("legacy", "acting"),
	}

	cases := []struct {
		name    string
		conv    *agentopsv1alpha1.Conversation
		image   string
		account string
		why     string
	}{{
		name: "conversation snapshot wins over everything",
		// The snapshot is FIRST because it is the frozen answer. A conversation
		// carrying both fields is what a Pipeline naming both produced.
		conv:    conv("legacy", "acting", "route-sa"),
		image:   "acting-image",
		account: "route-sa",
		why:     "the snapshot is the resolved answer and nothing re-derives it",
	}, {
		name:    "a snapshotted account overrides the runtime's own",
		conv:    conv("plain", "default", "route-sa"),
		image:   "default-image",
		account: "route-sa",
		why:     "one runtime image, several trust levels — the whole point of the field",
	}, {
		name:    "a snapshotted runtime with no account keeps the runtime's",
		conv:    conv("plain", "acting", ""),
		image:   "acting-image",
		account: "agentops-runtime-acting",
		why:     "absent means the runtime's, not empty",
	}, {
		name: "DEPRECATED: the profile ref is read when the snapshot has none",
		// Delete this row with AgentProfileSpec.RuntimeRef.
		conv:    conv("legacy", "", ""),
		image:   "acting-image",
		account: "agentops-runtime-acting",
		why:     "a profile applied before the upgrade keeps dispatching where it named",
	}, {
		name:    "a conversation predating the snapshot falls through to default",
		conv:    conv("plain", "", ""),
		image:   "default-image",
		account: "agentops-runtime",
		why:     "nothing backfills, so the old chain must still terminate",
	}, {
		name:    "a runtime naming no account keeps the bootstrap default",
		conv:    conv("plain", "saless", ""),
		image:   "saless-image",
		account: "agentops-runtime",
		why:     "FromRuntime leaves the fallback account when the runtime declares none",
	}, {
		name:    "a snapshotted account survives an unknown profile",
		conv:    conv("gone", "default", "route-sa"),
		image:   "default-image",
		account: "route-sa",
		why:     "an unreadable profile answers no runtime rather than failing the run",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).WithObjects(objs...).Build()
			got, err := ResolveFor(context.Background(), c, testNS, tc.conv, bootstrap)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got.Config.Image != tc.image {
				t.Errorf("image = %q, want %q — %s", got.Config.Image, tc.image, tc.why)
			}
			if got.Config.ServiceAccount != tc.account {
				t.Errorf("service account = %q, want %q — %s", got.Config.ServiceAccount, tc.account, tc.why)
			}
		})
	}
}

// A runtime the conversation NAMES must exist: dispatching to a different
// backend than the one wired is worse than not dispatching, because the pod
// would come up with different power and report success.
func TestANamedRuntimeThatIsMissingFails(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).
		WithObjects(prof("plain", "")).Build()

	if _, err := ResolveFor(context.Background(), c, testNS,
		conv("plain", "no-such-runtime", ""), Config{}); err == nil {
		t.Fatal("a named runtime that does not exist must fail resolution")
	}
}

// Nothing named, nothing found: the bootstrap config answers, and a snapshotted
// account still overrides it. An install with no AgentRuntime CR at all is the
// oldest supported shape and must not lose the field.
func TestBootstrapFallbackKeepsTheSnapshottedAccount(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).
		WithObjects(prof("plain", "")).Build()

	got, err := ResolveFor(context.Background(), c, testNS,
		conv("plain", "", "route-sa"), Config{Image: "bootstrap-image", ServiceAccount: "agentops-runtime"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Config.Image != "bootstrap-image" {
		t.Errorf("image = %q, want the bootstrap one", got.Config.Image)
	}
	if got.Config.ServiceAccount != "route-sa" {
		t.Errorf("service account = %q, want route-sa — a named identity means that identity, whichever backend answers", got.Config.ServiceAccount)
	}
}

// ContinuityPossible keeps its MEANING: it reads the resolved runtime's
// contextStorage, and only the resolution changed. Two conversations with the
// same runtime must answer identically however they reached it.
func TestContinuityFollowsTheResolvedRuntimeNotTheRoute(t *testing.T) {
	keeps := rt("keeps", "img", "")
	keeps.Spec.ContextStorage = agentopsv1alpha1.ContextExternal
	c := fake.NewClientBuilder().WithScheme(resolveScheme(t)).
		WithObjects(keeps, prof("plain", ""), prof("legacy", "keeps")).Build()

	viaSnapshot, err := ResolveFor(context.Background(), c, testNS, conv("plain", "keeps", "a-sa"), Config{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	viaProfile, err := ResolveFor(context.Background(), c, testNS, conv("legacy", "", ""), Config{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !viaSnapshot.ContinuityPossible() || !viaProfile.ContinuityPossible() {
		t.Fatal("continuity is a fact about the RUNTIME — the route that selected it changes nothing")
	}
}
