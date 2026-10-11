// tree.go renders the on-disk workspace into the tree entries the board dash
// fetches lazily: one directory level per call (slice 1), flat entries with
// a has_children flag on dirs so the client expands on click instead of
// swallowing the whole tree.
package board

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TreeEntry is one node of the workspace tree: a file or directory. Slice 1:
// one dir level per call — Children are never filled; dirs carry
// HasChildren so the client knows whether to draw the expand arrow.
// The JSON matches what the board dash renders.
type TreeEntry struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Size        int64  `json:"size,omitempty"`
	Mod         string `json:"mod,omitempty"`
	HasChildren bool   `json:"has_children"`
}

// skipNames are noise entries the tree hides anywhere in the workspace.
var skipNames = map[string]bool{".git": true, ".venv": true, ".next": true}

// treeCap bounds the entries of one level so one giant directory cannot
// flood the page (prefix == "" caps the root walk).
const treeCap = 500

// WalkTree renders one directory level of root: prefix picks the
// directory ("" = the root, else a root-relative slash path contained
// under root — anything escaping returns not-found). Entries are that
// level only: directories first in the listing (each name-less-than the
// first file), files name-sorted, mod as the RFC3339 instant to the
// second, directories report has_children, truncated flips at cap.
// Not-found or not-a-dir roots return nil, false, false.
func WalkTree(root string, prefix string) (entries []TreeEntry, truncated bool, found bool) {
	dir := root
	rel := ""
	if prefix != "" {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			return nil, false, false
		}
		abs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(prefix)))
		if err != nil {
			return nil, false, false
		}
		if abs != rootAbs && !strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
			return nil, false, false
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return nil, false, false
		}
		dir = abs
		rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(prefix)))
	}
	if _, err := os.ReadDir(dir); err != nil {
		return nil, false, false
	}
	count := 0
	var level func(dir string, rel string) []TreeEntry
	level = func(dir string, rel string) []TreeEntry {
		raw, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var dirs, files []TreeEntry
		for _, e := range raw {
			name := e.Name()
			if skipNames[name] {
				continue
			}
			if strings.HasPrefix(name, "._") || strings.HasPrefix(name, ".~") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			childRel := name
			if rel != "" {
				childRel = rel + "/" + name
			}
			te := TreeEntry{
				Path: childRel,
				Name: name,
				Mod:  info.ModTime().UTC().Truncate(time.Second).Format(time.RFC3339),
			}
			switch {
			case e.IsDir():
				te.Kind = "dir"
				te.HasChildren = dirHasChildren(filepath.Join(dir, name))
				dirs = append(dirs, te)
			case info.Mode().IsRegular():
				te.Kind = "file"
				te.Size = info.Size()
				files = append(files, te)
			default:
				continue
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
		entries := append(dirs, files...)
		var out []TreeEntry
		for _, te := range entries {
			if count >= treeCap {
				truncated = true
				break
			}
			count++
			out = append(out, te)
		}
		return out
	}
	entries = level(dir, rel)
	if entries == nil {
		// json.Marshal must never render a level as null: the client's
		// expand path treats a missing array as an error, an empty one as
		// "this dir has nothing visible".
		entries = []TreeEntry{}
	}
	return entries, truncated, true
}

// dirHasChildren reports whether a directory holds at least one entry the
// tree would show (skip rules applied) — the client's expand signal.
func dirHasChildren(dir string) bool {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range raw {
		name := e.Name()
		if skipNames[name] {
			continue
		}
		if strings.HasPrefix(name, "._") || strings.HasPrefix(name, ".~") {
			continue
		}
		return true
	}
	return false
}
