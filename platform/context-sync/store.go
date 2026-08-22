package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The durable store: generations, an atomic pointer, and incremental copies.
//
// INCREMENTAL IS LOAD-BEARING, not an optimisation. A conditional-but-FULL copy
// every two minutes would push the whole context over NFS on every change,
// increasing writes to the very filesystem this exists to protect. Unchanged
// files become HARDLINKS into the previous generation, so a checkpoint after one
// edited transcript writes exactly one file.
//
// Implemented in Go rather than by shelling out to `rsync --link-dest`. It is
// about fifty lines, and it keeps this module dependency-free and its image free
// of a tool that would otherwise have to be present and correct — which matters
// more here than saving the fifty lines, because every other module in this
// repo holds to the same rule.

const (
	// currentLink names the generation a restore should use. A symlink because
	// swapping one is a RENAME, which is atomic — a reader therefore never
	// observes a half-written context, only the old one or the new one.
	currentLink = "current"
	genPrefix   = "gen-"
	metaFile    = ".context-sync.json"
)

// GenerationMeta is recorded beside each copy.
type GenerationMeta struct {
	At    time.Time `json:"at"`
	Bytes int64     `json:"bytes"`
	Files int       `json:"files"`
	// Quiesced reports whether this copy was taken at a WORK BOUNDARY.
	//
	// A copy taken mid-run may hold a partially written file. It is still worth
	// taking — a long run is exactly what a crash would otherwise lose in full —
	// but a restore and a person both need to tell the two apart rather than
	// guess, which is the only reason this field exists.
	Quiesced bool `json:"quiesced"`
}

// Store is the durable context directory for ONE conversation.
type Store struct {
	// Root is the per-conversation directory on the context volume.
	Root string
	// Retain is how many generations to keep. Below 1 is treated as 1.
	Retain int
}

// Current returns the path of the generation a restore should use, or "" when
// the store holds none.
func (s *Store) Current() (string, error) {
	link := filepath.Join(s.Root, currentLink)
	target, err := os.Readlink(link)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(s.Root, target)
	}
	if _, err := os.Stat(target); err != nil {
		// A pointer to a generation that is gone is not an error worth failing
		// a run for — it is an empty store with a stale pointer, and treating
		// it as empty starts the conversation fresh rather than wedging it.
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return target, nil
}

// Meta reads a generation's metadata.
func (s *Store) Meta(gen string) (GenerationMeta, error) {
	var m GenerationMeta
	b, err := os.ReadFile(filepath.Join(gen, metaFile))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// Restore copies the current generation into dst.
//
// Copies rather than hardlinks: dst is the LIVE tree the agent writes to, and
// hardlinking into it would let the agent's first write mutate the durable copy
// it was restored from — silently destroying the snapshot that exists to
// survive exactly that.
func (s *Store) Restore(dst string) (GenerationMeta, bool, error) {
	gen, err := s.Current()
	if err != nil || gen == "" {
		return GenerationMeta{}, false, err
	}
	meta, err := s.Meta(gen)
	if err != nil && !os.IsNotExist(err) {
		return GenerationMeta{}, false, err
	}
	if err := copyTree(gen, dst); err != nil {
		return GenerationMeta{}, false, err
	}
	return meta, true, nil
}

// Checkpoint writes a new generation from the live tree, then swaps `current`
// onto it and prunes.
//
// The order matters: build fully, THEN swap. A crash mid-copy leaves an
// unreferenced partial directory and the previous generation still current,
// which is the failure mode worth having.
func (s *Store) Checkpoint(live string, m Manifest, quiesced bool, now time.Time) (GenerationMeta, error) {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return GenerationMeta{}, err
	}
	prev, err := s.Current()
	if err != nil {
		return GenerationMeta{}, err
	}
	next, err := s.nextGenDir()
	if err != nil {
		return GenerationMeta{}, err
	}
	if err := os.MkdirAll(next, 0o755); err != nil {
		return GenerationMeta{}, err
	}

	var written int64
	for _, e := range m.Entries {
		src := filepath.Join(live, filepath.FromSlash(e.Path))
		dst := filepath.Join(next, filepath.FromSlash(e.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return GenerationMeta{}, err
		}
		// Unchanged since the last generation? Hardlink it. This is what makes
		// a two-minute cadence affordable: an idle conversation's checkpoint
		// transfers nothing at all.
		if prev != "" && linkIfUnchanged(filepath.Join(prev, filepath.FromSlash(e.Path)), dst, e) {
			continue
		}
		n, err := copyFile(src, dst)
		if err != nil {
			if os.IsNotExist(err) {
				continue // vanished between scan and copy: not ours to mourn
			}
			return GenerationMeta{}, err
		}
		written += n
	}

	meta := GenerationMeta{At: now, Bytes: written, Files: len(m.Entries), Quiesced: quiesced}
	if err := writeMeta(next, meta); err != nil {
		return GenerationMeta{}, err
	}
	if err := s.swapCurrent(next); err != nil {
		return GenerationMeta{}, err
	}
	return meta, s.prune()
}

// swapCurrent atomically re-points `current` at gen.
//
// A symlink cannot be replaced in place, so it is created under a temporary
// name and RENAMED over the old one. Rename is the atomic primitive here: at
// every instant the link names a complete generation.
func (s *Store) swapCurrent(gen string) error {
	tmp := filepath.Join(s.Root, ".current.tmp")
	_ = os.Remove(tmp)
	if err := os.Symlink(filepath.Base(gen), tmp); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Root, currentLink))
}

// generations lists generation directories, oldest first.
func (s *Store) generations() ([]string, error) {
	ents, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var gens []string
	for _, e := range ents {
		if e.IsDir() && strings.HasPrefix(e.Name(), genPrefix) {
			gens = append(gens, e.Name())
		}
	}
	sort.Slice(gens, func(i, j int) bool { return genNum(gens[i]) < genNum(gens[j]) })
	return gens, nil
}

func genNum(name string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(name, genPrefix))
	return n
}

func (s *Store) nextGenDir() (string, error) {
	gens, err := s.generations()
	if err != nil {
		return "", err
	}
	next := 1
	if len(gens) > 0 {
		next = genNum(gens[len(gens)-1]) + 1
	}
	return filepath.Join(s.Root, fmt.Sprintf("%s%d", genPrefix, next)), nil
}

// prune removes the oldest generations beyond Retain, never the current one.
//
// More than one is kept deliberately: a mid-run copy may hold a torn file, and
// keeping its predecessors is what makes that cost a fallback rather than the
// context.
func (s *Store) prune() error {
	retain := s.Retain
	if retain < 1 {
		retain = 1
	}
	gens, err := s.generations()
	if err != nil {
		return err
	}
	cur, err := s.Current()
	if err != nil {
		return err
	}
	for i := 0; i < len(gens)-retain; i++ {
		p := filepath.Join(s.Root, gens[i])
		if p == cur {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

// linkIfUnchanged hardlinks prev->dst when prev matches the scanned entry.
func linkIfUnchanged(prev, dst string, e entry) bool {
	fi, err := os.Stat(prev)
	if err != nil || fi.Size() != e.Size || fi.ModTime().UnixNano() != e.MTime {
		return false
	}
	return os.Link(prev, dst) == nil
}

func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return 0, err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return 0, err
	}
	n, cerr := io.Copy(out, in)
	if closeErr := out.Close(); cerr == nil {
		cerr = closeErr
	}
	if cerr != nil {
		return n, cerr
	}
	// Preserve mtime: it is half the change-detection key, and a restored tree
	// whose times all read "now" would make the next scan report everything as
	// changed and copy the whole context again.
	return n, os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}

// copyTree copies a generation into dst, skipping the metadata file.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		if filepath.Base(p) == metaFile {
			return nil
		}
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		_, err = copyFile(p, target)
		return err
	})
}

func writeMeta(gen string, m GenerationMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(gen, metaFile), b, 0o644)
}
