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
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	_ "image/jpeg"
	"os"

	xdraw "golang.org/x/image/draw"
)

func main() {
	in := flag.String("in", "", "source artwork (PNG or JPEG)")
	out := flag.String("out", "", "destination PNG")
	canvas := flag.Int("canvas", 1024, "output canvas size")
	content := flag.Int("content", 824, "artwork size inside the canvas")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: icon -in artwork.png -out appicon.png")
		os.Exit(2)
	}
	if *content > *canvas {
		fmt.Fprintln(os.Stderr, "content must fit inside the canvas")
		os.Exit(2)
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

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating %s: %v\n", *out, err)
		os.Exit(1)
	}
	defer f.Close()

	if err := png.Encode(f, dst); err != nil {
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", *out, err)
		os.Exit(1)
	}

	fmt.Printf("%s: %dx%d artwork centred on a %dx%d transparent canvas\n",
		*out, w, h, *canvas, *canvas)
	if b.Dx() != b.Dy() {
		fmt.Printf("  source was %dx%d (not square) — fitted, not stretched\n", b.Dx(), b.Dy())
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
