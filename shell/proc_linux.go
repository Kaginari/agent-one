//go:build linux

package shell

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// descendants lists every process under pid, parents before children, from /proc.
func descendants(pid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	return walkChildren(pid, childrenFromProc(entries))
}

func childrenFromProc(entries []os.DirEntry) map[int][]int {
	children := map[int][]int{}
	for _, e := range entries {
		id, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		s := string(stat)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(s[i+1:])
		if len(f) < 2 {
			continue
		}
		ppid, _ := strconv.Atoi(f[1])
		children[ppid] = append(children[ppid], id)
	}
	return children
}

func walkChildren(pid int, children map[int][]int) []int {
	var out []int
	queue := []int{pid}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		out = append(out, children[cur]...)
		queue = append(queue, children[cur]...)
	}
	return out
}
