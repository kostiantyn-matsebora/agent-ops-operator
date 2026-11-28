package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// repoCredPrefix names every env var this process reads a credential VALUE
// from. Nothing else here is sensitive.
const repoCredPrefix = "REPO_CRED_"

// collectCredentials reads every REPO_CRED_<n> the environment carries,
// keyed by the zero-based position design D5 fixes. Absent entries are
// simply not credentialed repositories.
func collectCredentials() map[int]string {
	creds := map[int]string{}
	for _, kv := range os.Environ() {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, repoCredPrefix) {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimPrefix(name, repoCredPrefix))
		if err != nil {
			continue
		}
		creds[idx] = value
	}
	return creds
}

// credentialFiles is what gitEnvFor hands back: the env additions a clone
// must run under, and a cleanup the caller MUST run once that clone is
// done — every credential file this writes lives outside the workspace, in
// its own throwaway directory, and nothing should outlive the clone it was
// written for.
type credentialFiles struct {
	env     []string
	cleanup func()
}

// gitEnvFor builds the environment one repository's clone must run with,
// per design D5: the credential reaches git through GIT_ASKPASS (https) or
// GIT_SSH_COMMAND (ssh), never through the URL and never into a file under
// the workspace. GIT_TERMINAL_PROMPT=0 is set unconditionally so a missing
// or wrong credential fails the command instead of hanging on a prompt this
// container has no terminal to answer.
func gitEnvFor(idx int, repo RepoEntry, creds map[int]string) (credentialFiles, error) {
	base := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	switch repo.Auth.Type {
	case "":
		return credentialFiles{env: base, cleanup: func() {}}, nil
	case "https":
		token, ok := creds[idx]
		if !ok || token == "" {
			return credentialFiles{}, fmt.Errorf("auth.type %q declared but %s is not set", repo.Auth.Type, repoCredEnv(idx))
		}
		path, cleanup, err := writeAskpassScript(token)
		if err != nil {
			return credentialFiles{}, err
		}
		return credentialFiles{env: append(base, "GIT_ASKPASS="+path), cleanup: cleanup}, nil
	case "ssh":
		key, ok := creds[idx]
		if !ok || key == "" {
			return credentialFiles{}, fmt.Errorf("auth.type %q declared but %s is not set", repo.Auth.Type, repoCredEnv(idx))
		}
		path, cleanup, err := writeSSHKeyFile(key)
		if err != nil {
			return credentialFiles{}, err
		}
		// IdentitiesOnly=yes per design D5: only the key this process just
		// wrote is offered, never whatever the ambient ssh-agent holds.
		// UserKnownHostsFile=/dev/null + accept-new: a first connection to a
		// host this pod has never seen must not hang waiting on a prompt
		// nobody can answer.
		sshCmd := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null -o BatchMode=yes", path)
		return credentialFiles{env: append(base, "GIT_SSH_COMMAND="+sshCmd), cleanup: cleanup}, nil
	default:
		return credentialFiles{}, fmt.Errorf("unknown auth.type %q", repo.Auth.Type)
	}
}

// writeAskpassScript writes a GIT_ASKPASS helper answering git's two
// prompts — username, then password — without ever putting the token on a
// command line (so it never shows in `ps`) or in a file under the
// workspace. The directory is 0700 and the script 0700, mode-restricted to
// this process's own user, same posture as the ssh key file below.
func writeAskpassScript(token string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "wsync-cred-")
	if err != nil {
		return "", nil, fmt.Errorf("creating credential directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "askpass.sh")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  Username*) printf '%s' 'x-access-token' ;;\n" +
		"  *) printf '%s' '" + shellSingleQuote(token) + "' ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("writing askpass script: %w", err)
	}
	return path, cleanup, nil
}

// writeSSHKeyFile writes the ssh key to a mode-0600 file outside the
// workspace, per spec.md's credential table.
func writeSSHKeyFile(key string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "wsync-cred-")
	if err != nil {
		return "", nil, fmt.Errorf("creating credential directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	content := key
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	path := filepath.Join(dir, "id")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("writing ssh key: %w", err)
	}
	return path, cleanup, nil
}

// shellSingleQuote escapes a value for safe embedding between single quotes
// in the askpass script above.
func shellSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}
