package tool

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ValidCustomName reports whether a name may be a tools.custom entry (the same rule CustomTool
// enforces) — checked before any config path is built from it.
func ValidCustomName(name string) bool { return customName.MatchString(name) }

// SplitArgv splits a command line into argv the way a POSIX shell would quote it, and nothing
// more: whitespace separates words; '…' is literal; "…" keeps its text with \ escaping \ " $ `
// and newline; a bare \ escapes the next character. It never expands anything. A shell operator
// outside quotes (| || & && ; < > ` $( ) is refused: an argv runs without a shell, so a pipeline
// would arrive as literal words — that is a shell template, not an argv.
func SplitArgv(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			out = append(out, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		case c == '\'':
			inWord = true
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				cur.WriteRune(rs[j])
				j++
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unterminated single quote")
			}
			i = j
		case c == '"':
			inWord = true
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) && strings.ContainsRune("\\\"$`\n", rs[j+1]) {
					j++
					if rs[j] != '\n' {
						cur.WriteRune(rs[j])
					}
					continue
				}
				if rs[j] == '`' || (rs[j] == '$' && j+1 < len(rs) && rs[j+1] == '(') {
					return nil, fmt.Errorf("command substitution inside double quotes: an argv never runs a shell")
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unterminated double quote")
			}
			i = j
		case c == '\\':
			if i+1 >= len(rs) {
				return nil, fmt.Errorf("a trailing backslash escapes nothing")
			}
			inWord = true
			i++
			if rs[i] != '\n' {
				cur.WriteRune(rs[i])
			}
		case strings.ContainsRune("|&;<>`", c) || (c == '$' && i+1 < len(rs) && rs[i+1] == '('):
			return nil, fmt.Errorf("shell operator %q outside quotes: an argv runs without a shell (quote it if it is data)", string(c))
		default:
			inWord = true
			cur.WriteRune(c)
		}
	}
	flush()
	return out, nil
}

// QuoteArgv joins an argv back into one line a shell would split the same way (for the
// classifier and for display).
func QuoteArgv(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\|&;<>()$`*?[]#~=%!{}") {
			q[i] = a
			continue
		}
		q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}

// interpreters run their argument as code when given an eval flag.
var interpreters = map[string][]string{
	"sh": {"-c"}, "bash": {"-c"}, "zsh": {"-c"}, "dash": {"-c"}, "ksh": {"-c"}, "fish": {"-c", "--command"},
	"python": {"-c"}, "python2": {"-c"}, "python3": {"-c"},
	"node": {"-e", "--eval", "-p", "--print"}, "deno": {"eval"}, "bun": {"-e", "--eval"},
	"perl": {"-e", "-E"}, "ruby": {"-e"}, "php": {"-r"},
}

// InterpreterEval names the shape of an argv that hands a string to an interpreter to run
// (`bash -c …`, `python3 -c …`, `node -e …`, `eval …`) — "" when it is a plain command. Leading
// `env` and VAR=value words are looked through. Such a tool is a shell in disguise: what it runs
// is decided by a parameter, never by its declaration.
func InterpreterEval(argv []string) string {
	i := 0
	for i < len(argv) {
		w := argv[i]
		if filepath.Base(w) == "env" || (strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "=")) {
			i++
			continue
		}
		break
	}
	if i >= len(argv) {
		return ""
	}
	cmd := filepath.Base(argv[i])
	if cmd == "eval" {
		return "eval"
	}
	flags, ok := interpreters[cmd]
	if !ok {
		return ""
	}
	for _, a := range argv[i+1:] {
		for _, f := range flags {
			if a == f || (len(f) == 2 && f[0] == '-' && strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsRune(a[1:], rune(f[1]))) {
				return cmd + " " + f
			}
		}
	}
	return ""
}

// ArgvParams are the {{param}} holes of an argv, each once, sorted — the names a custom tool
// declares (the same placeholder rule its runner fills).
func ArgvParams(argv []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range argv {
		for _, m := range placeholder.FindAllStringSubmatch(a, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	sort.Strings(out)
	return out
}

// FillArgv fills every {{param}} hole; each element stays one argv entry.
func FillArgv(argv []string, params map[string]string) []string { return fillArgv(argv, params) }
