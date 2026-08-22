package chat

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

func pipeline(name, profile string, ready bool) *agentopsv1alpha1.Pipeline {
	p := &agentopsv1alpha1.Pipeline{}
	p.Namespace, p.Name = testNS, name
	p.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	p.Status.Conditions = []metav1.Condition{{
		Type: "Ready", Status: status, Reason: "T", LastTransitionTime: metav1.Now(),
	}}
	return p
}

func find(v Vocabulary, name string) (Entry, bool) {
	for _, e := range v.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// The built-ins and their POSITIONS are the contract a surface filters on, so
// they are pinned by name. `agents` must be absent: it still works, but
// publishing it would register the retired noun into a transport's own menu.
func TestVocabularyPinsBuiltinsAndPositions(t *testing.T) {
	r, _, _ := closeFixture(t)
	v := r.Vocabulary(context.Background())

	for name, want := range map[string]Position{
		"pipelines": PositionGeneral,
		"help":      PositionGeneral,
		"exit":      PositionThread,
		"close":     PositionThread,
	} {
		e, ok := find(v, name)
		if !ok {
			t.Fatalf("builtin %q missing from vocabulary", name)
		}
		if e.Kind != KindBuiltin {
			t.Fatalf("builtin %q: kind %q", name, e.Kind)
		}
		if e.Position != want {
			t.Fatalf("builtin %q: position %q, want %q", name, e.Position, want)
		}
		if e.Description == "" {
			t.Fatalf("builtin %q has no description — it is menu text", name)
		}
	}
	if _, ok := find(v, RetiredListCommand); ok {
		t.Fatalf("retired listing name is published; it must work but never be offered")
	}
}

// Only Ready Pipelines are addressable, and the description comes from what the
// Pipeline already declares — no CRD field was added to carry prose.
func TestVocabularyPublishesOnlyReadyPipelines(t *testing.T) {
	r, _, _ := closeFixture(t,
		pipeline("k8s-observe", "k8s-engineer", true),
		pipeline("half-wired", "nobody", false),
	)
	v := r.Vocabulary(context.Background())

	e, ok := find(v, "k8s-observe")
	if !ok {
		t.Fatal("ready pipeline missing")
	}
	if e.Kind != KindPipeline || e.Position != PositionGeneral {
		t.Fatalf("pipeline entry: kind=%q position=%q", e.Kind, e.Position)
	}
	if e.Profile != "k8s-engineer" || e.Description != "k8s-engineer" {
		t.Fatalf("profile/description derived wrong: %+v", e)
	}
	if _, ok := find(v, "half-wired"); ok {
		t.Fatal("unready pipeline offered — it names wiring that does not resolve")
	}
}

// A hyphenated name is published UNCHANGED. Telegram cannot register one, and
// that is Telegram's problem to solve in its own adapter — the moment this
// package re-spells a name to suit a transport, the division is gone.
func TestVocabularyNeverRespellsForATransport(t *testing.T) {
	r, _, _ := closeFixture(t, pipeline("k8s-observe", "p", true))
	v := r.Vocabulary(context.Background())
	if _, ok := find(v, "k8s-observe"); !ok {
		t.Fatal("hyphenated name not published verbatim")
	}
	if _, ok := find(v, "k8s_observe"); ok {
		t.Fatal("manager published a transport-local spelling")
	}
}

// The revision is DERIVED, so the same entries must hash the same across
// processes — and an unrelated Pipeline edit must not wake every adapter.
func TestRevisionIgnoresUnpublishedFields(t *testing.T) {
	base := pipeline("k8s-observe", "k8s-engineer", true)
	r1, _, _ := closeFixture(t, base.DeepCopy())
	before := r1.Vocabulary(context.Background()).Revision

	noisy := base.DeepCopy()
	noisy.ResourceVersion = ""
	noisy.Labels = map[string]string{"unrelated": "edit"}
	noisy.Spec.SignalSourceRefs = []agentopsv1alpha1.ObjectRef{{Name: "some-source"}}
	r2, _, _ := closeFixture(t, noisy)
	after := r2.Vocabulary(context.Background()).Revision

	if before != after {
		t.Fatalf("revision changed on an unpublished field: %s -> %s", before, after)
	}
	if before == "" {
		t.Fatal("revision is empty")
	}
}

func TestRevisionChangesWhenAPipelineBecomesReady(t *testing.T) {
	notYet, _, _ := closeFixture(t, pipeline("k8s-observe", "p", false))
	ready, _, _ := closeFixture(t, pipeline("k8s-observe", "p", true))
	if a, b := notYet.Vocabulary(context.Background()).Revision,
		ready.Vocabulary(context.Background()).Revision; a == b {
		t.Fatalf("revision unchanged when a pipeline became addressable (%s)", a)
	}
}
