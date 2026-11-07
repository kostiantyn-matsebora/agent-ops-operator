package kube

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// A bookmark carries only a resourceVersion; it becomes an EventBookmark whose
// Resume is that version, so an idle feed's resume point keeps moving.
func TestTranslateWatchEventForwardsABookmarkAsAResumePoint(t *testing.T) {
	conv := &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "42"}}
	ev, ok := translateWatchEvent(watch.Event{Type: watch.Bookmark, Object: conv})
	if !ok || ev.Type != state.EventBookmark || ev.Resume != "42" || ev.Key != "" {
		t.Fatalf("got %+v, %v", ev, ok)
	}
	if _, ok := translateWatchEvent(watch.Event{Type: watch.Bookmark, Object: &agentopsv1alpha1.Conversation{}}); ok {
		t.Fatal("a bookmark with no resourceVersion carries nothing to resume from")
	}
}
