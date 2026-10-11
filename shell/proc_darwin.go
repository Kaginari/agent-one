//go:build darwin

package shell

import (
	"os/exec"
	"strconv"
	"strings"
)

// descendants lists every process under pid, parents before children. Darwin has no /proc, so
// the parentage walk rides `pgrep -P`, recursing child by child; a pid without children exits
// pgrep non-zero, which is simply the floor.
func descendants(pid int) []int {
	var out []int
	queue := []int{pid}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		b, err := exec.Command("pgrep", "-P", strconv.Itoa(cur)).Output()
		if err != nil {
			continue // no children (pgrep exits 1), or pgrep missing
		}
		for _, tok := range strings.Fields(string(b)) {
			c, err := strconv.Atoi(tok)
			if err != nil {
				continue
			}
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out
}
