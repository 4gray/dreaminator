package server

import (
	"bytes"
	"image/png"
	"testing"
)

func TestQRPNG(t *testing.T) {
	data, err := qrPNG("http://192.168.1.45:8088", 440)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w != h || w > 440 || w < 300 {
		t.Fatalf("qr is %dx%d, want a square close to 440", w, h)
	}
	// Corner pixel is the white quiet zone, so phones can read it on a dark TV skin.
	if r, g, b, _ := img.At(0, 0).RGBA(); r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Fatalf("corner is not white: %v", img.At(0, 0))
	}
}
