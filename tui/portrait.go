package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/blacktop/go-termimg"
	xdraw "golang.org/x/image/draw"
)

// Portrait is a row-rendered stack of quarter-blocking cells, ready to be padded and
// joined into a card.
type Portrait struct {
	Rows []string
}

// PortraitFromBytes decodes a PNG portrait, pastes it onto bg (the portraits ship a white
// card — without the composite the pane's background would bleed through as white), and
// rasterizes it through go-termimg's half-block pass (each cell = two pixel rows).
// cheight is cell height; the source is rescaled to (w, cheight*2) px.
func PortraitFromBytes(png []byte, w int, bg color.Color) (*Portrait, error) {
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return nil, fmt.Errorf("portrait: %w", err)
	}
	// crop the PNG card's picture-frame edge (a fixed 12% bezel) so the portrait print is
	// the member, not the frame; then knock out the card's actual background colour —
	// sampled off the crop's corners (the member never reaches them after the bezel).
	// Tolerance 26 covers the card's gradient bleed without cutting into the member's
	// own colour.
	sb := img.Bounds()
	crop := image.Rect(sb.Min.X+sb.Dx()*12/100, sb.Min.Y+sb.Dy()*12/100, sb.Max.X-sb.Dx()*12/100, sb.Max.Y-sb.Dy()*12/100)
	if si, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		img = si.SubImage(crop)
	}
	bg2 := sampleCorners(img)
	// the pane cell is ~1:2 (h:w); packing 2 px rows per cell row means a square source
	// wants w px of height so the rendered w-cell portrait comes out visually square
	cheight := w
	img2 := image.NewRGBA(image.Rect(0, 0, w, cheight))
	xdraw.BiLinear.Scale(img2, img2.Bounds(), img, img.Bounds(), draw.Over, nil)
	for y := 0; y < cheight; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img2.At(x, y).RGBA()
			c := []int{int(r >> 8), int(g >> 8), int(b >> 8)}
			if okSquare := (c[0]-bg2[0])*(c[0]-bg2[0])+(c[1]-bg2[1])*(c[1]-bg2[1])+(c[2]-bg2[2])*(c[2]-bg2[2]) < 26*26; okSquare {
				img2.Set(x, y, color.RGBAModel.Convert(bg))
			}
		}
	}

	s, err := termimg.New(img2).Protocol(termimg.Halfblocks).Width(w).Render()
	if err != nil {
		return nil, fmt.Errorf("portrait render: %w", err)
	}
	rows := strings.Split(strings.TrimRight(s, "\r\n"), "\r\n")
	if len(rows) == 0 {
		return nil, fmt.Errorf("portrait render: empty")
	}
	for i, r := range rows {
		if dw := w - lipgloss.Width(r); dw > 0 {
			rows[i] = r + strings.Repeat(" ", dw)
		}
	}
	return &Portrait{Rows: rows}, nil
}

// sampleCorners averages the rgb of the four corners of img after its bezel crop — the
// card's background colour the knockout pass removes (the member's silhouette never
// reaches a corner of its own card's crop).
func sampleCorners(img image.Image) [3]int {
	b := img.Bounds()
	var sr, sg, sb int
	for _, p := range [][2]int{{0, 0}, {b.Dx() - 1, 0}, {0, b.Dy() - 1}, {b.Dx() - 1, b.Dy() - 1}} {
		r, g, bb, _ := img.At(b.Min.X+p[0], b.Min.Y+p[1]).RGBA()
		sr, sg, sb = sr+int(r>>8), sg+int(g>>8), sb+int(bb>>8)
	}
	return [3]int{sr / 4, sg / 4, sb / 4}
}
