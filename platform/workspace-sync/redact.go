package main

import (
	"sort"
	"strings"
)

// redactor removes every known credential value from a message before it is
// logged or written to the termination message — design D5 and
// spec.md's "A message names no secret" scenario. It is built once, from
// every REPO_CRED_<n> the environment carries, so a failure anywhere in this
// process's lifetime is scrubbed the same way.
type redactor struct {
	secrets []string
}

const redactedPlaceholder = "[redacted]"

// newRedactor builds a redactor from every known credential value. Sorted
// longest-first so a short credential that happens to be a substring of a
// longer one never leaves a partial match behind.
func newRedactor(creds map[int]string) *redactor {
	seen := make(map[string]bool, len(creds))
	var secrets []string
	for _, v := range creds {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		secrets = append(secrets, v)
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return &redactor{secrets: secrets}
}

// redact replaces every occurrence of every known secret in s.
func (r *redactor) redact(s string) string {
	if r == nil {
		return s
	}
	for _, v := range r.secrets {
		s = strings.ReplaceAll(s, v, redactedPlaceholder)
	}
	return s
}
