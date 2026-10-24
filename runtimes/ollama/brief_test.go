package main

import (
	"strings"
	"testing"
)

func TestExtractBriefPullsAStandaloneTagOutAndRemovesItFromTheText(t *testing.T) {
	rest, brief := extractBrief("the disk is fine\n\n<brief>\nthe api pod restart loop\n</brief>\n\nnothing else to do")
	if brief != "the api pod restart loop" {
		t.Fatalf("brief: %q", brief)
	}
	if strings.Contains(rest, "<brief>") {
		t.Fatalf("the tag must not reach Result: %q", rest)
	}
	if rest != "the disk is fine\n\nnothing else to do" {
		t.Fatalf("rest: %q", rest)
	}
}

func TestExtractBriefJoinsAMultiLineBriefIntoOneLine(t *testing.T) {
	_, brief := extractBrief("<brief>\nthe api pod,\nrestarting every few minutes\n</brief>")
	if brief != "the api pod, restarting every few minutes" {
		t.Fatalf("brief: %q", brief)
	}
}

func TestExtractBriefReportsEmptyWhenNoTagIsPresent(t *testing.T) {
	rest, brief := extractBrief("just an ordinary answer")
	if brief != "" {
		t.Fatalf("brief: %q", brief)
	}
	if rest != "just an ordinary answer" {
		t.Fatalf("rest: %q", rest)
	}
}

func TestExtractBriefIgnoresAMentionThatIsNotAStandaloneTag(t *testing.T) {
	rest, brief := extractBrief("say `<brief>` on its own line to set one")
	if brief != "" {
		t.Fatalf("brief: %q", brief)
	}
	if rest != "say `<brief>` on its own line to set one" {
		t.Fatalf("rest: %q", rest)
	}
}

func TestExtractBriefBoundsTheReportedBriefToMaxBrief(t *testing.T) {
	long := strings.Repeat("x", 600)
	_, brief := extractBrief("<brief>\n" + long + "\n</brief>")
	if len(brief) != MaxBrief {
		t.Fatalf("brief length: %d", len(brief))
	}
}
