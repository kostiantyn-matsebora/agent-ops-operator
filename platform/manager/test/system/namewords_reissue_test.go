//go:build system

package system

// conversation-naming (ADDED requirement "The name is display, and
// allocation hands back the created record") — the exact scenario it names:
// "A freed suffix is reissued under a new identity". identity_test.go
// already proves the trivial one-record case (a BASE name, suffix 1,
// reissued after the only life sharing it is deleted). What that lane
// cannot show is namewords.nextSuffix actually tracking the HIGHEST live
// suffix among several: this one opens three conversations sharing one
// word-chain (suffixes 1, 2, 3), deletes only the highest, and checks that
// a fourth allocation reissues exactly that slot — never collapsing onto a
// lower one still live, and never skipping past it to -4.
import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func TestFreedHighestSuffixIsReissuedUnderANewIdentity(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	fullStamp := fmt.Sprint(time.Now().UnixNano())
	// A SHORT 6-digit stamp for the title: namewords.Words keeps only the
	// first three non-stopword tokens AND drops whole trailing words past
	// MaxWordChainLength (24) — "suffix", "reissue" and a full UnixNano
	// stamp together overflow that bound, which would silently drop the
	// stamp itself and defeat the one thing making this NameBase unique per
	// run (the same short-stamp idiom coordinator_test.go's moveLane uses
	// for the same reason). ids stay on the FULL stamp: they are never
	// passed through namewords at all.
	stamp := fullStamp[len(fullStamp)-6:]
	title := "suffix-reissue-" + stamp

	var liveNames []string
	t.Cleanup(func() {
		for _, n := range liveNames {
			_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: n}})
		}
	})

	// The first signal under this fresh title allocates the BARE base name —
	// base is read back from it rather than assumed, since the stored name
	// also carries namewords' own "<kind>-" prefix (here "task-"), which this
	// test has no reason to hard-code.
	id0 := fmt.Sprintf("e2e-suffix-reissue-%d-%s", 0, fullStamp)
	e.PostSignal(t, SourceTasks, map[string]any{
		"initiationId": id0, "kind": "task", "payload": "echo suffix-reissue", "title": title,
	})
	first := e.ConversationFor(t, id0, 2*time.Minute)
	liveNames = append(liveNames, first.Name)
	base := first.Name

	byName := map[string]*agentopsv1alpha1.Conversation{base: first}
	for i := 1; i < 3; i++ {
		id := fmt.Sprintf("e2e-suffix-reissue-%d-%s", i, fullStamp)
		e.PostSignal(t, SourceTasks, map[string]any{
			"initiationId": id, "kind": "task", "payload": "echo suffix-reissue", "title": title,
		})
		conv := e.ConversationFor(t, id, 2*time.Minute)
		liveNames = append(liveNames, conv.Name)
		byName[conv.Name] = conv
	}

	wantNames := []string{base, base + "-2", base + "-3"}
	for _, n := range wantNames {
		if _, ok := byName[n]; !ok {
			t.Fatalf("three signals sharing one word-chain must allocate %v in order, got %v", wantNames, keysOf(byName))
		}
	}

	highest := byName[base+"-3"]

	// Delete ONLY the highest suffix, outright — never /close, since the
	// store key is the Name and this is the only way to free that exact slot
	// while -1 and -2 stay live, which is what makes the next assertion mean
	// "the HIGHEST live suffix", not merely "any freed slot".
	if err := e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: highest.Name}}); err != nil {
		t.Fatalf("deleting the highest-suffixed life %s: %v", highest.Name, err)
	}
	waitFor(t, "the deleted highest-suffix life to be gone", 2*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, highest.Name)
		return err != nil, nil
	})

	idNext := "e2e-suffix-reissue-next-" + fullStamp
	e.PostSignal(t, SourceTasks, map[string]any{
		"initiationId": idNext, "kind": "task", "payload": "echo suffix-reissue-next", "title": title,
	})
	reissued := e.ConversationFor(t, idNext, 2*time.Minute)
	liveNames = append(liveNames, reissued.Name)

	if reissued.Name != highest.Name {
		t.Fatalf("a freed HIGHEST suffix must be reissued under the same name, got %q want %q", reissued.Name, highest.Name)
	}
	if reissued.UID == "" || reissued.UID == highest.UID {
		t.Fatalf("the reissued name must carry its OWN identity, distinct from the deleted life's %q, got %q", highest.UID, reissued.UID)
	}

	// -1 and -2 were never touched: still exactly themselves, under their own
	// original identities, proving nextSuffix read them as still occupying
	// their slots rather than collapsing the base down after the delete.
	for _, n := range []string{base, base + "-2"} {
		cur, err := e.K.Conversation(ctx, n)
		if err != nil {
			t.Fatalf("the untouched life %s must still exist: %v", n, err)
		}
		if cur.UID != byName[n].UID {
			t.Fatalf("the untouched life %s must keep its own identity, got %q want %q", n, cur.UID, byName[n].UID)
		}
	}
}

func keysOf(m map[string]*agentopsv1alpha1.Conversation) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
