package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// lessons reads the guts' journals (one reading per act of every run) and ranks the classes that
// keep costing ground — by the reading's stable class code (fail, neg-lexeme, echo, thrash,
// repeat), never by its why, which embeds counts. A journal older than the codes groups by why. It aggregates; it does not consolidate — crystallizing a class into a
// skill or a policy is a writer's act, not the instrument's. Point it at a bench run's dir
// (`lessons bench/jobs/tb2-…`) to autopsy a grid, or run bare for the workspace's own loops.
func (a *App) cmdLessons(args []string, io IO) int {
	dir := filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "instruments", "loop")
	top := 3
	var pos []string
	for _, x := range args {
		if _, err := strconv.Atoi(x); err == nil && len(pos) > 0 {
			top, _ = strconv.Atoi(x)
			continue
		}
		pos = append(pos, x)
	}
	if len(pos) > 0 {
		dir = pos[0]
	}
	type cls struct {
		class  string
		why    string
		n, sum int
		spokes int
		lastAt string
	}
	by := map[string]*cls{}
	files := 0
	readings := 0
	walk := func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return err
		}
		// journals only: instruments/loop (workspace) or agent/loop (bench artifacts)
		if !strings.Contains(p, filepath.Join("instruments", "loop")) && !strings.Contains(p, filepath.Join("agent", "loop")) {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil // a lock riddle is not a lesson
		}
		defer f.Close()
		files++
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var ev map[string]interface{}
			if json.Unmarshal(sc.Bytes(), &ev) != nil || ev["t"] != "guts" {
				continue
			}
			readings++
			why, _ := ev["why"].(string)
			var classes []string
			if cs, ok := ev["classes"].([]interface{}); ok {
				for _, c := range cs {
					if s, _ := c.(string); s != "" && s != "positive" && s != "red-pin" {
						classes = append(classes, s)
					}
				}
			} else if why != "" {
				classes = []string{why} // a journal from before the class codes
			}
			for _, key := range classes {
				k := by[key]
				if k == nil {
					k = &cls{class: key, why: why}
					by[key] = k
				}
				k.n++
				if v, ok := ev["v"].(float64); ok {
					k.sum += int(v)
				}
				if ev["spoke"] == true {
					k.spokes++
				}
				if at, _ := ev["at"].(string); at > k.lastAt {
					k.lastAt = at
					k.why = why
				}
			}
		}
		return nil
	}
	if err := filepath.Walk(dir, walk); err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? lessons: %v\n", err)
		return 2
	}
	if readings == 0 {
		fmt.Fprintf(io.Out, "@S OK\n@? no guts readings under %s — runs journal them when policy.guts is on\n", dir)
		return 0
	}
	var all []*cls
	for _, k := range by {
		all = append(all, k)
	}
	// rank: most frequent first, ground-lost as the tiebreak
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].sum < all[j].sum
	})
	fmt.Fprintf(io.Out, "@S OK — %d readings across %d journals under %s\n", readings, files, dir)
	if len(all) == 0 {
		fmt.Fprintln(io.Out, "@? every reading landed neutral — nothing to learn yet")
		return 0
	}
	fmt.Fprintln(io.Out, "@F classes that keep costing ground:")
	if top > len(all) {
		top = len(all)
	}
	for _, k := range all[:top] {
		line := k.class
		if k.why != "" && k.why != k.class {
			line += " — " + k.why
		}
		fmt.Fprintf(io.Out, "@F %-40s ×%-3d value %-3d spoke %-3d last %s\n", line, k.n, k.sum, k.spokes, k.lastAt)
	}
	fmt.Fprintln(io.Out, "@? consolidate the top class into a skill or a policy — the instrument ranks; the crystallization is a writer's act")
	return 0
}
