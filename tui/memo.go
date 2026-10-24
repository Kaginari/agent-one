package tui

import "sync"

// rowMemo is the frame's content-keyed paint cache: a row's painted text (its plane, the
// sidebar's background, a rule on the ground) keyed by what it shows, its kind and its width.
// Most rows of a frame are the rows of the last one — an idle redraw, a spinner frame — so a
// frame pays the paint only for the rows that changed. A theme swap (renderGen) empties it; it
// is emptied too once it holds a few windows of rows, so a long scroll never grows it.
type rowMemo struct {
	gen        int
	m          map[rowKey]string
	hits, miss int
}

type rowKey struct {
	kind byte // 'c' a chat row, 's' a sidebar row, 'r' a sidebar rule, 'h' a frame rule, 'g' a frame glyph
	w    int
	s    string
}

// memoRow is paint() for the row (kind, w, s), from the memo when this theme painted it.
func (m *Model) memoRow(kind byte, w int, s string, paint func() string) string {
	r := &m.rowPaint
	if r.m == nil || r.gen != m.renderGen || len(r.m) > 8*max(m.height, 24) {
		r.m, r.gen = make(map[rowKey]string, 4*max(m.height, 24)), m.renderGen
	}
	k := rowKey{kind, w, s}
	if out, ok := r.m[k]; ok {
		r.hits++
		return out
	}
	r.miss++
	out := paint()
	r.m[k] = out
	return out
}

// mascotMemo holds the sprite's two faces per distribution: the art never changes, so its
// half-block render is drawn once, not every frame the sidebar shows it.
var mascotMemo sync.Map // mascotKey → mascotFace

type mascotKey struct {
	dist   string
	closed bool
}

type mascotFace struct {
	rows    []string
	tagline string
}

// mascotArtMemo is mascotArt, drawn once per distribution and eye.
func mascotArtMemo(dist string, closed bool) ([]string, string) {
	k := mascotKey{dist, closed}
	if f, ok := mascotMemo.Load(k); ok {
		face := f.(mascotFace)
		return face.rows, face.tagline
	}
	rows, tag := mascotArt(dist, closed)
	mascotMemo.Store(k, mascotFace{rows, tag})
	return rows, tag
}
