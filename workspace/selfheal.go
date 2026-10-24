package workspace

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/onto"
)

// Self-heal: the committed docs are the store, the roster is their cache. A doc another hand
// moved — a second session's relief, a human's edit, a checkout — left the cache standing until
// the next gate re-read it, so a dispatch could still register a subagent for a retired seat or refuse a
// agent whose claim had grown. The roster keeps a stamp of what it was derived from (every rank
// dir's seats and their .md files: name, size, mtime); SelfHeal compares the stamp and re-derives
// when the docs moved. The decisions that act on the roster ask it first: the dispatch preflight,
// an engine's build, a write's ownership policy, and a roster miss.
//
// Healing never moves trust. The verify lines the gate runs as "before the turn" (verifyBase)
// change only at an explicit Reload (Open, the gate, the harness's own registers); a heal leaves
// them as they were, a never-seen agent's included — an agent cannot pass its gate by rewriting its
// check and waiting for a heal, and a child registered this turn is not "known" before its provisioning.

// docStamp fingerprints the member docs on disk.
func (w *Workspace) docStamp() uint64 {
	h := fnv.New64a()
	for _, d := range rankDirs(w.Ranks) {
		base := filepath.Join(w.Dir(), d)
		seats, err := os.ReadDir(base)
		if err != nil {
			fmt.Fprintf(h, "%s:-\n", d)
			continue
		}
		for _, s := range seats {
			if !s.IsDir() {
				continue
			}
			files, _ := os.ReadDir(filepath.Join(base, s.Name()))
			var mds []string
			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
					continue
				}
				if fi, err := f.Info(); err == nil {
					mds = append(mds, fmt.Sprintf("%s:%d:%d", f.Name(), fi.Size(), fi.ModTime().UnixNano()))
				}
			}
			sort.Strings(mds)
			fmt.Fprintf(h, "%s/%s:%s\n", d, s.Name(), strings.Join(mds, ","))
		}
	}
	return h.Sum64()
}

// SelfHeal re-derives the roster when the docs moved since it was read; true when it did.
func (w *Workspace) SelfHeal() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.selfHealLocked()
}

// selfHealLocked is SelfHeal with w.mu held.
func (w *Workspace) selfHealLocked() bool {
	st := w.docStamp()
	if st == w.stamp {
		return false
	}
	ow, err := onto.LoadLayout(w.Root, w.Ranks.Layout(w.Lex))
	if err != nil {
		return false // the cache stands until the docs parse again
	}
	w.mergeFreshLocked(ow)
	w.stamp = st
	return true
}

// mergeFreshLocked puts a fresh derivation on the roster: the docs' members, then the foreign
// agents the docs do not name. The gate's base is not touched: an agent the heal brings in is not
// on it until the next explicit Reload — its lines are new to the gate (never run unconfirmed),
// and a no-git provisioning still reads the child as new (gate_provisioning.go).
func (w *Workspace) mergeFreshLocked(ow *onto.Workspace) {
	w.Onto = ow
	w.Members = membersOf(w.Root, ow, w.Ranks)
	for _, f := range w.Foreign {
		if w.memberLocked(f.Name) == nil {
			w.Members = append(w.Members, f)
		}
	}
}
