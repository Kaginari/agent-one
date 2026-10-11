package tui

import (
	"fmt"
	"image/color"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

var (
	mdMu    sync.Mutex
	mdCache = map[string]*glamour.TermRenderer{}
)

// Markdown renders an assistant answer at a width: Glamour with the theme's style (dark,
// light, or notty when the terminal has no colour), no document margin so the text sits
// flush with the blocks around it. A render failure falls back to the plain text.
func (t Theme) Markdown(md string, width int) string {
	md = textSanitize(strings.ReplaceAll(strings.TrimSpace(md), "\t", "    "))
	if md == "" {
		return ""
	}
	if width < 20 {
		width = 20
	}
	r, err := t.renderer(width)
	if err != nil {
		return wrap(md, width)
	}
	mdMu.Lock()
	out, err := r.Render(escapeAngles(md))
	mdMu.Unlock()
	if err != nil {
		return wrap(md, width)
	}
	// glamour pads every line to the width and wraps the document in blank lines
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// escapeAngles backslash-escapes < and > outside code spans and fences: glamour drops raw HTML,
// so `Vec<T>` read "Vec" and `providers.<name>.script` read "providers..script". A line's
// leading > markers (a quote) and autolinks (<https://…>) keep their meaning; indented code
// (4 spaces outside a list) is code, left alone.
func escapeAngles(md string) string {
	if !strings.ContainsAny(md, "<>") {
		return md
	}
	lines := strings.Split(md, "\n")
	fence, list, blank := "", false, true
	for i, l := range lines {
		t := strings.TrimLeft(l, " ")
		ind := len(l) - len(t)
		if fence != "" {
			if ind < 4 && strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if ind < 4 && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			fence = t[:3]
			for _, c := range t[3:] {
				if string(c) != fence[:1] {
					break
				}
				fence += fence[:1]
			}
			continue
		}
		switch {
		case t == "":
			blank = true
			continue
		case isListLine(l):
			list = true
		case ind == 0:
			list = false
		}
		if ind >= 4 && blank && !list {
			continue // an indented code block
		}
		blank = false
		lines[i] = l[:ind] + escapeLine(t)
	}
	return strings.Join(lines, "\n")
}

// escapeLine escapes one line's angles outside its code spans; the leading quote markers stay.
func escapeLine(l string) string {
	var b strings.Builder
	i := 0
	for i < len(l) && (l[i] == '>' || l[i] == ' ') {
		i++
	}
	b.WriteString(l[:i])
	for i < len(l) {
		c := l[i]
		switch {
		case c == '\\' && i+1 < len(l):
			b.WriteString(l[i : i+2])
			i += 2
			continue
		case c == '`':
			n := 1
			for i+n < len(l) && l[i+n] == '`' {
				n++
			}
			run := l[i : i+n]
			if end := strings.Index(l[i+n:], run); end >= 0 {
				b.WriteString(l[i : i+n+end+n])
				i += n + end + n
				continue
			}
			b.WriteString(run)
			i += n
			continue
		case c == '<' && autolink(l[i:]):
			end := strings.IndexByte(l[i:], '>')
			b.WriteString(l[i : i+end+1])
			i += end + 1
			continue
		case c == '<' || c == '>':
			b.WriteByte('\\')
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// autolink: <scheme://…> or <mailto:…> with no space before its closing >.
func autolink(s string) bool {
	end := strings.IndexByte(s, '>')
	if end < 0 || strings.ContainsAny(s[:end], " <") {
		return false
	}
	in := s[1:end]
	return strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") || strings.HasPrefix(in, "mailto:")
}

func (t Theme) renderer(width int) (*glamour.TermRenderer, error) {
	key := t.styleName() + ":" + itoa(width)
	mdMu.Lock()
	r, ok := mdCache[key]
	mdMu.Unlock()
	if ok {
		return r, nil
	}
	sc := t.styleConfig()
	r, err := glamour.NewTermRenderer(glamour.WithStyles(sc), glamour.WithWordWrap(width))
	if err != nil {
		return nil, err
	}
	mdMu.Lock()
	mdCache[key] = r
	mdMu.Unlock()
	return r, nil
}

func (t Theme) styleName() string {
	switch {
	case !t.Color:
		return "notty"
	case t.Dark:
		return "dark"
	}
	return "light"
}

func (t Theme) styleConfig() ansi.StyleConfig {
	var sc ansi.StyleConfig
	switch t.styleName() {
	case "notty":
		sc = styles.NoTTYStyleConfig
	case "light":
		sc = styles.LightStyleConfig
	default:
		sc = styles.DarkStyleConfig
	}
	zero := uint(0)
	sc.Document.Margin = &zero
	sc.Document.BlockPrefix = ""
	sc.Document.BlockSuffix = ""
	sc.CodeBlock.Margin = &zero
	if t.Color {
		sc = t.brandStyleConfig(sc)
	}
	return sc
}

// brandStyleConfig re-paints the stock dark/light config into the theme's roles: headings in
// the primary, strong text bold in the text tone (weight, not colour), emphasis italic, links
// in the primary underlined, inline code in the human's magenta (the answer's one second hue),
// code blocks resting on the panel ground, quotes sunk into the dim. Colours only, never a fake.
func (t Theme) brandStyleConfig(sc ansi.StyleConfig) ansi.StyleConfig {
	primary := hexOf(t.accent.GetForeground())
	code := hexOf(t.magenta.GetForeground())
	dimC := hexOf(t.dim.GetForeground())
	paneBgHex := hexOf(t.paneBg())
	yes := true

	sc.Heading.Color = &primary
	sc.Strong.Color = nil
	sc.Strong.Bold = &yes
	sc.Emph.Color = nil
	sc.Emph.Italic = &yes
	sc.Link.Color = &primary
	sc.Link.Underline = &yes
	sc.LinkText.Color = &primary
	sc.LinkText.Italic = nil
	sc.Code.Color = &code

	sc.BlockQuote.Color = &dimC
	sep := "▎ "
	sc.BlockQuote.IndentToken = &sep
	sc.CodeBlock.StyleBlock.BackgroundColor = &paneBgHex

	return sc
}

// hexOf renders a palette colour as glamour's "#rrggbb". A nil colour (a lane the rank
// doesn't have) yields a neutral grey rather than a bad escape.
func hexOf(c color.Color) string {
	if c == nil {
		return "#888888"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
