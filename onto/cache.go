package onto

import (
	"encoding/binary"
	"hash/fnv"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Kaginari/agent-one/discover"
)

// Stamp fingerprints every input LoadLayout reads — the ontology's turtle, the rank dirs'
// member docs, the skill bases' skills — by path, size and modification time, directories
// included (an entry added or removed moves its dir's time). Equal stamps load equal workspaces; a
// walk of stats costs a small fraction of a parse, derive and reason.
func Stamp(root string, l Layout) uint64 {
	l = l.norm()
	roots := []string{OntologyDirIn(root, l)}
	for _, r := range l.Ranks {
		if r.Dir != "" {
			roots = append(roots, filepath.Join(root, l.WorkspaceDir, r.Dir))
		}
	}
	for _, b := range discover.SkillBases(l.WorkspaceDir) {
		roots = append(roots, filepath.Join(root, b))
	}
	h := fnv.New64a()
	var buf [16]byte
	for _, r := range roots {
		_ = filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // an absent root is a stamp like any other: absent
			}
			if !d.IsDir() && !strings.HasSuffix(p, ".md") && !strings.HasSuffix(p, ".ttl") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			h.Write([]byte(p))
			binary.LittleEndian.PutUint64(buf[:8], uint64(info.Size()))
			binary.LittleEndian.PutUint64(buf[8:], uint64(info.ModTime().UnixNano()))
			h.Write(buf[:])
			return nil
		})
	}
	return h.Sum64()
}

var cache struct {
	mu sync.Mutex
	at map[string]cached
}

type cached struct {
	stamp uint64
	w     *Workspace
}

// Cached is LoadLayout memoized on Stamp: the boards poll it every second, and the workspace it
// answers moves only when a doc or a turtle file does. The Workspace is shared — read it, never add
// to it (a fact is recorded on a workspace of its own, from LoadLayout).
func Cached(root string, l Layout) (*Workspace, error) {
	key := root + "\x00" + strings.Join(append([]string{l.WorkspaceDir, l.Policy}, rankKeys(l.Ranks)...), "\x00")
	stamp := Stamp(root, l)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if c, ok := cache.at[key]; ok && c.stamp == stamp {
		return c.w, nil
	}
	w, err := LoadLayout(root, l)
	if err != nil {
		return nil, err
	}
	if cache.at == nil {
		cache.at = map[string]cached{}
	}
	cache.at[key] = cached{stamp, w}
	return w, nil
}

func rankKeys(rs []RankDir) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name+"/"+r.Dir+"/"+r.Parent)
	}
	return out
}
