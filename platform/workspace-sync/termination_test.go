package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactorRemovesEveryKnownSecret(t *testing.T) {
	red := newRedactor(map[int]string{0: "tok-abc", 1: "key-xyz"})
	got := red.redact("clone failed with tok-abc and also key-xyz in the log")
	if strings.Contains(got, "tok-abc") || strings.Contains(got, "key-xyz") {
		t.Fatalf("secret survived redaction: %q", got)
	}
}

func TestRedactorOnNilCredsIsANoOp(t *testing.T) {
	red := newRedactor(nil)
	if got := red.redact("nothing secret here"); got != "nothing secret here" {
		t.Fatalf("got %q", got)
	}
}

// TestFailedCloneNeverLeaksTheTokenToTheTerminationMessage is task 3.6's
// required test: a fake token is planted in the environment, a clone is
// forced to fail, and the token must appear in neither the termination
// message nor the error this process would otherwise log.
func TestFailedCloneNeverLeaksTheTokenToTheTerminationMessage(t *testing.T) {
	const token = "super-secret-planted-token"
	workspace := t.TempDir()
	// A URL that cannot possibly succeed: no such local path, so the
	// failure is immediate and needs no network.
	badURL := fileURL(t, filepath.Join(t.TempDir(), "does-not-exist"))
	repos := []RepoEntry{{Name: "app", URL: badURL, Ref: "main", Auth: RepoAuth{Type: "https"}}}
	creds := map[int]string{0: token}
	red := newRedactor(creds)

	err := syncAll(context.Background(), workspace, repos, nil, creds)
	if err == nil {
		t.Fatal("expected the clone to fail")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("the raw error must not be trusted un-redacted, got %q", err.Error())
	}

	msg := red.redact(err.Error())
	if strings.Contains(msg, token) {
		t.Fatalf("the redacted message still contains the token: %q", msg)
	}
	if !strings.Contains(msg, "app") {
		t.Fatalf("the message should name the repository, got %q", msg)
	}

	logPath := filepath.Join(t.TempDir(), "termination-log")
	t.Setenv(terminationLogPathEnv, logPath)
	writeTerminationMessage(msg)
	written, rerr := os.ReadFile(logPath)
	if rerr != nil {
		t.Fatalf("reading termination log: %v", rerr)
	}
	if strings.Contains(string(written), token) {
		t.Fatalf("the termination message file contains the token: %q", written)
	}
}

func TestWriteTerminationMessageTruncatesOversizeMessages(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "termination-log")
	t.Setenv(terminationLogPathEnv, logPath)
	writeTerminationMessage(strings.Repeat("x", maxTerminationMessage+500))
	written := mustReadFile(t, logPath)
	if len(written) != maxTerminationMessage {
		t.Fatalf("got %d bytes, want %d", len(written), maxTerminationMessage)
	}
}
