package main

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// The mark: the five category accents as one ring on the near-black tile.
// Drawn from geometry rather than scaled from a picture, so every size is
// sharp and the colours are the token values exactly.

// Clockwise from the top, starting with the segment centred there. These are
// the accent stops in packages/ui/src/tokens.css; change them together.
var ringColours = []color.RGBA{
	{0x7a, 0x3d, 0xff, 0xff}, // accent-purple
	{0xed, 0x52, 0xcb, 0xff}, // accent-pink
	{0x3b, 0x89, 0xff, 0xff}, // accent-blue
	{0xff, 0x6b, 0x00, 0xff}, // accent-orange
	{0x00, 0xd7, 0x22, 0xff}, // accent-green
}

// tile is #080808, the near-black the system is built on: light's primary
// and dark's canvas.
var tile = color.RGBA{0x08, 0x08, 0x08, 0xff}

// Proportions of the tile's width.
const (
	// Apple's template: 185.4px on an 824px tile, which is what makes the
	// icon share the corner radius of its neighbours in the Dock.
	cornerRatio = 0.225
	// The ring's outer edge, and the width of its band.
	ringOuterRatio = 0.384
	ringBandRatio  = 0.109
)

// drawRing renders the icon onto a transparent canvas, the tile content
// pixels wide and centred.
func drawRing(canvas, content int, band float64) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, canvas, canvas))
	size := float64(content)
	x0 := float64(canvas-content) / 2
	cx, cy := float64(canvas)/2, float64(canvas)/2

	fill := func(c color.RGBA, path func(p *pen)) {
		r := vector.NewRasterizer(canvas, canvas)
		path(&pen{r})
		r.ClosePath()
		r.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
	}

	// The tile.
	fill(tile, func(p *pen) {
		rc, x1 := cornerRatio*size, x0+size
		y0, y1 := x0, x0+size
		p.move(x0+rc, y0)
		p.line(x1-rc, y0)
		p.arc(x1-rc, y0+rc, rc, 0, 90)
		p.line(x1, y1-rc)
		p.arc(x1-rc, y1-rc, rc, 90, 180)
		p.line(x0+rc, y1)
		p.arc(x0+rc, y1-rc, rc, 180, 270)
		p.line(x0, y0+rc)
		p.arc(x0+rc, y0+rc, rc, 270, 360)
	})

	outer := ringOuterRatio * size
	inner := outer - band*size
	mid := (outer + inner) / 2
	step := 360.0 / float64(len(ringColours))
	start := -step / 2 // the first segment is centred on the top

	// The segments, butted end to end.
	for i, c := range ringColours {
		a0 := start + float64(i)*step
		a1 := a0 + step
		fill(c, func(p *pen) {
			p.move(polar(cx, cy, outer, a0))
			p.arc(cx, cy, outer, a0, a1)
			p.line(polar(cx, cy, inner, a1))
			p.arc(cx, cy, inner, a1, a0)
		})
	}

	// Each segment's rounded end, laid over the start of the next. A cap is a
	// disc as wide as the band, centred where the two meet, so it also covers
	// the seam the two butted edges would otherwise leave.
	for i, c := range ringColours {
		end := start + float64(i+1)*step
		x, y := polar(cx, cy, mid, end)
		fill(c, func(p *pen) {
			p.move(polar(x, y, (outer-inner)/2, 0))
			p.arc(x, y, (outer-inner)/2, 0, 360)
		})
	}
	return dst
}

// polar is the point at radius r and angle a, in degrees clockwise from the
// top — the way a clock face reads, which is how the ring is described.
func polar(cx, cy, r, a float64) (float64, float64) {
	rad := a * math.Pi / 180
	return cx + r*math.Sin(rad), cy - r*math.Cos(rad)
}

type pen struct{ r *vector.Rasterizer }

func (p *pen) move(x, y float64) { p.r.MoveTo(float32(x), float32(y)) }
func (p *pen) line(x, y float64) { p.r.LineTo(float32(x), float32(y)) }

// arc continues the path along a circle from angle a0 to a1, as cubic Béziers
// of at most 30° each, well inside the accuracy a pixel can show.
func (p *pen) arc(cx, cy, r, a0, a1 float64) {
	n := int(math.Ceil(math.Abs(a1-a0) / 30))
	delta := (a1 - a0) / float64(n)
	k := 4.0 / 3.0 * math.Tan(delta*math.Pi/180/4)
	for i := 0; i < n; i++ {
		s := (a0 + float64(i)*delta) * math.Pi / 180
		e := (a0 + float64(i+1)*delta) * math.Pi / 180
		sx, sy := cx+r*math.Sin(s), cy-r*math.Cos(s)
		ex, ey := cx+r*math.Sin(e), cy-r*math.Cos(e)
		p.r.CubeTo(
			float32(sx+k*r*math.Cos(s)), float32(sy+k*r*math.Sin(s)),
			float32(ex-k*r*math.Cos(e)), float32(ey-k*r*math.Sin(e)),
			float32(ex), float32(ey),
		)
	}
}
