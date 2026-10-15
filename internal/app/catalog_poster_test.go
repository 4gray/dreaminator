package app

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encoded(t *testing.T, w, h int, asJPEG bool) []byte {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	src.Set(w/2, h/2, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	var err error
	if asJPEG {
		err = jpeg.Encode(&b, src, nil)
	} else {
		err = png.Encode(&b, src)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// The receiver decodes JPEG/PNG itself; Go must not re-encode them.
func TestArtworkPassesThroughDecodablePictures(t *testing.T) {
	for _, asJPEG := range []bool{true, false} {
		data := encoded(t, 800, 1200, asJPEG)
		art, err := prepareArtwork(data, 400, 600)
		if err != nil || !bytes.Equal(art.Data, data) {
			t.Fatalf("jpeg=%v: re-encoded or rejected: %v", asJPEG, err)
		}
		want := "image/png"
		if asJPEG {
			want = "image/jpeg"
		}
		if art.ContentType != want {
			t.Fatalf("content type %q", art.ContentType)
		}
	}
}

func TestArtworkShrinksOversizedPictures(t *testing.T) {
	data := encoded(t, 2100, 1800, false)
	art, err := prepareArtwork(data, 400, 600)
	if err != nil || art.ContentType != "image/jpeg" {
		t.Fatalf("shrink: %v %q", err, art.ContentType)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(art.Data))
	if err != nil || cfg.Width != 400 || cfg.Height != 342 {
		t.Fatalf("invalid artwork: %+v %v", cfg, err)
	}
	wide := shrink(image.NewRGBA(image.Rect(0, 0, 1000, 100)), 400, 600)
	if wide.Bounds().Dx() != 400 || wide.Bounds().Dy() != 40 {
		t.Fatalf("aspect lost: %v", wide.Bounds())
	}
	if _, err := prepareArtwork(encoded(t, 4000, 4000, false), 400, 600); err == nil {
		t.Fatal("decoded a picture too large for the receiver")
	}
	if _, err := prepareArtwork([]byte("<html>login</html>"), 400, 600); err == nil {
		t.Fatal("accepted invalid picture")
	}
}
