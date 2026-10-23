package tool

import (
	"fmt"
	"strconv"
	"strings"
)

// FileReadHint reads a shell command that only prints one file or a window of it — cat, head,
// tail, sed -n 'A,Bp' — and answers the read path that serves the same lines ("f.go",
// "f.go:40:20", "f.go:-20"); "" for anything else. A hint, never a verdict: the command's class
// is the classifier's alone, and anything with a pipe, a redirect, a glob or an expansion is
// not a plain read and gets no hint.
func FileReadHint(cmd string) string {
	if strings.ContainsAny(cmd, "|&;<>$`(){}*?[]\\\n") {
		return ""
	}
	f := strings.Fields(cmd)
	for i := range f {
		f[i] = strings.Trim(f[i], `'"`)
	}
	file := func(s string) bool { return s != "" && !strings.HasPrefix(s, "-") }
	count := func(args []string) (int, string, bool) { // -n N F · -N F · F
		switch {
		case len(args) == 1 && file(args[0]):
			return 10, args[0], true
		case len(args) == 2 && strings.HasPrefix(args[0], "-") && file(args[1]):
			n, err := strconv.Atoi(strings.TrimPrefix(args[0], "-"))
			return n, args[1], err == nil && n > 0
		case len(args) == 3 && args[0] == "-n" && !strings.HasPrefix(args[1], "+") && file(args[2]):
			n, err := strconv.Atoi(args[1])
			return n, args[2], err == nil && n > 0
		}
		return 0, "", false
	}
	if len(f) < 2 {
		return ""
	}
	switch f[0] {
	case "cat":
		if len(f) == 2 && file(f[1]) {
			return f[1]
		}
	case "head":
		if n, p, ok := count(f[1:]); ok {
			return fmt.Sprintf("%s:1:%d", p, n)
		}
	case "tail":
		if n, p, ok := count(f[1:]); ok {
			return fmt.Sprintf("%s:-%d", p, n)
		}
	case "sed":
		if len(f) == 4 && f[1] == "-n" && strings.HasSuffix(f[2], "p") && file(f[3]) {
			a, b, _ := strings.Cut(strings.TrimSuffix(f[2], "p"), ",")
			from, err := strconv.Atoi(a)
			to := from
			if b != "" {
				to, _ = strconv.Atoi(b)
			}
			if err == nil && from > 0 && to >= from {
				return fmt.Sprintf("%s:%d:%d", f[3], from, to-from+1)
			}
		}
	}
	return ""
}
