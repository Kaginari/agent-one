package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// initReport is `init`'s whole answer, gathered once: a terminal renders it styled
// (renderInit), pipes and bench read the wire form printed by cmdInit — byte-stable,
// tests and scripts depend on it.
type initReport struct {
	dist, root, level string
	made              []string // scaffold writes from Init
	setupPath         string   // config written by the setup step ("" when none/skipped)
	boardPort         int      // the board port the written config chose (0 when nothing written)
	medium            *mediumRep
	bench             bool
	housekept         int // chain-policy repairs sealed at found (0 = whole at provisioning)
	leftover          int // findings still wanting an operator
}

type mediumRep struct {
	members []plan
	wrote   []string
	kept    int
}

// initWords is the workspace's voice for the init face.
type initWords struct {
	header       string
	scaffoldNote string
	teamWord     string
	keptWord     string
	complexNote  string
	footer       string
}

func defaultInitWords() initWords {
	return initWords{
		header:       "founding a workspace at",
		scaffoldNote: "the policy · the instruments · memory tiers",
		teamWord:     "a team sketched from the tree",
		keptWord:     "kept as they were",
		complexNote:  "the founding session starts: the model reads all the code and founds the team",
		footer:       "human gate on · guts on",
	}
}

// styledInit answers whether Out deserves the formatted face: a real terminal with colour.
// The same check the tui's palette uses (tui/theme.go) — lipgloss itself strips the escapes
// on an ASCII profile, but the wire form is the better pipe answer, so pipes never render.
func styledInit(w io.Writer, plain bool) bool {
	if plain {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	p := colorprofile.Detect(f, os.Environ())
	return p != colorprofile.Ascii && p != colorprofile.NoTTY
}

// initStyles mirrors the tui dark palette (the board tokens in tui/theme.go). A light-bg
// variant comes when the tui theme exposes one for commands.
type initStyles struct {
	title, section, okGlow, dim, count, file lipgloss.Style
	rank                                     map[string]lipgloss.Style
}

func newInitStyles() initStyles {
	c := lipgloss.Color
	return initStyles{
		title:   lipgloss.NewStyle().Foreground(c("#d4af37")).Bold(true),
		section: lipgloss.NewStyle().Foreground(c("#9fb0c3")).Bold(true),
		okGlow:  lipgloss.NewStyle().Foreground(c("#5fbf7a")).Bold(true),
		dim:     lipgloss.NewStyle().Foreground(c("#7f8a99")),
		count:   lipgloss.NewStyle().Foreground(c("#bd8c24")),
		file:    lipgloss.NewStyle().Foreground(c("#9fb0c3")),
		rank: map[string]lipgloss.Style{
			"coord":   lipgloss.NewStyle().Foreground(c("#bd8c24")),
			"domain":  lipgloss.NewStyle().Foreground(c("#7c5cd6")),
			"zone":    lipgloss.NewStyle().Foreground(c("#2695bd")),
			"service": lipgloss.NewStyle().Foreground(c("#c53d34")),
		},
	}
}

// renderInit draws the init answer for a terminal: sectioned, spelled out, no wire tags.
func renderInit(w io.Writer, r initReport, words initWords) {
	st := newInitStyles()
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n\n", st.title.Render("◆ "+r.dist), st.dim.Render("· "+words.header+" "+r.root))
	writeInitSection(&b, st, "scaffold", words.scaffoldNote, r.made, len(r.made), "already founded — nothing touched")
	if r.setupPath != "" {
		writeInitSection(&b, st, "setup", fmt.Sprintf("config written · board on http://127.0.0.1:%d", r.boardPort), []string{r.setupPath}, 1, "")
	} else {
		b.WriteString(st.dim.Render("  board → http://127.0.0.1:7411 · the schema default (no workspace config written; set ui.board.port to move it)") + "\n")
	}
	if r.medium != nil {
		m := r.medium
		head := st.okGlow.Render("  ✓ ") + st.section.Render(words.teamWord) + st.dim.Render("  ·  ") +
			st.count.Render(fmt.Sprintf("%d written", len(m.wrote)))
		if m.kept > 0 {
			head += st.dim.Render(fmt.Sprintf(" · %d %s", m.kept, words.keptWord))
		}
		b.WriteString(head + "\n")
		for _, p := range m.members {
			rank := st.rank[p.Rank]
			b.WriteString(rank.Render("    "+padTo(p.Rank, 8)+" "+p.Short) + st.dim.Render("  → "+p.Parent))
			if len(p.Ownership) > 0 {
				b.WriteString(st.dim.Render("  · " + strings.Join(p.Ownership, " ")))
			}
			b.WriteString("\n")
		}
	}
	if r.level == "complex" {
		b.WriteString(st.okGlow.Render("  ✦ ") + st.section.Render("founding session") + st.dim.Render("  "+words.complexNote) + "\n")
	}
	if r.housekept > 0 || r.leftover > 0 {
		b.WriteString(st.okGlow.Render("  ✓ ") + st.section.Render("housekeep") + st.dim.Render(fmt.Sprintf("  · %d sealed by the chain policy · %d want an operator", r.housekept, r.leftover)) + "\n")
	}
	if r.bench {
		b.WriteString(st.dim.Render("  · bench: config from "+strings.ToUpper(strings.ReplaceAll(r.dist, "-", "_"))+"_CONFIG_CONTENT; the gate needs a TTY or a file-layer pre-approval") + "\n")
	}
	fmt.Fprintf(&b, "\n%s\n  %s %s\n", st.dim.Render("── next ──────────────────────────"), st.section.Render(r.dist+" run \"<ask>\""), st.dim.Render("· "+words.footer))
	fmt.Fprint(w, b.String())
}

func writeInitSection(b *strings.Builder, st initStyles, name, note string, files []string, n int, emptyNote string) {
	fmt.Fprintf(b, "%s%s", st.okGlow.Render("  ✓ "), st.section.Render(name))
	if note != "" {
		b.WriteString(st.dim.Render("  ·  " + note))
	}
	if n > 0 {
		b.WriteString(st.count.Render(fmt.Sprintf("  %d written", n)) + "\n")
		for _, m := range files {
			b.WriteString(st.file.Render("    + "+m) + "\n")
		}
	} else if emptyNote != "" {
		b.WriteString(st.dim.Render("  ·  "+emptyNote) + "\n")
	} else {
		b.WriteString("\n")
	}
}

// padTo right-pads a word with spaces to at least n columns (ASCII-only content here).
func padTo(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}
