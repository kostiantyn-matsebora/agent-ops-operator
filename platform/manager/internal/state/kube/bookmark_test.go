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
func TestTranslateObjectForwardsABookmarkAsAResumePoint(t *testing.T) {
	read := ConversationLayout().Read
	conv := &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "42"}}
	evs := translateObject(watch.Event{Type: watch.Bookmark, Object: conv}, read)
	if len(evs) != 1 || evs[0].Type != state.EventBookmark || evs[0].Resume != "42" || evs[0].Key != "" {
		t.Fatalf("got %+v", evs)
	}
	if evs := translateObject(watch.Event{Type: watch.Bookmark, Object: &agentopsv1alpha1.Conversation{}}, read); len(evs) != 0 {
		t.Fatal("a bookmark with no resourceVersion carries nothing to resume from")
	}
}
