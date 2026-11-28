package main

import (
	"strings"
	"testing"
)

func TestParseReposEmpty(t *testing.T) {
	repos, err := parseRepos("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repos != nil {
		t.Fatalf("expected nil, got %v", repos)
	}
}

func TestParseReposValid(t *testing.T) {
	repos, err := parseRepos(`[
		{"name":"app","url":"https://example.invalid/app.git","ref":"main","auth":{"type":"https"}},
		{"name":"infra","url":"git@example.invalid:infra.git","ref":"main","auth":{"type":"ssh"},"keepGit":false}
	]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if !repos[0].keepGit() {
		t.Error("app should default keepGit to true")
	}
	if repos[1].keepGit() {
		t.Error("infra declared keepGit:false")
	}
}

func TestParseReposRejectsBadJSON(t *testing.T) {
	if _, err := parseRepos("not json"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseReposRejectsUnsafeName(t *testing.T) {
	cases := []string{
		`[{"name":"","url":"https://x.invalid/a.git"}]`,
		`[{"name":"../escape","url":"https://x.invalid/a.git"}]`,
		`[{"name":"a/b","url":"https://x.invalid/a.git"}]`,
		`[{"name":".hidden","url":"https://x.invalid/a.git"}]`,
	}
	for _, c := range cases {
		if _, err := parseRepos(c); err == nil {
			t.Errorf("expected an error for %s", c)
		}
	}
}

func TestParseReposRejectsDuplicateNames(t *testing.T) {
	_, err := parseRepos(`[{"name":"app","url":"https://x.invalid/a.git"},{"name":"app","url":"https://x.invalid/b.git"}]`)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected a duplicate-name error, got %v", err)
	}
}

func TestParseReposRejectsMissingURL(t *testing.T) {
	if _, err := parseRepos(`[{"name":"app"}]`); err == nil {
		t.Fatal("expected an error for a missing url")
	}
}

func TestParseReposRejectsUnknownAuthType(t *testing.T) {
	if _, err := parseRepos(`[{"name":"app","url":"https://x.invalid/a.git","auth":{"type":"token"}}]`); err == nil {
		t.Fatal("expected an error for an unknown auth.type")
	}
}

func TestParseAgentFileEmpty(t *testing.T) {
	ref, err := parseAgentFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != nil {
		t.Fatalf("expected nil, got %v", ref)
	}
}

func TestParseAgentFileValid(t *testing.T) {
	ref, err := parseAgentFile(`{"repo":"app","path":"agents/main.md"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Repo != "app" || ref.Path != "agents/main.md" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestParseAgentFileRejectsAbsolutePath(t *testing.T) {
	if _, err := parseAgentFile(`{"path":"/etc/passwd"}`); err == nil {
		t.Fatal("expected an error for an absolute path")
	}
}

func TestParseAgentFileRejectsDotDot(t *testing.T) {
	if _, err := parseAgentFile(`{"path":"../escape.md"}`); err == nil {
		t.Fatal("expected an error for a path containing \"..\"")
	}
}

func TestParseAgentFileRejectsMissingPath(t *testing.T) {
	if _, err := parseAgentFile(`{"repo":"app"}`); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}
