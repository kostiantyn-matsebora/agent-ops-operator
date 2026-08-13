package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Lister is the API half, injected so the tests can create a conversation
// mid-scan — the one race the ordering below exists to eliminate.
type Lister interface {
	ListConversations(ctx context.Context) ([]Conversation, error)
}

// Options bound and soften every run. The first run on an established install
// is the dangerous one, so neither of these is optional.
type Options struct {
	// DryRun reports what would be removed and removes nothing.
	DryRun bool
	// MaxDeletions bounds ONE run, so a first pass on an old install cannot
	// reclaim a thousand trees at once. Zero means unbounded, which the
	// configuration layer never sets.
	MaxDeletions int
	// SessionGrace must exceed the longest plausible run. See ReclaimSessions.
	SessionGrace time.Duration
	// Now is injectable for tests.
	Now func() time.Time
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Report is what one reclaimer did, for the log line that is this job's whole
// user interface.
type Report struct {
	Kind       string
	Scanned    int
	Removed    []string
	Skipped    int // withheld by the per-run bound
	Retained   int // referenced, or inside the grace period
	DryRun     bool
	BytesFreed int64
}

func (r Report) String() string {
	verb := "removed"
	if r.DryRun {
		verb = "would remove"
	}
	s := fmt.Sprintf("%s: scanned %d, %s %d, retained %d", r.Kind, r.Scanned, verb, len(r.Removed), r.Retained)
	if r.Skipped > 0 {
		s += fmt.Sprintf(", %d left for the next run (per-run bound)", r.Skipped)
	}
	if len(r.Removed) > 0 {
		s += ": " + strings.Join(r.Removed, ", ")
	}
	return s
}

// ReclaimWorkspaces removes <root>/<name> directories with no Conversation of
// that name.
//
// THE ORDER IS THE CORRECTNESS ARGUMENT, not a precaution. A workspace
// directory is created by the kubelet mounting a subPath for a runtime pod, and
// the reconciler creates that pod only for a Conversation that already exists —
// so THE CR ALWAYS PREDATES ITS DIRECTORY. Scanning first (T0) and listing
// second (T1 > T0) therefore means any directory visible at T0 had a CR before
// T0, and a listing at T1 sees it, UNLESS the conversation was deleted in
// between — which is precisely the case worth reclaiming.
//
// Reversing this is what breaks: listing first and scanning second lets a
// conversation created in between look like an orphan and lose its workspace on
// its first run.
func ReclaimWorkspaces(ctx context.Context, root string, lister Lister, opts Options) (Report, error) {
	rep := Report{Kind: "workspaces", DryRun: opts.DryRun}

	// T0 — the disk.
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return rep, nil // nothing mounted: nothing to reclaim
		}
		return rep, fmt.Errorf("scanning %s: %w", root, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	rep.Scanned = len(dirs)
	if len(dirs) == 0 {
		return rep, nil
	}

	// T1 — the API, strictly after. Phase-blind: a closed conversation has a
	// CR, so this listing protects its workspace with no extra rule.
	convs, err := lister.ListConversations(ctx)
	if err != nil {
		return rep, fmt.Errorf("listing conversations: %w", err)
	}
	live := make(map[string]bool, len(convs))
	for _, c := range convs {
		live[c.Name] = true
	}

	sort.Strings(dirs) // deterministic, so a bounded run makes progress in order
	for _, name := range dirs {
		if live[name] {
			rep.Retained++
			continue
		}
		if opts.MaxDeletions > 0 && len(rep.Removed) >= opts.MaxDeletions {
			rep.Skipped++
			continue
		}
		path := filepath.Join(root, name)
		if !opts.DryRun {
			if err := os.RemoveAll(path); err != nil {
				return rep, fmt.Errorf("removing %s: %w", path, err)
			}
		}
		rep.Removed = append(rep.Removed, name)
	}
	return rep, nil
}

// ReclaimSessions removes transcript files whose session id no longer appears
// in any conversation AND which are older than the grace period.
//
// BOTH conditions are required, and the ordering argument of ReclaimWorkspaces
// runs BACKWARDS here so it cannot be used: a conversation's context handle is
// written by POST /work/done, i.e. AFTER the transcript file already exists. A
// transcript for a run in flight is therefore unreferenced and perfectly alive,
// which is exactly what the grace period covers. It must exceed the longest
// plausible run — reclaiming early breaks resume for a live conversation, while
// keeping one too long costs disk.
//
// A CLOSED conversation still carries its context handle, so its transcripts
// are still referenced and still retained. Nothing extra is needed — but a
// sweep keyed on "recently active conversations" rather than ALL conversations
// would silently break every reopen, the same trap as the phase filter one
// directory over.
func ReclaimSessions(ctx context.Context, root string, lister Lister, opts Options) (Report, error) {
	rep := Report{Kind: "sessions", DryRun: opts.DryRun}

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return rep, nil
		}
		return rep, fmt.Errorf("scanning %s: %w", root, err)
	}
	type file struct {
		name    string
		session string
		modTime time.Time
		size    int64
	}
	var files []file
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, file{
			name:    e.Name(),
			session: strings.TrimSuffix(e.Name(), ".jsonl"),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}
	rep.Scanned = len(files)
	if len(files) == 0 {
		return rep, nil
	}

	convs, err := lister.ListConversations(ctx)
	if err != nil {
		return rep, fmt.Errorf("listing conversations: %w", err)
	}
	referenced := make(map[string]bool, len(convs))
	for _, c := range convs {
		if c.ContextID != "" {
			referenced[c.ContextID] = true
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	cutoff := opts.now().Add(-opts.SessionGrace)
	for _, f := range files {
		if referenced[f.session] || !f.modTime.Before(cutoff) {
			rep.Retained++
			continue
		}
		if opts.MaxDeletions > 0 && len(rep.Removed) >= opts.MaxDeletions {
			rep.Skipped++
			continue
		}
		if !opts.DryRun {
			if err := os.Remove(filepath.Join(root, f.name)); err != nil && !os.IsNotExist(err) {
				return rep, fmt.Errorf("removing %s: %w", f.name, err)
			}
		}
		rep.Removed = append(rep.Removed, f.name)
		rep.BytesFreed += f.size
	}
	return rep, nil
}
