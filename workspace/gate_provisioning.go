package workspace

import (
	"os"
	"path/filepath"
)

// Provisioning is how the tree grows: a zone-parent writes its own child's README, and the child is
// provisioned. Right-author would read that write as an author on another member's seat (the child
// is on the roster the moment its doc parses), and the orchestrator's founding of a coordinator as an
// unpaired re-authoring of a drifted doc. provisioningExempt is the one exemption, and it holds only
// while all four conditions do:
//
//  1. the path is the child's own README, on its seat;
//  2. the child names the writer as its one hop up (the orchestrator: a child of orchestrator);
//  3. the doc is new — absent from the last commit (provisioningUntracked): one commit window, after
//     which the doc is the child's own and re-authoring it is the ordinary arms' business;
//  4. every claim the child makes stands on the live tree, on soil of the writer's own line
//     (the writer and those above it) or open ground, never over a third agent's
//     (registerClaimsLiveTree) — a parent hands down what its line holds.
func (w *Workspace) provisioningExempt(as, rel string, zone Zone, before map[string][]string) bool {
	child := zone.Member
	if zone.Soil != SoilSeat || child == nil || child.Doc == "" || normRel(rel) != normRel(child.Doc) || child.Name == as {
		return false
	}
	parent := or(as, Orchestrator)
	if w.RankOf(parent).Name == Orchestrator {
		parent = Orchestrator
	}
	if child.Parent != parent {
		return false
	}
	return w.provisioningUntracked(child.Doc, child.Name, before) && w.registerClaimsLiveTree(child, parent)
}

// provisioningUntracked: the doc is absent from HEAD (staged or not, it was never committed). With
// no git to ask, the child must be absent from the roster the turn opened with.
func (w *Workspace) provisioningUntracked(doc, name string, before map[string][]string) bool {
	if _, err := w.gitOut("rev-parse --is-inside-work-tree"); err != nil {
		_, known := before[name]
		return !known
	}
	_, err := w.gitOut("cat-file -e HEAD:./" + filepath.ToSlash(filepath.Clean(doc)))
	return err != nil
}

// registerClaimsLiveTree: each of the child's claims exists on disk and neither covers nor sits under
// a claim of any living agent outside the child and its parent's line.
func (w *Workspace) registerClaimsLiveTree(child *Member, parent string) bool {
	line := map[string]bool{child.Name: true, parent: true}
	for _, up := range w.lineOf(parent) {
		line[up] = true
	}
	for _, t := range child.Ownership {
		if _, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(t))); err != nil {
			return false
		}
		for i := range w.Members {
			c := &w.Members[i]
			if line[c.Name] {
				continue
			}
			for _, p := range c.Ownership {
				if under(normRel(t), p) || under(normRel(p), t) {
					return false
				}
			}
		}
	}
	return true
}
