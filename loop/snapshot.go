package loop

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The gate must see every write, whatever tool made it. A tool that knows its paths reports
// them (write, edit, patch); the shell, a custom tool or an MCP server does not. So a session
// stamps the workspace tree — size and mtime per file, bounded — at the start of a turn, again after
// every write-class step (the difference is that step's writes when it reported none), and once
// more at the turn's end (a write a read-classified command slipped in). What differs is a write.
//
// The workspace dir's own instruments and records are not the model's writes and are not stamped:
// instruments/, shared/ (the shared shelf: the work queue the harness moves), attic/ (old docs and
// the births ledger the harness keeps), tmp/, memory/
// (the tiers the binary maintains), toolbox/registry.json (derived),
// ontology/graph/unsaid.ttl (where a report's @U lines land) and log.md (the gate's own
// record). `.git` and the dependency/build dirs of heavyDirs are never walked.
//
// The tree is shared: a subagent working in the background beside its dispatcher changes it too,
// and a stamp cannot say whose hand moved a file. The receipt can — a tool that names its paths
// notes them in the engines' shared Receipts book with the file's stamp as the write left it,
// and a change another run noted since this session last stamped, the file still standing as
// that write left it, is that run's write, gated on its account, not this one's.

type stamp struct {
	size  int64
	mtime int64
}

// snapshot is the tree as last seen, keyed by root-relative slash path.
type snapshot map[string]stamp

// SnapshotCap bounds one walk; a tree past it is not watched, and the hole is named once.
const SnapshotCap = 200000

// heavyDirs are dependency and build dirs an agent never authors: hundreds of thousands of
// lstats per stamp, zero gate signal. Skipped wherever they sit in the tree (a write inside
// one through a reporting tool still reaches the gate by its paths).
var heavyDirs = map[string]bool{
	"node_modules": true, ".venv": true, "venv": true, "__pycache__": true,
	"target": true, "dist": true, ".next": true, ".turbo": true, ".cache": true,
}

func (e *Engine) unwatched(rel string) bool {
	// git's own store, and dependency trees a package manager rewrites wholesale: walking them
	// after every write-class step costs seconds and tells the gate nothing
	for _, d := range []string{".git", "node_modules", ".venv", "__pycache__"} {
		if rel == d || strings.HasSuffix(rel, "/"+d) {
			return true
		}
	}
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	if heavyDirs[base] {
		return true
	}
	wd := e.Lexicon.workspaceDir()
	for _, p := range []string{"instruments", "shared", "attic", "tmp", "memory", "toolbox/registry.json", "ontology/graph/unsaid.ttl", "log.md"} {
		if rel == wd+"/"+p {
			return true
		}
	}
	return false
}

// stampTree walks the root; ok is false (with why) past the cap or when the root is unreadable.
func (e *Engine) stampTree() (snap snapshot, why string) {
	snap = snapshot{}
	root := e.Root
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // an unreadable entry is not a write
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if e.unwatched(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if n++; n > SnapshotCap {
			return errTreeTooLarge
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		snap[rel] = stamp{size: info.Size(), mtime: info.ModTime().UnixNano()}
		return nil
	})
	switch {
	case err == errTreeTooLarge:
		return nil, fmt.Sprintf("the workspace tree has more than %d files — writes through the shell are not watched this session; only tools that report their paths reach the gate", SnapshotCap)
	case err != nil:
		return nil, "the workspace root cannot be walked (" + err.Error() + ") — writes through the shell are not watched"
	}
	return snap, ""
}

var errTreeTooLarge = fmt.Errorf("tree past the snapshot cap")

// diff lists what changed from a to b: new, changed or removed files, sorted.
func (a snapshot) diff(b snapshot) []string {
	var out []string
	for p, sb := range b {
		if sa, ok := a[p]; !ok || sa != sb {
			out = append(out, p)
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Receipts is the write book of the engines sharing a root: which run's tool last reported
// writing each root-relative path, when, and the file as that write left it. It keeps only what
// a watching turn could still need: a receipt explains a change only to a turn whose baseline
// is older than it, so one noted before every live baseline (or with no turn watching — the
// next baseline is newer still) leaves the book.
type Receipts struct {
	mu   sync.Mutex
	by   map[string]receipt
	live map[*Session]time.Time // each watching turn's baseline (stampedAt)
}

type receipt struct {
	run  string
	at   time.Time
	left stamp // the file's stamp right after the write
	gone bool  // the write left no file (a delete)
}

// NewReceipts is an empty book.
func NewReceipts() *Receipts {
	return &Receipts{by: map[string]receipt{}, live: map[*Session]time.Time{}}
}

// watch books s's baseline at at — now, read under the book's lock, when at is zero, so no
// receipt noted after it can be dropped before s is booked — and answers it.
func (b *Receipts) watch(s *Session, at time.Time) time.Time {
	if b == nil {
		if at.IsZero() {
			at = time.Now()
		}
		return at
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if at.IsZero() {
		at = time.Now()
	}
	b.live[s] = at
	b.prune()
	return at
}

// unwatch ends s's turn: it asks the book nothing more until its next baseline.
func (b *Receipts) unwatch(s *Session) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.live, s)
	b.prune()
}

// prune drops every receipt no live baseline is older than (under b.mu).
func (b *Receipts) prune() {
	var oldest time.Time
	for _, at := range b.live {
		if oldest.IsZero() || at.Before(oldest) {
			oldest = at
		}
	}
	for p, r := range b.by {
		if oldest.IsZero() || !r.at.After(oldest) {
			delete(b.by, p)
		}
	}
}

// note books run's write of rel (root-relative) under root, stamping the file as it now stands.
func (b *Receipts) note(run, root, rel string) {
	if b == nil {
		return
	}
	r := receipt{run: run, at: time.Now(), gone: true}
	if info, err := os.Lstat(filepath.Join(root, rel)); err == nil {
		r.left, r.gone = stamp{size: info.Size(), mtime: info.ModTime().UnixNano()}, false
	}
	b.mu.Lock()
	b.by[filepath.ToSlash(rel)] = r
	b.prune()
	b.mu.Unlock()
}

// foreign is true when another run noted writing rel after since and the file still stands as
// that write left it (cur, present): the change is its receipt's. A receipt older than since was
// already in the watcher's baseline; a file moved again after its receipt was moved by another
// hand — the watcher's own, if nobody else holds a receipt for it.
func (b *Receipts) foreign(run, rel string, since time.Time, cur stamp, present bool) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.by[rel]
	return ok && r.run != run && r.at.After(since) && r.gone == !present && (!present || r.left == cur)
}

// watch takes the turn's baseline; a failed stamp names the hole once and stops watching.
func (s *Session) watch(r *Result) {
	if s.nowatch {
		return
	}
	s.stampedAt = s.Engine.Receipts.watch(s, time.Time{})
	snap, why := s.Engine.stampTree()
	if why != "" {
		s.nowatch = true
		s.base = nil
		s.Engine.Receipts.unwatch(s)
		r.hole(why)
		s.Journal.Log(Event{"t": "watch", "off": why})
		return
	}
	s.base = snap
	s.TestsBefore = s.Engine.readTests(snap)
}

// seen re-stamps the tree and returns what changed since the baseline, root-relative, less what
// another run holds a fresh receipt for; the new stamp becomes the baseline. Nothing when the
// session is not watching.
func (s *Session) seen() []string {
	if s.nowatch || s.base == nil {
		return nil
	}
	now := time.Now()
	snap, why := s.Engine.stampTree()
	if why != "" {
		s.nowatch = true
		s.base = nil
		s.Engine.Receipts.unwatch(s)
		return nil
	}
	var changed []string
	for _, p := range s.base.diff(snap) {
		cur, present := snap[p]
		if !s.Engine.Receipts.foreign(s.RunID, p, s.stampedAt, cur, present) {
			changed = append(changed, p)
		}
	}
	s.base, s.stampedAt = snap, s.Engine.Receipts.watch(s, now) // the old baseline was booked until the diff was read
	return changed
}

// testFile names a test file in the languages the gate reads: Go, JS/TS, Python.
var testFile = regexp.MustCompile(`(_test\.go|\.(test|spec)\.[cm]?[jt]sx?|(^|/)test_[^/]*\.py|_test\.py)$`)

// IsTestFile reports whether a root-relative path is a test file.
func IsTestFile(rel string) bool { return testFile.MatchString(rel) }

// readTests keeps the test files' text as the turn opens, so the gate can see what a turn did to
// the tests it is judged by (bounded: 256 KiB a file, 4 MiB in all).
func (e *Engine) readTests(snap snapshot) map[string]string {
	out := map[string]string{}
	total := 0
	for rel := range snap {
		if !testFile.MatchString(rel) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(rel)))
		if err != nil || len(b) > 256<<10 || total+len(b) > 4<<20 {
			continue
		}
		total += len(b)
		out[rel] = string(b)
	}
	return out
}

// Stamps is the write watch for a host that runs the loop itself (a Claude Code hook): the tree
// as "size:mtime" per root-relative path, the same files a session stamps, or why it cannot be
// read. Changed compares two readings.
func (e *Engine) Stamps() (map[string]string, string) {
	snap, why := e.stampTree()
	if why != "" {
		return nil, why
	}
	out := make(map[string]string, len(snap))
	for p, s := range snap {
		out[p] = fmt.Sprintf("%d:%d", s.size, s.mtime)
	}
	return out, ""
}

// Changed lists what differs from a to b: new, changed or removed paths, sorted.
func Changed(a, b map[string]string) []string {
	var out []string
	for p, s := range b {
		if a[p] != s {
			out = append(out, p)
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
