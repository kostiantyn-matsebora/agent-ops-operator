// Package main is the context-sync sidecar: it keeps a runtime's LIVE context on
// pod-local storage and a durable snapshot on the context volume.
//
// WHY A SIDECAR AT ALL. On 2026-08-20 a node reboot corrupted the ext4
// filesystem on the shared home volume. The live context WAS that volume, so
// one damaged filesystem took every conversation's context with it and blocked
// every runtime pod from starting. Moving the live copy to pod-local storage
// makes the durable volume a SNAPSHOT: a run already going keeps working when
// the volume goes bad underneath it, and a corrupt snapshot costs continuity
// rather than availability.
//
// WHY IT PROXIES THE WORK CONTRACT. It needs to know when a work unit starts and
// finishes, and the runtime already tells the manager exactly that. Sitting
// between them means no runtime image has to change — including images adopters
// bring themselves, which is the whole reason this is not a library.
//
// NO DEPENDENCIES outside this module, like every adapter here.
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// entry is one file's identity for change detection.
type entry struct {
	Path  string
	Size  int64
	MTime int64 // UnixNano
}

// Manifest is a cheap fingerprint of the live context tree.
//
// (path, size, mtime) rather than content hashes: the live copy is pod-local
// storage, so a stat walk of a few thousand files is milliseconds, while hashing
// reads every byte to answer a question a stat already answers. This is the same
// quick-check rsync makes.
type Manifest struct {
	Entries []entry
}

// Scan walks the include globs under root, applying the excludes, and returns a
// manifest.
//
// Include globs rather than an exclude list: caches, tool state and telemetry
// are then excluded BY CONSTRUCTION rather than by a list that has to chase
// every file a vendor adds. Excludes exist only for churn INSIDE the included
// tree — lock files and temp files, which otherwise report a change on nearly
// every cycle and defeat the skip-when-unchanged rule entirely.
func Scan(root string, includes, excludes []string) (Manifest, error) {
	seen := map[string]entry{}
	for _, pattern := range includes {
		if err := walkGlob(root, pattern, excludes, seen); err != nil {
			return Manifest{}, err
		}
	}
	m := Manifest{Entries: make([]entry, 0, len(seen))}
	for _, e := range seen {
		m.Entries = append(m.Entries, e)
	}
	// Stable order: a manifest that reshuffles would compare unequal to an
	// identical tree and force a pointless copy every cycle.
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	return m, nil
}

// literalPrefix returns the leading path components of a glob that contain no
// wildcard, so a walk can start there instead of at the root.
//
// It is what keeps `.claude/projects/-data-workspace/**` from walking a home
// directory full of package caches just to find two transcripts.
func literalPrefix(pattern string) string {
	parts := strings.Split(pattern, "/")
	var lit []string
	for _, p := range parts {
		if strings.ContainsAny(p, "*?[") {
			break
		}
		lit = append(lit, p)
	}
	return strings.Join(lit, "/")
}

// walkGlob expands one include pattern under root.
//
// Walk from the literal prefix, then filter with matchGlob. ONE rule rather than
// a special case per pattern shape: `**`, `**/*`, `x/**` and a plain relative
// path all go through the same path, which is why `**/*` silently matching
// nothing cannot happen again.
//
// `**` is supported because a real context path needs it — claude-code files
// transcripts several directories deep — and Go's filepath.Match does not
// implement it.
func walkGlob(root, pattern string, excludes []string, out map[string]entry) error {
	start := root
	if lit := literalPrefix(pattern); lit != "" {
		start = filepath.Join(root, filepath.FromSlash(lit))
	}

	return filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// A missing include is not an error: a runtime that has not yet
			// written any context is the normal state on a first start, and
			// failing here would make every fresh conversation look broken.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if !matchGlob(pattern, rel) {
			return nil
		}
		if excluded(rel, excludes) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			if os.IsNotExist(ierr) {
				return nil // vanished mid-walk: it is not context we can copy
			}
			return ierr
		}
		out[rel] = entry{Path: rel, Size: info.Size(), MTime: info.ModTime().UnixNano()}
		return nil
	})
}

// excluded reports whether a relative path matches any exclude glob.
func excluded(rel string, excludes []string) bool {
	for _, ex := range excludes {
		if matchGlob(ex, rel) {
			return true
		}
	}
	return false
}

// matchGlob matches a slash-separated path against a glob supporting `**`.
func matchGlob(pattern, name string) bool {
	if pattern == "**" || pattern == "**/*" {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		// `**/x` matches x at any depth, including the top level.
		tail := strings.TrimPrefix(pattern, "**/")
		if matchGlob(tail, name) {
			return true
		}
		for i := 0; i < len(name); i++ {
			if name[i] == '/' && matchGlob(tail, name[i+1:]) {
				return true
			}
		}
		return false
	}
	if base, ok := strings.CutSuffix(pattern, "/**"); ok {
		return name == base || strings.HasPrefix(name, base+"/")
	}
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

// Changed reports whether the tree differs from a previous manifest.
//
// This is the whole of "smart": when it returns false the checkpoint is skipped
// and NOTHING is written to the durable volume. On a two-minute timer over a
// conversation that is sitting idle, that is the difference between touching the
// fragile filesystem 720 times a day and not touching it at all.
func (m Manifest) Changed(prev Manifest) bool {
	if len(m.Entries) != len(prev.Entries) {
		return true
	}
	for i := range m.Entries {
		if m.Entries[i] != prev.Entries[i] {
			return true
		}
	}
	return false
}

// Bytes totals the manifest, for reporting how large a checkpoint was.
func (m Manifest) Bytes() int64 {
	var n int64
	for _, e := range m.Entries {
		n += e.Size
	}
	return n
}
