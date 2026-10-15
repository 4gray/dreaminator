package picon

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"1:0:1:757F:15EC:1DE:0:0:0:0:":                          "1_0_1_757F_15EC_1DE_0_0_0_0.png",
		"4097:0:1:757F:15EC:1DE:0:0:0:0:http%3a//h/1.ts:Первый": "1_0_1_757F_15EC_1DE_0_0_0_0.png",
	} {
		if got := FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRender(t *testing.T) {
	// A wide red logo with lots of transparent padding.
	src := image.NewNRGBA(image.Rect(0, 0, 1000, 1000))
	for y := 450; y < 550; y++ {
		for x := 100; x < 900; x++ {
			src.Set(x, y, color.NRGBA{200, 0, 0, 255})
		}
	}
	var in bytes.Buffer
	png.Encode(&in, src)
	out, err := Render(in.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != Width || img.Bounds().Dy() != Height {
		t.Fatalf("size %v", img.Bounds())
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("corner must stay transparent")
	}
	// Padding trimmed: the bar now spans almost the whole width.
	if _, _, _, a := img.At(margin+2, Height/2).RGBA(); a == 0 {
		t.Error("logo was not scaled to fill the picon")
	}

	var jp bytes.Buffer
	jpeg.Encode(&jp, image.NewRGBA(image.Rect(0, 0, 64, 64)), nil)
	if _, err := Render(jp.Bytes()); err != nil {
		t.Errorf("jpeg: %v", err)
	}
	if _, err := Render([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); err == nil {
		t.Error("svg must be refused")
	}
}

func TestDetectDir(t *testing.T) {
	root := t.TempDir()
	usb, flash := filepath.Join(root, "usb"), filepath.Join(root, "flash")
	os.MkdirAll(usb, 0o755)
	os.MkdirAll(flash, 0o755)
	cands := []string{filepath.Join(root, "hdd"), usb, flash}
	if got := DetectDir(cands); got != usb {
		t.Errorf("empty folders: got %s, want the first existing", got)
	}
	os.WriteFile(filepath.Join(flash, "1_0_1_A_B_C_0_0_0_0.png"), []byte("x"), 0o644)
	if got := DetectDir(cands); got != flash {
		t.Errorf("must follow existing picons: got %s", got)
	}
	if got := DetectDir([]string{filepath.Join(root, "none")}); got != Default {
		t.Errorf("fallback = %s", got)
	}
}

func TestDarkLogoGetsPlate(t *testing.T) {
	render := func(c color.NRGBA) image.Image {
		src := image.NewNRGBA(image.Rect(0, 0, 100, 60))
		for y := 10; y < 50; y++ {
			for x := 10; x < 90; x++ {
				src.Set(x, y, c)
			}
		}
		var in bytes.Buffer
		png.Encode(&in, src)
		out, err := Render(in.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		img, _ := png.Decode(bytes.NewReader(out))
		return img
	}
	// Near the edge but outside the logo area.
	if _, _, _, a := render(color.NRGBA{10, 10, 10, 255}).At(Width/2, 2).RGBA(); a == 0 {
		t.Error("dark logo has no plate")
	}
	if _, _, _, a := render(color.NRGBA{230, 40, 40, 255}).At(Width/2, 2).RGBA(); a != 0 {
		t.Error("bright logo must stay on transparency")
	}
}
