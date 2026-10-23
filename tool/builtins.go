package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/sandbox"
)

const (
	outputCap   = 24 << 10 // bytes of tool output handed to the model; the rest is a pointer
	bashTimeout = 600      // seconds, loop.js's default per-step time
	grepCap     = 200
	globCap     = 500
)

func fail(format string, a ...interface{}) Result {
	return Result{Output: fmt.Sprintf(format, a...), Err: true}
}

func decode(input json.RawMessage, v interface{}) error {
	if len(input) == 0 {
		return nil
	}
	return json.Unmarshal(input, v)
}

func clip(s string) string {
	if len(s) <= outputCap {
		return s
	}
	return s[:outputCap] + fmt.Sprintf("\n… [%d bytes clipped — read the file or narrow the ask]", len(s)-outputCap)
}

// ---- read

// readAutoLimit bounds a whole-file ask: past it read serves the first window and says how to
// widen (offset/limit) or, for Go, where the outline lives (skim). Named windows are untouched.
const readAutoLimit = 240

// ReadTool reads files — one path per call, or a batch through `paths`. Every call re-checks
// what it served: an unchanged window answers "withheld — unchanged since the first read this
// run" instead of re-shipping the bytes (the wire's budgets are measured in served bytes, and
// an agent touring a package re-reads its anchors constantly). A changed file hashes differently
// and is served fresh, so the cache can never go stale on an intervening edit.
func ReadTool() *Tool {
	var mu sync.Mutex
	type served struct {
		sum  uint64
		mod  time.Time
		size int64
		at   time.Time // when the window was last shipped
	}
	seen := map[string]served{}
	// readOne serves one window; the bool says it was withheld as already served
	readOne := func(env Env, rel string, offset, limit int, full bool) (string, bool, error) {
		abs := env.Resolve(rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			// a missing file answers with the closest siblings — the second guess costs a turn
			if sug := siblingSuggestions(abs); sug != "" {
				return "", false, fmt.Errorf("%v —%s", err, sug)
			}
			return "", false, err
		}
		if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
			return "", false, fmt.Errorf("%s is binary (%d bytes)", env.Rel(abs), len(data))
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(data) == 0 {
			lines = nil
		}
		start := offset
		if start < 1 {
			start = 1
		}
		if limit < 0 { // an agent's `tail`: last |limit| lines, exact claim, never auto-capped
			start = len(lines) + limit + 1
			if start < 1 {
				start = 1
			}
			limit, full = 0, true
		}
		if start > len(lines) {
			return fmt.Sprintf("(%s has %d lines; offset %d is past the end)", env.Rel(abs), len(lines), start), false, nil
		}
		windowed := limit > 0
		auto := !windowed && !full && len(lines) > readAutoLimit
		end := len(lines)
		if windowed && start-1+limit < end {
			end = start - 1 + limit
		}
		if auto && start-1+readAutoLimit < end {
			end = start - 1 + readAutoLimit
		}
		var b strings.Builder
		for i := start - 1; i < end; i++ {
			fmt.Fprintf(&b, "%6d\t%s\n", i+1, clampLine(lines[i], 2000))
		}
		if end < len(lines) {
			if auto {
				fmt.Fprintf(&b, "… %d more lines — widen with offset/limit (or full: true), or skim %s for the outline\n", len(lines)-end, env.Rel(abs))
			} else {
				fmt.Fprintf(&b, "… %d more lines (read offset:%d)\n", len(lines)-end, end+1)
			}
		}
		// the read-once cache: same window, same bytes → a pointer, not a re-serve. A file that
		// changed under us (mod/size drift) misses the cache and re-serves; the sum guards the
		// same-size same-second edit rank.
		key := fmt.Sprintf("%s\x00%d\x00%d", abs, start, end)
		out := b.String()
		h := fnv.New64a()
		h.Write([]byte(out))
		sum := h.Sum64()
		st, _ := os.Stat(abs)
		mu.Lock()
		prev, ok := seen[key]
		fresh := ok && prev.sum == sum && (!ok || st == nil || (prev.mod.Equal(st.ModTime()) && prev.size == st.Size()))
		if fresh && env.Folded != nil && !env.Folded(abs).Before(prev.at) {
			// the serve was folded off the wire since: the agent no longer holds those bytes,
			// so the same window ships again rather than pointing at what it cannot see
			fresh = false
		}
		if !fresh && st != nil {
			seen[key] = served{sum: sum, mod: st.ModTime(), size: st.Size(), at: time.Now()}
		}
		mu.Unlock()
		if fresh {
			return fmt.Sprintf("(%s window %d-%d withheld — unchanged since this run's first read; say why the bytes are needed again)", env.Rel(abs), start, end), true, nil
		}
		return strings.TrimRight(out, "\n"), false, nil
	}
	return &Tool{
		Name: "read",
		Description: "Read files. Lines are numbered, per-file headers on a batch. `path` is one file; `paths` is a batch of windows (same offset/limit each). " +
			"Unwindowed reads over 240 lines ship the first 240 with the widen hint; say full: true when every byte is honestly needed. skim gives a Go file's outline.",
		Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"paths":{"type":"array","items":{"type":"string"}},"offset":{"type":"integer"},"limit":{"type":"integer"},"full":{"type":"boolean"}},"anyOf":[{"required":["path"]},{"required":["paths"]}]}`),
		Class:  Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct {
				Path  string
				Paths []string
			}
			_ = decode(in, &a)
			names := a.Paths
			if a.Path != "" {
				names = append([]string{a.Path}, names...)
			}
			var abs []string
			for _, n := range names {
				abs = append(abs, env.Resolve(n))
			}
			for _, p := range abs {
				if !env.Inside(p) {
					return Classification{Class: Outward, Why: "path outside the workspace: " + p, Paths: abs}
				}
			}
			return Classification{Class: Read, Why: fmt.Sprintf("reads %d path(s)", len(abs)), Paths: abs}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Path   string
				Paths  []string
				Offset int
				Limit  int
				Full   bool
			}
			if err := decode(in, &a); err != nil || (a.Path == "" && len(a.Paths) == 0) {
				return fail("read: path or paths is required")
			}
			names := a.Paths
			if a.Path != "" {
				names = append([]string{a.Path}, names...)
			}
			if len(names) == 1 {
				out, unchanged, err := readOne(env, names[0], a.Offset, a.Limit, a.Full)
				if err != nil {
					return fail("read: %v", err)
				}
				if z := zoneOwner(env, env.Rel(env.Resolve(names[0]))); z != "" && !strings.HasSuffix(out, "\n") {
					out += "\n(zone of " + z + ")"
				}
				return Result{Output: clip(out), Unchanged: unchanged}
			}
			var b strings.Builder
			all := true // every window of the batch withheld as unchanged
			for i, n := range names {
				if i > 0 {
					b.WriteString("\n")
				}
				// a batch entry may pin its own window: "f.go:40" or "f.go:40:60" — the
				// shared offset/limit stays the floor for entries that don't.
				name, o, l := splitReadWindow(n, a.Offset, a.Limit)
				out, unchanged, err := readOne(env, name, o, l, a.Full)
				if err != nil {
					out = "! " + err.Error()
				}
				all = all && unchanged && err == nil
				fmt.Fprintf(&b, "== %s ==\n%s\n", name, out)
			}
			return Result{Output: clip(b.String()), Unchanged: all}
		},
	}
}

// ---- write

func WriteTool() *Tool {
	return &Tool{
		Name:        "write",
		Description: "Write a whole file (creating directories). Overwriting a record (AGENT-ONE.md, log.md, design, notes.jsonl) is destructive and gated.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			return env.ClassifyPath(env.Resolve(a.Path))
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct{ Path, Content string }
			if err := decode(in, &a); err != nil || a.Path == "" {
				return fail("write: path is required")
			}
			abs := env.Resolve(a.Path)
			var drift string
			if old, err := os.ReadFile(abs); err == nil {
				// the blind-overwrite drift receipt: an overwrite names what it replaced so a
				// runaway full-write is visible in the wire, not silent ground truth
				switch {
				case string(old) == a.Content:
					drift = fmt.Sprintf(" — over same %d bytes, no drift", len(old))
				default:
					drift = fmt.Sprintf(" — over %d bytes now %d (drift %+d)", len(old), len(a.Content), len(a.Content)-len(old))
				}
			}
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return fail("write: %v", err)
			}
			if err := os.WriteFile(abs, []byte(a.Content), 0o644); err != nil {
				return fail("write: %v", err)
			}
			out := fmt.Sprintf("wrote %s (%d bytes)%s", env.Rel(abs), len(a.Content), drift)
			if z := zoneOwner(env, env.Rel(abs)); z != "" {
				out += " · zone of " + z
			}
			return Result{Output: out, Wrote: []string{abs}}
		},
	}
}

// ---- edit

func EditTool() *Tool {
	return &Tool{
		Name:        "edit",
		Description: "Replace one exact string in a file. old must match exactly once (or set replace_all).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old","new"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			c := env.ClassifyPath(env.Resolve(a.Path))
			if c.Class == Destructive {
				c.Why = "in-place edit of a record: " + env.Rel(c.Paths[0])
			}
			return c
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Path, Old, New string
				ReplaceAll     bool `json:"replace_all"`
			}
			if err := decode(in, &a); err != nil || a.Path == "" || a.Old == "" {
				return fail("edit: path and a non-empty old are required")
			}
			abs := env.Resolve(a.Path)
			data, err := os.ReadFile(abs)
			if err != nil {
				return fail("edit: %v", err)
			}
			n := strings.Count(string(data), a.Old)
			switch {
			case n == 0:
				hint := editHint(string(data), a.Old)
				if hint != "" {
					return fail("edit: old string not found in %s — %s", env.Rel(abs), hint)
				}
				return fail("edit: old string not found in %s", env.Rel(abs))
			case n > 1 && !a.ReplaceAll:
				var lns []int
				for i, l := range strings.Split(string(data), "\n") {
					if strings.Contains(l, a.Old) {
						lns = append(lns, i+1)
						if len(lns) == 10 {
							break
						}
					}
				}
				where := ""
				if len(lns) > 0 {
					where = fmt.Sprintf(" (lines %s)", trimJoin(lns))
				}
				return fail("edit: old string matches %d times in %s%s — make it unique or set replace_all", n, env.Rel(abs), where)
			}
			out := strings.Replace(string(data), a.Old, a.New, map[bool]int{true: -1, false: 1}[a.ReplaceAll])
			if err := os.WriteFile(abs, []byte(out), 0o644); err != nil {
				return fail("edit: %v", err)
			}
			msg := fmt.Sprintf("edited %s (%d replacement%s)", env.Rel(abs), n, map[bool]string{true: "s", false: ""}[n != 1])
			if z := zoneOwner(env, env.Rel(abs)); z != "" {
				msg += " · zone of " + z
			}
			return Result{Output: msg, Wrote: []string{abs}}
		},
	}
}

// editHint is the near-miss the "not found" error owes the model: which line carries the old
// string's most distinctive word, or its own first line. A dead "not found" pays a second
// read; a line-pointer pays none.
func editHint(data, old string) string {
	var needles []string
	if fl, _, _ := strings.Cut(old, "\n"); len(fl) >= 8 {
		needles = append(needles, fl)
	}
	bestWord := ""
	for _, w := range strings.Fields(old) {
		if alnum(w) && len(w) > len(bestWord) {
			bestWord = w
		}
	}
	if bestWord != "" && (len(needles) == 0 || bestWord != needles[0]) {
		needles = append(needles, bestWord)
	}
	for _, needle := range needles {
		for i, l := range strings.Split(data, "\n") {
			if strings.Contains(l, needle) {
				return fmt.Sprintf("closest: line %d: %s", i+1, strings.TrimSpace(truncateRunes(l, 100)))
			}
		}
	}
	return ""
}

func alnum(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return false
		}
	}
	return len(s) > 0
}

func trimJoin(v []int) string {
	var b strings.Builder
	for i, n := range v {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%d", n)
		if i == 9 {
			break
		}
	}
	return b.String()
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// clampLine caps one physical line — minified/generated single lines are token bombs.
func clampLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + " [truncated]"
}

// siblingSuggestions answers "did you mean …" for a missing file, from its directory's
// listing (case-insensitive substring on the base name; up to 3, sorted).
func siblingSuggestions(abs string) string {
	dir := filepath.Dir(abs)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	base := strings.ToLower(filepath.Base(abs))
	var close, all []string
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(strings.ToLower(name), base) || strings.Contains(base, strings.ToLower(name)) {
			close = append(close, name)
			continue
		}
		if len(base) >= 3 {
			all = append(all, name)
		}
	}
	pick := close
	if len(pick) == 0 {
		pick = all
	}
	sort.Strings(pick)
	if len(pick) > 3 {
		pick = pick[:3]
	}
	if len(pick) == 0 {
		return ""
	}
	return " did you mean: " + strings.Join(pick, ", ")
}

// ReadPaths is the files a read call names, absolute, each batch entry's own window peeled.
func ReadPaths(env Env, in json.RawMessage) []string {
	var a struct {
		Path  string
		Paths []string
	}
	_ = decode(in, &a)
	names := a.Paths
	if a.Path != "" {
		names = append([]string{a.Path}, names...)
	}
	var abs []string
	for _, n := range names {
		name, _, _ := splitReadWindow(n, 0, 0)
		abs = append(abs, env.Resolve(name))
	}
	return abs
}

// splitReadWindow peels an optional per-path window off a batch entry: "f.go:40" wins the
// offset, "f.go:40:60" both ends. The shared offset/limit hold otherwise.
func splitReadWindow(n string, off, limit int) (string, int, int) {
	parts := strings.Split(n, ":")
	switch len(parts) {
	case 1:
		return n, off, limit
	case 2:
		if v, err := strconv.Atoi(parts[1]); err == nil {
			return parts[0], v, limit
		}
	case 3:
		o, eo := strconv.Atoi(parts[1])
		l, el := strconv.Atoi(parts[2])
		if eo == nil && el == nil {
			return parts[0], o, max(l, 1)
		}
	}
	return n, off, limit
}

// ---- bash

func BashTool() *Tool {
	return &Tool{
		Name:        "bash",
		Description: "Run a shell command in the workspace root. Its class is read from the command (git push, curl are outward; rm, git reset are destructive); `class` may declare a tighter one.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"cwd":{"type":"string"},"timeout":{"type":"integer","description":"seconds"},"class":{"type":"string","enum":["read","write","outward","destructive"]}},"required":["command"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Command, Cwd string }
			_ = decode(in, &a)
			if a.Cwd != "" {
				env.Cwd = env.Resolve(a.Cwd)
			}
			c := env.ClassifyCommand(a.Command)
			if a.Cwd != "" && !env.Inside(env.Cwd) && c.Class < Outward {
				c = Classification{Class: Outward, Why: "cwd outside the workspace: " + a.Cwd}
			}
			return c
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Command, Cwd string
				Timeout      int
			}
			if err := decode(in, &a); err != nil || strings.TrimSpace(a.Command) == "" {
				return fail("bash: command is required")
			}
			dir := env.Dir()
			if a.Cwd != "" {
				dir = env.Resolve(a.Cwd)
			}
			t := a.Timeout
			if t <= 0 {
				t = env.Timeout
			}
			if t <= 0 {
				t = bashTimeout
			}
			cctx, cancel := context.WithTimeout(ctx, time.Duration(t)*time.Second)
			defer cancel()
			// the shelf's bash runs in the sandboxed shell (NewBashTool); this one, for a bare
			// registry, still never hands a secret to the command or outlives its timeout
			cmd := sandbox.Contain(exec.CommandContext(cctx, "bash", "-c", a.Command))
			cmd.Dir = dir
			cmd.Env = append(sandbox.ScrubEnv(os.Environ(), nil, nil), "AGENT_ONE_ROOT="+env.Root)
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			t0 := time.Now()
			err := cmd.Run()
			code := 0
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else {
					code = 1
				}
			}
			var b strings.Builder
			b.WriteString(out.String())
			if errb.Len() > 0 {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteString("\n")
				}
				b.WriteString("[stderr]\n" + errb.String())
			}
			if cctx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(&b, "\n[killed after %ds]", t)
				code = 124
			}
			if code != 0 {
				fmt.Fprintf(&b, "\n[exit %d, %s]", code, time.Since(t0).Round(time.Millisecond))
			}
			return Result{Output: clip(b.String()), Err: code != 0}
		},
	}
}

// ---- glob

func GlobTool() *Tool {
	return &Tool{
		Name:        "glob",
		Description: "Find files by pattern (** allowed), relative to the workspace root or `path`. Sorted, newest first.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs}
			}
			return Classification{Class: Read, Why: "lists " + env.Rel(abs)}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct{ Pattern, Path string }
			if err := decode(in, &a); err != nil || a.Pattern == "" {
				return fail("glob: pattern is required")
			}
			base := env.Resolve(a.Path)
			hits, err := Glob(base, a.Pattern)
			if err != nil {
				return fail("glob: %v", err)
			}
			if len(hits) == 0 {
				return Result{Output: "no files match " + a.Pattern}
			}
			var b strings.Builder
			for i, h := range hits {
				if i == globCap {
					fmt.Fprintf(&b, "… %d more\n", len(hits)-globCap)
					break
				}
				b.WriteString(env.Rel(h) + "\n")
			}
			return Result{Output: b.String()}
		},
	}
}

var skipDirs = map[string]bool{".git": true, "node_modules": true}

// Glob walks base and returns the absolute paths matching pattern; `**` spans directories.
// Results are newest-modified first.
func Glob(base, pattern string) ([]string, error) {
	pattern = filepath.ToSlash(filepath.Clean(pattern))
	parts := strings.Split(pattern, "/")
	type hit struct {
		p string
		t time.Time
	}
	var hits []hit
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if p != base && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if matchParts(parts, strings.Split(filepath.ToSlash(rel), "/")) {
			hits = append(hits, hit{p, info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].t.After(hits[j].t) })
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.p
	}
	return out, nil
}

func matchParts(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchParts(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if ok, _ := filepath.Match(pat[0], segs[0]); !ok {
		return false
	}
	return matchParts(pat[1:], segs[1:])
}

// ---- grep

func GrepTool() *Tool {
	return &Tool{
		Name:        "grep",
		Description: "Search file contents with a Go regexp under `path` (default: the workspace root). `include` filters by file name glob. Output: path:line:text.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"include":{"type":"string"},"ignore_case":{"type":"boolean"}},"required":["pattern"]}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs}
			}
			return Classification{Class: Read, Why: "searches " + env.Rel(abs)}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Pattern, Path, Include string
				IgnoreCase             bool `json:"ignore_case"`
			}
			if err := decode(in, &a); err != nil || a.Pattern == "" {
				return fail("grep: pattern is required")
			}
			pat := a.Pattern
			if a.IgnoreCase {
				pat = "(?i)" + pat
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return fail("grep: %v", err)
			}
			base := env.Resolve(a.Path)
			var b strings.Builder
			n, files := 0, 0
			_ = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
				if err != nil || n >= grepCap {
					return nil
				}
				if info.IsDir() {
					if p != base && skipDirs[info.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				if a.Include != "" {
					if ok, _ := filepath.Match(a.Include, info.Name()); !ok {
						return nil
					}
				}
				if info.Size() > 8<<20 {
					return nil
				}
				data, err := os.ReadFile(p)
				if err != nil || bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
					return nil
				}
				files++
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						n++
						if len(line) > 300 {
							line = line[:300] + "…"
						}
						fmt.Fprintf(&b, "%s:%d:%s\n", env.Rel(p), i+1, line)
						if n >= grepCap {
							fmt.Fprintf(&b, "… stopped at %d matches\n", grepCap)
							return filepath.SkipDir
						}
					}
				}
				return nil
			})
			if n == 0 {
				return Result{Output: fmt.Sprintf("no matches for %q in %d files", a.Pattern, files)}
			}
			return Result{Output: clip(b.String())}
		},
	}
}
