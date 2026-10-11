package tui

import (
	"flag"
	"fmt"
	"github.com/charmbracelet/colorprofile"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("goldens", false, "rewrite the golden files")

// Every block, at 80 and 120 columns, against a golden text with the escapes stripped. The
// goldens are looked at, not only compared: a wrong wrap or a cut border shows there.
func TestBlocksGolden(t *testing.T) {
	th := NewTheme(true)
	long := strings.Repeat("a line of output that goes on for a while to test the wrapping and truncation of long output lines ", 1)
	var many []string
	for i := 1; i <= 30; i++ {
		many = append(many, fmt.Sprintf("line %d of the output", i))
	}
	before := "package auth\n\nfunc Login() {\n\treturn\n}\n\nfunc Logout() {}\n"
	after := "package auth\n\nimport \"time\"\n\nfunc Login() {\n\tttl := 15 * time.Minute\n\t_ = ttl\n\treturn\n}\n\nfunc Logout() {}\n"
	diff := DiffText(before, after, 2)
	cases := []struct {
		name string
		f    func(width int) string
	}{
		{"welcome", func(w int) string {
			return th.Welcome(Welcome{Dist: "agent-one", Version: "v0.1.5", Workspace: "/home/monta/kaginari/pfcli", Model: "gemini/gemini-3.8-flash",
				Roles: []string{"analyst → gemini/gemini-3.8-flash", "judge → gemini/gemini-3.8-flash", "drafter → gemini/gemini-3.8-flash"},
				Off:   2, Session: "20260926-171200-a1b2c3", Holes: []string{"GEMINI_API_KEY is not set — the mount has no key"}}, w)
		}},
		{"user", func(w int) string {
			return th.User("explain the gate in this workspace and then run the tests under src/auth — take your time, I want the full picture of what the verify beat reads", w)
		}},
		{"assistant", func(w int) string {
			return th.Assistant("The gate has **four checks** (Policy 3):\n\n1. Right zone worker authored\n2. Traits hold\n3. Duties done\n4. Doc truthful\n\n```go\nfunc Gate(w *Workspace) Verdict {\n\treturn w.Check(4)\n}\n```\n\nRun `go test ./...` to see the verify beat read the exit codes.", w)
		}},
		{"tool-bash", func(w int) string {
			return th.Tool(ToolView{ID: "s1", Name: "bash", Summary: "go test ./...", Class: "read", Status: "done", Ms: 1234, Output: "ok  \tgithub.com/x/y\t0.512s\nok  \tgithub.com/x/z\t0.201s\n"}, w)
		}},
		{"tool-running", func(w int) string {
			return th.Tool(ToolView{ID: "s2", Name: "bash", Summary: "git push origin main", Class: "outward", Status: "running"}, w)
		}},
		{"tool-collapsed", func(w int) string {
			return th.Tool(ToolView{ID: "s3", Name: "read", Summary: "src/auth/login.go", Class: "read", Status: "done", Ms: 3, Output: strings.Join(many, "\n") + "\n" + long}, w)
		}},
		{"tool-expanded", func(w int) string {
			return th.Tool(ToolView{ID: "s3", Name: "read", Summary: "src/auth/login.go", Class: "read", Status: "done", Ms: 3, Output: strings.Join(many[:10], "\n"), Expand: true}, w)
		}},
		{"tool-failed", func(w int) string {
			return th.Tool(ToolView{ID: "s4", Name: "bash", Summary: "go vet ./...", Class: "read", Status: "failed", Ms: 800, Output: "vet: ./x.go:3:1: undefined: y\nexit status 2"}, w)
		}},
		{"tool-denied", func(w int) string {
			return th.Tool(ToolView{ID: "s5", Name: "bash", Summary: "git push origin main", Class: "outward", Status: "denied", Why: "the operator said: not before the tests pass"}, w)
		}},
		{"tool-edit-diff", func(w int) string {
			return th.Tool(ToolView{ID: "s6", Name: "edit", Summary: "src/auth/login.go", Class: "write", Status: "done", Ms: 12, Diff: &diff, Wrote: []string{"src/auth/login.go"}}, w)
		}},
		{"tool-write-diff-collapsed", func(w int) string {
			var lines []string
			for i := 1; i <= 40; i++ {
				lines = append(lines, fmt.Sprintf("line %d", i))
			}
			d := DiffText("", strings.Join(lines, "\n")+"\n", 2)
			return th.Tool(ToolView{ID: "s7", Name: "write", Summary: "src/new.go", Class: "write", Status: "done", Ms: 5, Diff: &d}, w)
		}},
		{"subagent-running", func(w int) string {
			return th.Subagent(SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "gemini/gemini-3.8-flash", State: "tool", Elapsed: 4 * time.Second, Ask: "findings on the login zone: land the token file"}, w)
		}},
		{"subagent-report", func(w int) string {
			return th.Subagent(SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "gemini/gemini-3.8-flash", State: "done", Elapsed: 9 * time.Second, Ask: "findings on the login zone",
				Report: &ReportView{Status: "PASS", Findings: []string{"src/auth/token.go:1 token ttl is 15m", "src/auth/login.go:12 the session cookie is httpOnly"}, Holes: []string{"src/auth/refresh.go: no test covers the refresh path"}, Unsaid: []string{"domain: the token TTL is 15m, set by the auth zone"}}}, w)
		}},
		{"subagent-failed", func(w int) string {
			return th.Subagent(SubagentView{Name: "domain-security", Rank: "domain", Role: "judge", Model: "mock/m", State: "done", Elapsed: 2 * time.Second, Failed: true,
				Report: &ReportView{Status: "FAIL — provider: HTTP 503", Holes: []string{"provider: HTTP 503 after 3 retries"}}}, w)
		}},
		// the slice-1 pulse states (N15/N16): the row's motion, frozen mid-frame by an explicit
		// Pulse — the goldens pin the geometry; the colour rides the ANSI shots
		{"subagent-pulse-thinking", func(w int) string {
			return th.Subagent(SubagentView{Name: "coord-agent-one", Rank: "coord", Role: "drafter", Model: "mock/m", State: "thinking", Elapsed: 28 * time.Second, Ask: "coordinate the files-panel slices"}, w,
				Pulse{Lane: th.lane("coord"), Phase: 6})
		}},
		{"subagent-pulse-tool", func(w int) string {
			return th.Subagent(SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "mock/m", State: "tool", Elapsed: 12 * time.Second, Ask: "land the token file"}, w,
				Pulse{Lane: th.lane("zone"), Phase: 3, Tool: "edit", Frames: []string{"⠋", "⠙", "⠹", "⠸"}})
		}},
		{"subagent-pulse-gate", func(w int) string {
			return th.Subagent(SubagentView{Name: "domain-security", Rank: "domain", Role: "judge", Model: "mock/m", State: "waiting on gate", Elapsed: 31 * time.Second, Ask: "verdict on the import"}, w,
				Pulse{Lane: th.lane("domain"), Phase: 9, Bob: 1.4})
		}},
		{"subagent-pulse-flash", func(w int) string {
			return th.Subagent(SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "mock/m", State: "done", Elapsed: 9 * time.Second, Ask: "land the token file",
				Report: &ReportView{Status: "PASS", Findings: []string{"src/auth/token.go:1 token ttl is 15m"}}}, w,
				Pulse{Lane: th.lane("zone"), Flash: 0.9})
		}},
		{"subagent-pulse-fail-flash", func(w int) string {
			return th.Subagent(SubagentView{Name: "domain-security", Rank: "domain", Role: "judge", Model: "mock/m", State: "done", Elapsed: 2 * time.Second, Failed: true,
				Report: &ReportView{Status: "FAIL — provider: HTTP 503", Holes: []string{"provider: HTTP 503 after 3 retries"}}}, w,
				Pulse{Lane: th.lane("domain"), Flash: 0.6})
		}},
		{"subagent-principles", func(w int) string {
			ic := th.Icon("agent-one", "domain", 0)
			return th.Subagent(SubagentView{Name: "domain-security", Rank: "domain", Role: "judge", Model: "mock/m", State: "thinking", Elapsed: 8 * time.Second, Ask: "verdict on the import: is the payload lawful"}, w,
				Pulse{Lane: th.lane("domain"), Phase: 4, Principles: [2]string{ic[0], ic[1]}})
		}},
		{"gate", func(w int) string { return th.Gate("n/a (no domain owners)", ".agent-one/log.md", w, "") }},
		// slice 2: the fixed left rail — vitals, the touch ledger, the roster (ride-width
		// tripwire and rendering pinned here)
		// the right rail's cards (third-of-window proportioning; the fixture rides 36 wide)
		{"rail", func(w int) string {
			return th.Rail(RailView{Workspace: "agent-one", Model: "kimi-3-conduit", CtxPct: 42, CtxKnown: true, CtxMax: "128k", Tokens: 15420, Cost: "$0.0123",
				Layers: []RailLayer{{Rank: "orchestrator", Tokens: 8210, Lane: th.lane("orchestrator")}, {Rank: "coord", Tokens: 2301, Lane: th.lane("coord")}, {Rank: "domain", Tokens: 3302, Lane: th.lane("domain")}, {Rank: "zone", Tokens: 1607, Lane: th.lane("zone")}},
				Files:  []RailFile{{Path: "tui/blocks.go", Agent: "zone-tui", Lane: th.lane("zone")}, {Path: "tui/model.go", Agent: "orchestrator", Lane: th.lane("orchestrator")}, {Path: "tui/legacy.go", Agent: "mystery"}},
				Subagents: []RailSubagent{
					{Name: "coord-agent-one", State: "thinking", Aged: "1m12s", Lane: th.lane("coord")},
					{Name: "domain-security", State: "done", Done: true, Failed: true},
					{Name: "zone-auth", State: "done", Done: true, Lane: th.lane("zone")},
				}}, 36)
		}},
		{"rail-plan", func(w int) string {
			return th.Rail(RailView{Workspace: "agent-one", Model: "kimi-3-conduit", CtxPct: 42, CtxKnown: true, Tokens: 15420, Cost: "$0.0123",
				Plan: &RailPlan{Agent: "coord-agent-one", Rank: "coord", Lane: th.lane("coord"), Items: []RailPlanItem{
					{Text: "Coordinate the files-panel slices", Status: "in_progress"},
					{Text: "Land the token file", Status: "done"},
					{Text: "Ask domain-db for the store slice", Status: "pending"},
					{Text: "Key ttl audit", Status: "blocked"},
				}},
				Files:     []RailFile{{Path: "tui/blocks.go", Lane: th.lane("zone")}},
				Subagents: []RailSubagent{{Name: "coord-agent-one", State: "thinking", Aged: "1m12s", Lane: th.lane("coord")}},
			}, 36)
		}},
		{"rail-mascot", func(w int) string {
			art, _ := Mascot("agent-one")
			return th.Rail(RailView{Mascot: art, Workspace: "agent-one", Model: "kimi-3-conduit", CtxPct: 87, CtxKnown: true, Stress: true, Zone: "NEAR", Tokens: 15420,
				Files:     []RailFile{{Path: "tui/blocks.go", Lane: th.lane("zone")}},
				Subagents: []RailSubagent{{Name: "coord-agent-one", State: "thinking", Aged: "1m12s", Lane: th.lane("coord")}, {Name: "zone-auth", State: "done", Done: true, Lane: th.lane("zone")}},
			}, 36)
		}},
		{"rail-empty", func(w int) string {
			return th.Rail(RailView{Workspace: "agent-one", Model: "mock/m"}, 36)
		}},
		{"gate-fail", func(w int) string {
			return th.Gate("fail: src/api/rogue.go is outside zone-auth's domain", ".agent-one/log.md", w, "")
		}},
		{"holes", func(w int) string {
			return th.Holes([]string{"s3: the tree could not be stamped — shell writes go unseen", "report carries no @U line — nothing unsaid, or the duty failed (the dispatcher may ask)"}, w)
		}},
		{"approval", func(w int) string {
			return th.Choice(ChoiceView{Title: "Approval", Class: "outward", Tool: "bash", Summary: "git push origin main", Why: "git push reaches beyond the workspace (outward): it needs your word",
				Options: []string{"Yes", "Yes, and don't ask again for bash:git push*", "No, and tell the model why"},
				Notes:   []string{"", "writes a rule to .agent-one/config.local.yaml", ""}, Cursor: 0}, w)
		}},
		{"approval-typing", func(w int) string {
			return th.Choice(ChoiceView{Title: "Approval", Class: "outward", Tool: "bash", Summary: "git push origin main", Why: "outward",
				Options: []string{"Yes", "Yes, and don't ask again for bash:git push*", "No, and tell the model why"}, Cursor: 2, Typing: true, Prompt: "why not?", Typed: "run the tests first"}, w)
		}},
		{"question", func(w int) string {
			return th.Choice(ChoiceView{Title: "Question", Why: "Which storage should the session index use?", Options: []string{"sqlite", "jsonl"}, Cursor: 1, Free: true}, w)
		}},
		{"spinner", func(w int) string { return th.Spinner("⠋", "Running bash…", 3*time.Second, 12_345, w) }},
		{"footer", func(w int) string {
			return th.Footer(FooterView{Model: "gemini-3.8-flash", Ctx: "ctx 12%", Cost: "$0.0123", Live: 1, Workspace: "~/kaginari/pfcli", Hint: "? for shortcuts · ctrl+p plans"}, w)
		}},
		{"footer-fresh", func(w int) string {
			return th.Footer(FooterView{Model: "gemini-3.8-flash", Ctx: "ctx —", Cost: "no calls yet", Workspace: "~/kaginari/pfcli", Hint: "? for shortcuts · ctrl+p plans", Queued: 2}, w)
		}},
		{"menu", func(w int) string {
			return th.Menu([]MenuItem{{"agents", "the live subagent"}, {"status", "the honesty rule and the instrument board"}, {"send", "/send <agent> <text> — a line for a running agent"}, {"review", "Review $ARGUMENTS carefully (.claude/commands)"}}, 1, w)
		}},
		{"completions", func(w int) string {
			return th.Completions([]string{"src/auth/login.go", "src/auth/token.go", "src/api/server.go"}, 0, w)
		}},
		{"shortcuts", func(w int) string { return th.Shortcuts(DefaultShortcuts(), w) }},
		{"queued", func(w int) string { return th.Queued("also check the api zone", w) }},
		{"input", func(w int) string { return th.InputBox("› write a test for the login zone\n  ", w, false) }},
	}
	dir := filepath.Join("testdata", "golden")
	if *update {
		_ = os.MkdirAll(dir, 0o755)
	}
	for _, c := range cases {
		for _, w := range []int{80, 120} {
			name := fmt.Sprintf("%s-%d", c.name, w)
			t.Run(name, func(t *testing.T) {
				got := ansi.Strip(c.f(w))
				for i, l := range strings.Split(got, "\n") {
					if ansi.StringWidth(l) > w {
						t.Errorf("line %d wider than %d: %q", i+1, w, l)
					}
				}
				p := filepath.Join(dir, name+".txt")
				if *update {
					if err := os.WriteFile(p, []byte(got+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(p)
				if err != nil {
					t.Fatalf("no golden %s (run with -update): %v", p, err)
				}
				if strings.TrimRight(string(want), "\n") != got {
					t.Errorf("%s differs from its golden:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
				}
			})
		}
	}
}

// The slice-4 principles geometry: a live row shifts its card right of the two-row sprite, and a
// window too narrow drops the principles whole rather than starving the card.
func TestSubagentPrinciplesGeometry(t *testing.T) {
	th := NewTheme(true)
	ic := th.Icon("agent-one", "zone", 0)
	v := SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Elapsed: 5 * time.Second, Ask: "watch"}
	live := th.Subagent(v, 80, Pulse{Lane: th.lane("zone"), Phase: 4, Principles: [2]string{ic[0], ic[1]}})
	sl := strings.Split(ansi.Strip(live), "\n")
	if len(sl) != 2 || !strings.Contains(sl[0], "◆ subagent zone-auth") {
		t.Fatalf("topped head wrong: %q", ansi.Strip(live))
	}
	if !strings.Contains(sl[1], "└ watch") || strings.Contains(sl[1], "◆ subagent") {
		t.Fatalf("the ask row must ride beside the principles's second half: %q", sl[1])
	}
	narrow := th.Subagent(v, 30, Pulse{Lane: th.lane("zone"), Phase: 4, Principles: [2]string{ic[0], ic[1]}})
	for _, l := range strings.Split(ansi.Strip(narrow), "\n") {
		if ansi.StringWidth(l) > 30 {
			t.Fatalf("the principles must not push the card past 30: %q", l)
		}
	}
}

func TestDiff(t *testing.T) {
	d := DiffText("a\nb\nc\n", "a\nB\nc\nd\n", 1)
	if d.Add != 2 || d.Del != 1 {
		t.Fatalf("stat: %+v", d)
	}
	var kinds []byte
	for _, l := range d.Lines {
		kinds = append(kinds, l.Kind)
	}
	if string(kinds) != " -+ +" {
		t.Fatalf("kinds %q", kinds)
	}
	if d.Lines[1].Old != 2 || d.Lines[2].New != 2 || d.Lines[4].New != 4 {
		t.Fatalf("numbers: %+v", d.Lines)
	}
	// a gap between two far hunks
	var a, b []string
	for i := 0; i < 30; i++ {
		a = append(a, itoa(i))
		b = append(b, itoa(i))
	}
	b[2], b[27] = "x", "y"
	d = DiffText(strings.Join(a, "\n"), strings.Join(b, "\n"), 2)
	gaps := 0
	for _, l := range d.Lines {
		if l.Kind == '~' {
			gaps++
		}
	}
	if gaps != 1 || d.Add != 2 || d.Del != 2 {
		t.Fatalf("hunks: gaps %d %+v", gaps, d)
	}
	if !DiffText(strings.Repeat("x\n", diffMaxLines+1), "", 1).Truncated {
		t.Fatal("oversize input is not marked truncated")
	}
}

// Lip Gloss v2 styles in full color; the program's writer downsamples to the terminal's profile.
// On an ASCII profile no color survives the writer.
func TestNoColorIsPlain(t *testing.T) {
	th := NewTheme(true)
	th.Color = false
	out := th.Tool(ToolView{Name: "bash", Summary: "ls", Class: "read", Status: "done", Ms: 1, Output: "a\nb"}, 80)
	var buf strings.Builder
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.Ascii}
	if _, err := w.WriteString(out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "38;") || strings.Contains(buf.String(), "48;") {
		t.Fatalf("a color reached an ASCII terminal: %q", buf.String())
	}
	if !strings.Contains(ansi.Strip(out), "● bash  ls  read") {
		t.Fatalf("header: %q", ansi.Strip(out))
	}
}

func TestMarkdownFallsBackAndTrims(t *testing.T) {
	th := NewTheme(true)
	out := ansi.Strip(th.Markdown("hello **workspace**\n\n- a\n- b\n", 40))
	if !strings.Contains(out, "hello") || strings.HasPrefix(out, "\n") || strings.HasSuffix(out, "\n") {
		t.Fatalf("markdown: %q", out)
	}
	if th.Markdown("   ", 40) != "" {
		t.Fatal("blank markdown renders something")
	}
}

// The branded glamour config re-paints the stock palette into the theme's roles: headings and
// links hold the primary, inline code the human's magenta, strong text is weight (no colour),
// and code blocks rest on the panel ground. no-tricks policy: this test fails loudly the day
// someone brands with a colour the theme doesn't already carry.
func TestGlamourBrandRidesThePalette(t *testing.T) {
	th := NewTheme(true)
	th.Color = true // the suite runs without a tty; brand asserts still matter
	sc := th.styleConfig()
	if got, want := *sc.Heading.Color, hexOf(th.accent.GetForeground()); got != want {
		t.Fatalf("headings hold %s, want the accent's %s", got, want)
	}
	if got, want := *sc.Link.Color, hexOf(th.accent.GetForeground()); got != want {
		t.Fatalf("links hold %s, want the primary %s", got, want)
	}
	if got, want := *sc.Code.Color, hexOf(th.magenta.GetForeground()); got != want {
		t.Fatalf("inline code holds %s, want the magenta %s", got, want)
	}
	if sc.Strong.Color != nil || sc.Strong.Bold == nil || !*sc.Strong.Bold {
		t.Fatal("strong text is weight, not colour")
	}
	if sc.CodeBlock.BackgroundColor == nil {
		t.Fatal("code blocks must rest on the pane's background")
	}
	// NO_COLOR drops every escape; a plain run renders plain
	notty := Theme{Dark: true, Color: false}
	out := notty.Markdown("**bold**\n[link](https://x)", 40)
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("notty markdown leaked colour")
	}
}
