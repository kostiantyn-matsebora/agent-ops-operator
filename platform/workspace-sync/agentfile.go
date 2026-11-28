package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// verifyAgentFile implements the requirement "the path of an agent file is
// verified before the runtime starts": it must exist, be a regular file,
// and resolve inside repoDir once every symlink — the leaf's own and any
// symlinked parent directory — is followed. Failing here with a named path
// is a clearer error than a runtime that quietly contributes nothing
// (design D7).
func verifyAgentFile(repoDir, relPath string) error {
	full := filepath.Join(repoDir, relPath)
	if _, err := os.Lstat(full); err != nil {
		return fmt.Errorf("does not exist: %w", err)
	}

	resolvedRepo, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		return fmt.Errorf("resolving repository directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	rel, err := filepath.Rel(resolvedRepo, resolved)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("resolves outside its repository")
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("is not a regular file")
	}
	return nil
}
