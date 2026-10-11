package tui

import (
	"image/color"
	"strings"
)

// gauge is a horizontal bar exactly cells wide, filled to frac (0..1) in hue over a quiet
// track: under the truecolor depth a slim half-row bar, flat a heavy ━ fill over the border
// tone's ─. hueAt (nil: hue) colours the fill by how far along a cell sits — the
// context meter's ramp.
func (t Theme) gauge(frac float64, cells int, hue color.Color, hueAt func(x float64) color.Color) string {
	if cells <= 0 {
		return ""
	}
	frac = min(max(frac, 0), 1)
	if hue == nil {
		hue = t.accent.GetForeground()
	}
	at := func(i int) color.Color {
		if hueAt != nil {
			return hueAt(float64(i) / float64(max(cells-1, 1)))
		}
		return hue
	}
	if !t.Color {
		n := int(frac*float64(cells) + 0.5)
		return strings.Repeat("━", n) + strings.Repeat("─", cells-n)
	}
	if !t.Depth {
		n := int(frac*float64(cells) + 0.5)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(gl("━", at(i), nil))
		}
		b.WriteString(t.border.Render(strings.Repeat("─", cells-n)))
		return b.String()
	}
	// under the depth: a slim bar on the lower half of its row, half-cell precise — the fill in
	// the hue, the track a tint of it; the cell's upper half stays the surface's own face
	track := t.dep.card.mix(toRGB(hue), 0.3)
	if !t.Dark {
		track = t.dep.card.mix(toRGB(hue), 0.22)
	}
	halves := int(frac*float64(cells*2) + 0.5)
	full, half := halves/2, halves%2
	var b strings.Builder
	for i := 0; i < full; i++ {
		b.WriteString(gl("▄", at(i), nil))
	}
	if half > 0 && full < cells {
		// the half cell: its left quadrant filled — the bar's rounded end
		b.WriteString(gl("▖", at(full), nil))
		full++
	}
	if full < cells {
		b.WriteString(gls("▄", cells-full, track.col(), nil))
	}
	return b.String()
}

// ctxHue is the context meter's colour at a point along it: calm, then warning, then danger.
func (t Theme) ctxHue(x float64) color.Color {
	ok, warn, bad := toRGB(t.ok.GetForeground()), toRGB(t.warn.GetForeground()), toRGB(t.bad.GetForeground())
	if x < 0.6 {
		return ok.col()
	}
	if x < 0.85 {
		return ok.mix(warn, (x-0.6)/0.25).col()
	}
	return warn.mix(bad, min((x-0.85)/0.15, 1)).col()
}

// bar is the thin scrollbar of a pane rows tall over content total rows long, its window top
// rows from the content's start: the track's and the thumb's glyph for each row — nil when the
// content fits. The thumb is sized to the visible fraction (never under one row) and placed at
// half-row precision: a thumb starting or ending mid-cell holds the quadrant (▗ ▝) on that row.
func scrollThumb(rows, total, top int) []rune {
	if rows <= 0 || total <= rows {
		return nil
	}
	h2 := rows * 2 // half rows
	size := max(h2*rows/total, 2)
	maxTop := max(total-rows, 1)
	start := (h2 - size) * min(max(top, 0), maxTop) / maxTop
	end := start + size
	out := make([]rune, rows)
	for i := range out {
		up, lo := i*2, i*2+1
		u := up >= start && up < end
		l := lo >= start && lo < end
		switch {
		case u && l:
			out[i] = '▐'
		case l:
			out[i] = '▗'
		case u:
			out[i] = '▝'
		default:
			out[i] = '▕'
		}
	}
	return out
}

// scrollGlyph draws one row of a scrollbar: the thumb's part in the primary over the plane, the
// track a quiet eighth-line in the border tone.
func (t Theme) scrollGlyph(r rune, bg color.Color) string {
	if !t.Color {
		return string(r)
	}
	if r == '▕' {
		return gl("▕", toRGB(t.border.GetForeground()).mix(toRGB(t.dim.GetForeground()), 0.3).col(), bg)
	}
	thumb := t.accent.GetForeground()
	if t.Depth {
		thumb = toRGB(thumb).mix(t.dep.plane, 0.25).col()
	}
	return gl(string(r), thumb, bg)
}
