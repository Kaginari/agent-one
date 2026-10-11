package tui

import (
	"strings"
)

// Small-caps titles: one line, drawn from the Unicode small-capital runes — a designed-looking
// face at single-line weight, no pixel walls. Runes the map misses (digits, ·, anything else)
// pass through as-is; the title reads.
//
// The pixel VHS rig from tonight sits retired: it was two text rows of glyph thickness and the
// rail said it read heavy. One honest line of small caps is the better font at this scale.
var smallCaps = map[rune]rune{
	'a': 'ᴀ', 'b': 'ʙ', 'c': 'ᴄ', 'd': 'ᴅ', 'e': 'ᴇ', 'f': 'ꜰ', 'g': 'ɢ', 'h': 'ʜ', 'i': 'ɪ',
	'j': 'ᴊ', 'k': 'ᴋ', 'l': 'ʟ', 'm': 'ᴍ', 'n': 'ɴ', 'o': 'ᴏ', 'p': 'ᴘ', 'q': 'ǫ', 'r': 'ʀ',
	's': 'ꜱ', 't': 'ᴛ', 'u': 'ᴜ', 'v': 'ᴠ', 'w': 'ᴡ', 'x': 'x', 'y': 'ʏ', 'z': 'ᴢ',
}

// smallCap renders s with small-capital letters.
func smallCap(s string) string {
	var b strings.Builder
	for _, c := range s {
		if r, ok := smallCaps[c]; ok {
			b.WriteRune(r)
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}
