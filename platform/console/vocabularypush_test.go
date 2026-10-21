package main

import (
	"context"
	"testing"
	"time"
)

// B6: ADA-6 requires a command-menu surface to learn of a vocabulary change
// from the long-poll's X-Agentops-Vocabulary-Revision header, never a timer.

// noteVocabularyRevision/SubscribeVocabulary in isolation: a CHANGE wakes a
// subscriber, the first observation and a repeat do not.
func TestNoteVocabularyRevisionWakesSubscribersOnlyOnChange(t *testing.T) {
	a := NewAdapter(NewManager("http://unused", "tok"), staticCache(), NewTranscripts(), "console")
	ch, cancel := a.SubscribeVocabulary()
	defer cancel()

	a.noteVocabularyRevision("rev-1") // first observation: no prior value to differ from
	select {
	case <-ch:
		t.Fatal("the first observed revision must not itself be a change")
	default:
	}

	a.noteVocabularyRevision("rev-1") // repeat: still no change
	select {
	case <-ch:
		t.Fatal("an unchanged revision must not notify")
	default:
	}

	a.noteVocabularyRevision("rev-2") // change
	select {
	case <-ch:
	default:
		t.Fatal("a changed revision must wake the subscriber")
	}
}

// End to end: a revision change observed on the ops long-poll reaches a
// subscriber through the adapter's own Run loop, not a polling timer.
func TestRunLoopNotifiesVocabularySubscriberOnARevisionChange(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	f.setVocabRevisions("rev-1", "rev-1", "rev-2")
	a, _, _ := consoleUnderTest(t, f)
	vocab, cancel := a.SubscribeVocabulary()
	defer cancel()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go a.Run(ctx)

	select {
	case <-vocab:
	case <-time.After(3 * time.Second):
		t.Fatal("never notified of the revision change observed on the long-poll")
	}
}
