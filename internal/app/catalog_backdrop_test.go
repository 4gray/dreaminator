package app

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestShadeBackdropFadesIntoBackground(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 780, 439))
	for i := range src.Pix {
		src.Pix[i] = 230 // a bright picture
	}
	var in bytes.Buffer
	jpeg.Encode(&in, src, &jpeg.Options{Quality: 90})
	art, err := shadeBackdrop(in.Bytes())
	if err != nil || art.ContentType != "image/jpeg" {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(bytes.NewReader(art.Data))
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds() != src.Bounds() {
		t.Fatalf("size changed: %v", out.Bounds())
	}
	luma := func(x, y int) int {
		r, g, b, _ := out.At(x, y).RGBA()
		return int(r+g+b) / 3 >> 8
	}
	// Left and bottom edges meet the screen background; the picture stays visible.
	if l := luma(2, 100); l > 30 {
		t.Fatalf("left edge too bright: %d", l)
	}
	if l := luma(700, 437); l > 30 {
		t.Fatalf("bottom edge too bright: %d", l)
	}
	if l := luma(770, 100); l < 60 || l > 120 {
		t.Fatalf("picture should be dimmed but visible: %d", l)
	}
}

func TestShadeBackdropRejectsOtherPictures(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 780, 439)))
	if _, err := shadeBackdrop(b.Bytes()); err == nil {
		t.Fatal("PNG accepted")
	}
	big := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	big.Set(0, 0, color.White)
	b.Reset()
	jpeg.Encode(&b, big, nil)
	if _, err := shadeBackdrop(b.Bytes()); err == nil {
		t.Fatal("oversized backdrop accepted: the receiver must not decode it")
	}
}
