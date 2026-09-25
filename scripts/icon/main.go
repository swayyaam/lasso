// Command icon prepares an app icon that follows Apple's macOS template.
//
// macOS app icons are not edge-to-edge: the artwork occupies roughly 824px of
// a 1024x1024 canvas, and the surrounding transparency is what makes every
// icon in the Dock line up at the same visual size with the same corner
// radius. Artwork that fills the canvas renders larger than its neighbours.
//
// Source artwork is fitted, never stretched: a non-square image keeps its
// aspect ratio and is centred, so nothing is distorted to make it square.
//
//	go run . -in artwork.png -out appicon.png
//
// With -ring it draws Lasso's own mark instead of fitting a picture; see
// ring.go.
//
//	go run . -ring -out appicon.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"

	xdraw "golang.org/x/image/draw"
)

func main() {
	in := flag.String("in", "", "source artwork (PNG or JPEG)")
	out := flag.String("out", "", "destination PNG")
	canvas := flag.Int("canvas", 1024, "output canvas size")
	content := flag.Int("content", 824, "artwork size inside the canvas")
	ring := flag.Bool("ring", false, "draw Lasso's mark instead of fitting -in")
	band := flag.Float64("band", ringBandRatio, "with -ring, the band's width as a fraction of the tile")
	flag.Parse()

	if (*in == "" && !*ring) || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: icon -in artwork.png -out appicon.png")
		fmt.Fprintln(os.Stderr, "       icon -ring -out appicon.png")
		os.Exit(2)
	}
	if *content > *canvas {
		fmt.Fprintln(os.Stderr, "content must fit inside the canvas")
		os.Exit(2)
	}

	if *ring {
		write(*out, drawRing(*canvas, *content, *band))
		fmt.Printf("%s: the mark on a %dx%d tile, centred on a %dx%d transparent canvas\n",
			*out, *content, *content, *canvas, *canvas)
		return
	}

	src, err := load(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading %s: %v\n", *in, err)
		os.Exit(1)
	}

	b := src.Bounds()
	// Fit rather than fill: scale by the longer side so the whole image is
	// inside the content box, leaving the aspect ratio alone.
	scale := float64(*content) / float64(max(b.Dx(), b.Dy()))
	w := int(float64(b.Dx())*scale + 0.5)
	h := int(float64(b.Dy())*scale + 0.5)

	dst := image.NewRGBA(image.Rect(0, 0, *canvas, *canvas))
	// Transparent, not white: the canvas margin must not show as a box.
	draw.Draw(dst, dst.Bounds(), image.Transparent, image.Point{}, draw.Src)

	offX := (*canvas - w) / 2
	offY := (*canvas - h) / 2
	target := image.Rect(offX, offY, offX+w, offY+h)
	xdraw.CatmullRom.Scale(dst, target, src, b, xdraw.Over, nil)
	write(*out, dst)

	fmt.Printf("%s: %dx%d artwork centred on a %dx%d transparent canvas\n",
		*out, w, h, *canvas, *canvas)
	if b.Dx() != b.Dy() {
		fmt.Printf("  source was %dx%d (not square) — fitted, not stretched\n", b.Dx(), b.Dy())
	}
}

func write(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating %s: %v\n", path, err)
		os.Exit(1)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", path, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", path, err)
		os.Exit(1)
	}
}

func load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}
