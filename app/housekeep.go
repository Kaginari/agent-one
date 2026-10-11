package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kaginari/agent-one/onto"
)

// cmdHousekeep: the chain policy, on demand. Check-mode lists findings; --heal seals the
// mechanical classes; --spawn seats draft zones on tracked dirs no member covers.
func cmdHousekeep(dist string, f cliFlags, io IO) int {
	if f.attribute {
		return cmdAttribute(dist, f, io) // the audit arm: ownership findings, proposals only
	}
	root := f.root
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)
	rep, findings, err := onto.Housekeep(root, f.heal, f.spawn)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? housekeep: %v\n", err)
		return 2
	}
	if !f.heal && !f.spawn {
		for _, fd := range findings {
			fmt.Fprintln(io.Out, fd.String())
		}
		for _, n := range rep.Notes {
			fmt.Fprintln(io.Out, n)
		}
		if len(findings) == 0 && len(rep.Notes) == 0 {
			fmt.Fprintln(io.Out, "@S PASS — 0 findings, 0 notes, the chain is whole")
		} else {
			fmt.Fprintf(io.Out, "@S %d finding(s) · %d note(s) — `housekeep --heal` seals the mechanical classes\n", len(findings), len(rep.Notes))
		}
		return 0
	}
	for _, r := range rep.Repairs {
		fmt.Fprintf(io.Out, "@S REP %s — %s\n", r.Path, r.What)
	}
	for _, s := range rep.Spawned {
		fmt.Fprintf(io.Out, "@S SPAWN %s\n", s)
	}
	for _, l := range rep.Leftover {
		fmt.Fprintf(io.Out, "@? %s\n", l)
	}
	fmt.Fprintf(io.Out, "@S housekeep: %d sealed · %d finding(s) · %d note(s) remain\n@E %d\n", rep.Healed, len(findings), len(rep.Notes), len(findings)+len(rep.Notes))
	return 0
}
