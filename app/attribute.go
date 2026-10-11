package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/config"
)

// attribute — housekeep's audit arm (Operator order 2026-10-03: "audit is better" over me
// hand-editing the coordinator's ownership line). Every tracked, untracked-non-ignored, or root-level
// ignored file no member's ownership owns is the gate's repeat-failure class — the coordinator's
// HANDOFF-debt had exactly this shape and burned every coordinator turn until surfaced.
//
// The audit REPORTS, never assigns: an ownership line is member soil (Principle 4, quoted
// ownership is out-loud policy). What it owes the human instead: for every unowned file, the
// exact zone it suggests and the report text to hand the workspace's agents.

type ownershipFinding struct {
	Path       string // inventory-relative path
	ZonedBy    string // how the suggestion was measured: "same-dir majority" | "ancestor ownership" | "journal class"
	Suggest    string // member home to hand the Ownership line (e.g. coord/agent-one)
	Why        string // the human-readable reason
	Suggestion string // the concrete Ownership-line fragment to add
}

// inventory lists the files the audit charges against ownerships: tracked (git ls-files),
// untracked-not-ignored (git status '??'), and root-level ignored files ('!!' at the root —
// e.g. log-git.md, which agents write even while git blindness is lawful).
func gitInventory(root string) ([]string, error) {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		b, err := cmd.Output()
		return string(b), err
	}
	files := map[string]bool{}
	if out, err := run("ls-files"); err == nil {
		for _, f := range strings.Fields(out) {
			files[f] = true
		}
	}
	if out, err := run("status", "--porcelain", "--untracked-files=all"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if len(line) < 4 {
				continue
			}
			tag, p := line[:2], strings.TrimSpace(line[3:])
			if tag == "??" {
				files[p] = true
			} else if tag == "!!" && !strings.Contains(p, "/") {
				files[p] = true // root-level ignored journal surfaces (log-git.md)
			}
		}
	}
	var out []string
	for f := range files {
		if !strings.HasPrefix(f, ".agent-one/") { // member soil is the workspace's own graph's charge
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out, nil
}

// journalClass is the fallback taxonomy: root-level chronicles/voice hold the coordinator's soil by
// census (the shared skill and voice), independent of prefix evidence.
var journalClass = map[string]bool{
	"README.md": true, "CHANGELOG.md": true, "HANDOFF.md": true, "log-git.md": true, "log.md": true,
	"review-chart.md": true,
}

// auditOwnership files every inventory path the owner func answers "" for, with the strongest
// suggestion available: dir-majority, then ancestor ownership, then the journal class home.
// findOwner is workspace.Owner over a real workspace; pure for tests.
func auditOwnership(files []string, owner func(rel string) string) []ownershipFinding {
	dirOwners := map[string]map[string]int{}
	owned := map[string]string{}
	for _, f := range files {
		o := owner(f)
		if o == "" {
			continue
		}
		owned[f] = o
		dir := filepath.Dir(f)
		if dirOwners[dir] == nil {
			dirOwners[dir] = map[string]int{}
		}
		dirOwners[dir][o]++
	}
	var out []ownershipFinding
	for _, f := range files {
		if owned[f] != "" {
			continue
		}
		fnd := ownershipFinding{Path: f}
		dir := filepath.Dir(f)
		// s1: the directory's plurality of owners
		best, bestN := "", 0
		for o, n := range dirOwners[dir] {
			if n > bestN {
				best, bestN = o, n
			}
		}
		if best != "" {
			fnd.ZonedBy, fnd.Suggest = "same-dir majority", best
			fnd.Why = fmt.Sprintf("this dir's files are owned by %s in %d case%s", best, bestN, plural(bestN))
			fnd.Suggestion = ownershipLine(best, f)
			out = append(out, fnd)
			continue
		}
		// s2: longest ancestor dir that resolves
		for d := dir; d != "." && d != "/"; d = filepath.Dir(d) {
			if o := owner(d); o != "" {
				fnd.ZonedBy, fnd.Suggest = "ancestor ownership", o
				fnd.Why = fmt.Sprintf("the ancestor directory %s is %s's soil", d, o)
				fnd.Suggestion = fmt.Sprintf("— file rides %s's ownership prefix %s — no line needed if the prefix already holds it PREFIX-checked; else add: %s", o, d, ownershipLine(o, f))
				out = append(out, fnd)
				break
			}
		}
		if fnd.Suggest != "" {
			continue
		}
		if journalClass[filepath.Base(f)] && !strings.Contains(f, "/") {
			fnd.ZonedBy, fnd.Suggest = "journal class", "coord/agent-one"
			fnd.Why = "a root chronicle/voice file — the shared skill and voice's charge by census"
			fnd.Suggestion = ownershipLine("coord/agent-one", f)
		} else {
			fnd.ZonedBy, fnd.Suggest = "no trace", ""
			fnd.Why = "no sibling owner, no ancestor ownership, no census class — decide out loud (assign a member or exempt it as orchestrator seat)"
		}
		out = append(out, fnd)
	}
	return out
}

func ownershipLine(memberHome, path string) string {
	return fmt.Sprintf("add to .agent-one/%s/README.md's Ownership line: backticked `%s`", memberHome, path)
}

// cmdAttribute audits the root's file inventory against member ownerships. Reports only.
func cmdAttribute(dist string, f cliFlags, io IO) int {
	root := f.root
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)
	cfg, err := config.Load(dist)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? attribute: config: %v\n", err)
		return 2
	}
	lex := lexiconFor()
	w, err := openWorkspace(cfg, root, lex)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? attribute: %v\n", err)
		return 2
	}
	files, err := gitInventory(root)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? attribute: %v\n", err)
		return 2
	}
	owner := func(rel string) string {
		if c := w.Owner(rel); c != nil {
			return c.Name
		}
		return ""
	}
	findings := auditOwnership(files, owner)
	fmt.Fprintf(io.Out, "@S attribute — %d files inventoried · %d unowned\n", len(files), len(findings))
	for _, fd := range findings {
		fmt.Fprintf(io.Out, "@? unowned: %s — %s [%s]\n", fd.Path, fd.Why, fd.ZonedBy)
		if fd.Suggestion != "" {
			fmt.Fprintf(io.Out, "@F   %s\n", fd.Suggestion)
		}
	}
	if len(findings) == 0 {
		fmt.Fprintln(io.Out, "@S chain whole — every inventoried file rides an ownership")
	}
	fmt.Fprintln(io.Out, "@? assignments land in the member's Ownership line via its owner act — never from this command (Principle 4, out-loud)")
	return 0
}
