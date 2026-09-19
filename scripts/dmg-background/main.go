// Command dmg-background draws the picture behind the install window.
//
// A DMG with no background is a grey Finder window with two icons dropped in
// the corner and a lot of nothing under them, which is the first thing anyone
// sees of the app. This draws the window it should be instead: the app on the
// left, Applications on the right, an arrow saying what to do with them, and
// the same white canvas and near-black ink the app itself uses.
//
// It renders at a given scale so the same geometry produces both the 1x and 2x
// images that make up the retina .tiff; make-dmg.sh combines them.
//
// Usage:
//
//	go run . -out background.png -scale 1
//	go run . -out background@2x.png -scale 2
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// The window, in points. Finder positions icons in this same space, so these
// numbers and the ones in make-dmg.sh have to agree.
const (
	windowW = 640
	windowH = 420

	// Icon centres. The app sits left of centre and Applications right of it,
	// which is the arrangement every macOS user already knows how to read.
	appX     = 172
	appsX    = 468
	iconY    = 182
	iconSize = 128

	// The arrow runs between the two icons, clear of both.
	arrowFromX = 258
	arrowToX   = 382
)

// Colours from DESIGN-webflow.md, which is also what the app is drawn with.
var (
	canvas    = color.RGBA{0xff, 0xff, 0xff, 0xff}
	surface   = color.RGBA{0xf5, 0xf5, 0xf5, 0xff}
	ink       = color.RGBA{0x08, 0x08, 0x08, 0xff}
	inkSubtle = color.RGBA{0x5a, 0x5a, 0x5a, 0xff}
	hairline  = color.RGBA{0xd8, 0xd8, 0xd8, 0xff}
)

// fontCandidates are tried in order. The first is what macOS itself draws with,
// so the window reads as part of the system rather than as a picture of one.
var fontCandidates = []string{
	"/System/Library/Fonts/SFNS.ttf",
	"/System/Library/Fonts/HelveticaNeue.ttc",
	"/System/Library/Fonts/Helvetica.ttc",
	"/System/Library/Fonts/Supplemental/Arial.ttf",
}

func main() {
	out := flag.String("out", "background.png", "file to write")
	scale := flag.Float64("scale", 1, "pixels per point: 1 for standard, 2 for retina")
	flag.Parse()

	if *scale <= 0 {
		log.Fatal("dmg-background: scale must be positive")
	}

	img := render(*scale)

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("dmg-background: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		log.Fatalf("dmg-background: %v", err)
	}
	fmt.Printf("wrote %s (%dx%d)\n", *out, img.Bounds().Dx(), img.Bounds().Dy())
}

func render(scale float64) image.Image {
	w := int(math.Round(windowW * scale))
	h := int(math.Round(windowH * scale))
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	drawBackdrop(img, scale)
	drawIconWells(img, scale)
	drawArrow(img, scale)
	drawText(img, scale)

	return img
}

// drawBackdrop lays down the canvas with a shallow vertical gradient.
//
// Flat white is correct per the design system but reads as unfinished at this
// size, where there is nothing else on screen. Two stops of the same neutral
// give the window a floor to sit on without introducing a colour.
func drawBackdrop(img *image.RGBA, scale float64) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		t := float64(y) / float64(bounds.Dy())
		// Ease so most of the panel stays canvas-white and the shading
		// gathers towards the bottom.
		t = t * t
		row := color.RGBA{
			R: lerp(canvas.R, surface.R, t),
			G: lerp(canvas.G, surface.G, t),
			B: lerp(canvas.B, surface.B, t),
			A: 0xff,
		}
		draw.Draw(img, image.Rect(bounds.Min.X, y, bounds.Max.X, y+1), &image.Uniform{row}, image.Point{}, draw.Src)
	}
}

// drawIconWells puts a soft rounded panel behind each icon.
//
// Finder draws the icons themselves; this is the shelf they stand on, which is
// what stops two unrelated images floating in an empty rectangle.
func drawIconWells(img *image.RGBA, scale float64) {
	const pad = 26
	size := float64(iconSize + pad*2)

	for _, cx := range []float64{appX, appsX} {
		well := rect{
			x: (cx - size/2) * scale,
			y: (iconY - size/2) * scale,
			w: size * scale,
			h: size * scale,
			r: 20 * scale,
		}
		// Barely-there: the well should be felt rather than seen, so a drop
		// target is suggested without putting a grey box behind the icons.
		//
		// Both colours go through withAlpha because image/draw works in
		// premultiplied alpha and does not clamp. A literal like
		// {0xff, 0xff, 0xff, 0xd0} has channels above its own alpha, which is
		// not a colour — the over operator overflows and it renders near-black.
		fillRoundRect(img, well, withAlpha(hairline, 0x26))
		strokeRoundRect(img, well, 1*scale, withAlpha(hairline, 0xb4))
	}
}

// drawArrow points from the app to the folder.
func drawArrow(img *image.RGBA, scale float64) {
	y := float64(iconY) * scale
	from := float64(arrowFromX) * scale
	to := float64(arrowToX) * scale
	thickness := 2.0 * scale

	// The shaft stops short of the head so the join is not doubled up.
	head := 11.0 * scale
	shaft := rect{x: from, y: y - thickness/2, w: (to - head) - from, h: thickness, r: thickness / 2}
	fillRoundRect(img, shaft, withAlpha(ink, 0x55))

	// A chevron rather than a filled triangle: lighter, and it matches the
	// stroked icons in the app.
	r := vector.NewRasterizer(img.Bounds().Dx(), img.Bounds().Dy())
	spread := 8.0 * scale
	strokeLine(r, to-head, y-spread, to, y, thickness)
	strokeLine(r, to-head, y+spread, to, y, thickness)
	r.Draw(img, img.Bounds(), &image.Uniform{withAlpha(ink, 0x55)}, image.Point{})
}

// drawText writes the instruction and a quiet line of provenance.
func drawText(img *image.RGBA, scale float64) {
	face, err := loadFace(15 * scale)
	if err != nil {
		// A background without the caption is still a good background, and
		// failing the whole build over a font would be out of proportion.
		log.Printf("dmg-background: no usable system font (%v); skipping the caption", err)
		return
	}
	defer face.Close()

	centreText(img, face, "Drag Lasso into your Applications folder", windowH*scale-84*scale, inkSubtle)

	small, err := loadFace(11.5 * scale)
	if err != nil {
		return
	}
	defer small.Close()
	centreText(img, small, "yt-dlp, ffmpeg and everything else is already inside", windowH*scale-58*scale, withAlpha(inkSubtle, 0xaa))
}

// centreText draws a single line centred horizontally on the given baseline.
func centreText(img *image.RGBA, face font.Face, text string, baseline float64, c color.Color) {
	width := font.MeasureString(face, text)
	x := (fixed.I(img.Bounds().Dx()) - width) / 2

	d := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{c},
		Face: face,
		Dot:  fixed.Point26_6{X: x, Y: fixed.Int26_6(baseline * 64)},
	}
	d.DrawString(text)
}

// loadFace opens the first system font that parses, at the given size.
func loadFace(size float64) (font.Face, error) {
	var lastErr error
	for _, path := range fontCandidates {
		raw, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}

		parsed, err := parseFont(raw)
		if err != nil {
			lastErr = err
			continue
		}

		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
			Size: size,
			DPI:  72,
			// Full hinting keeps small text crisp, which is the whole reason
			// the 1x image exists alongside the 2x one.
			Hinting: font.HintingFull,
		})
		if err != nil {
			lastErr = err
			continue
		}
		return face, nil
	}
	return nil, fmt.Errorf("no usable font: %w", lastErr)
}

// parseFont handles both plain fonts and the collections macOS ships.
func parseFont(raw []byte) (*sfnt.Font, error) {
	if f, err := sfnt.Parse(raw); err == nil {
		return f, nil
	}
	collection, err := sfnt.ParseCollection(raw)
	if err != nil {
		return nil, err
	}
	return collection.Font(0)
}

// ---- small drawing helpers -------------------------------------------

type rect struct{ x, y, w, h, r float64 }

// fillRoundRect fills a rounded rectangle.
func fillRoundRect(img *image.RGBA, b rect, c color.Color) {
	r := vector.NewRasterizer(img.Bounds().Dx(), img.Bounds().Dy())
	traceRoundRect(r, b)
	r.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{})
}

// strokeRoundRect draws a rounded rectangle outline, as the difference between
// the shape and a smaller copy of it.
func strokeRoundRect(img *image.RGBA, b rect, width float64, c color.Color) {
	outer := vector.NewRasterizer(img.Bounds().Dx(), img.Bounds().Dy())
	traceRoundRect(outer, b)
	traceRoundRectReversed(outer, rect{
		x: b.x + width, y: b.y + width,
		w: b.w - width*2, h: b.h - width*2,
		r: math.Max(0, b.r-width),
	})
	outer.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{})
}

func traceRoundRect(r *vector.Rasterizer, b rect) {
	x, y, w, h, rad := float32(b.x), float32(b.y), float32(b.w), float32(b.h), float32(b.r)
	r.MoveTo(x+rad, y)
	r.LineTo(x+w-rad, y)
	r.QuadTo(x+w, y, x+w, y+rad)
	r.LineTo(x+w, y+h-rad)
	r.QuadTo(x+w, y+h, x+w-rad, y+h)
	r.LineTo(x+rad, y+h)
	r.QuadTo(x, y+h, x, y+h-rad)
	r.LineTo(x, y+rad)
	r.QuadTo(x, y, x+rad, y)
	r.ClosePath()
}

// traceRoundRectReversed winds the other way, so it subtracts from the shape
// already on the rasterizer rather than adding to it.
func traceRoundRectReversed(r *vector.Rasterizer, b rect) {
	x, y, w, h, rad := float32(b.x), float32(b.y), float32(b.w), float32(b.h), float32(b.r)
	r.MoveTo(x+rad, y)
	r.QuadTo(x, y, x, y+rad)
	r.LineTo(x, y+h-rad)
	r.QuadTo(x, y+h, x+rad, y+h)
	r.LineTo(x+w-rad, y+h)
	r.QuadTo(x+w, y+h, x+w, y+h-rad)
	r.LineTo(x+w, y+rad)
	r.QuadTo(x+w, y, x+w-rad, y)
	r.ClosePath()
}

// strokeLine adds a round-capped line to a rasterizer.
func strokeLine(r *vector.Rasterizer, x1, y1, x2, y2, width float64) {
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	// The normal, scaled to half the stroke width.
	nx, ny := -dy/length*width/2, dx/length*width/2

	r.MoveTo(float32(x1+nx), float32(y1+ny))
	r.LineTo(float32(x2+nx), float32(y2+ny))
	r.LineTo(float32(x2-nx), float32(y2-ny))
	r.LineTo(float32(x1-nx), float32(y1-ny))
	r.ClosePath()
}

func lerp(from, to uint8, t float64) uint8 {
	return uint8(math.Round(float64(from) + (float64(to)-float64(from))*t))
}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	// Premultiplied, which is what image/draw expects.
	f := float64(a) / 255
	return color.RGBA{
		R: uint8(math.Round(float64(c.R) * f)),
		G: uint8(math.Round(float64(c.G) * f)),
		B: uint8(math.Round(float64(c.B) * f)),
		A: a,
	}
}
