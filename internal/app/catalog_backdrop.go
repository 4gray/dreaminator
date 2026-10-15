package app

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
)

// shadeBackdrop bakes the card's scrim into a small backdrop: dimmed, and
// faded into the screen background on the left (where the text is) and at
// the bottom. Enigma2 can then draw it as a plain picture — blending an
// overlay over a decoded image stalls a DM520 for seconds, this costs the
// service about 0.7 s once per film (integer maths only, at nice 10).
func shadeBackdrop(data []byte) (Artwork, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" || cfg.Width < 64 || cfg.Height < 36 || cfg.Width*cfg.Height > 1400*800 {
		return Artwork{}, errArtwork
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return Artwork{}, errArtwork
	}
	ycc, ok := img.(*image.YCbCr)
	if !ok {
		return Artwork{}, errArtwork
	}
	b := ycc.Bounds()
	w, h := b.Dx(), b.Dy()
	// Screen background #0b0d11 in YCbCr.
	by, bcb, bcr := color.RGBToYCbCr(0x0b, 0x0d, 0x11)
	keep := func(x, y int) int {
		k := 96 // overall dimming (of 256): bright stills must not drown the text
		if edge := w * 70 / 100; x < edge {
			t := x * 256 / edge
			k = k * t * t / (256 * 256) // ease-in: reaches the background softly
		}
		if top := h * 58 / 100; y > top {
			t := (h - 1 - y) * 256 / (h - 1 - top)
			k = k * t / 256
		}
		return k
	}
	for y := 0; y < h; y++ {
		row := ycc.Y[(y)*ycc.YStride : (y)*ycc.YStride+w]
		for x := range row {
			k := keep(x, y)
			row[x] = uint8((int(row[x])*k + int(by)*(256-k)) >> 8)
		}
	}
	cw, ch := chromaSize(ycc)
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			x, y := cx*w/cw, cy*h/ch
			k := keep(x, y)
			i := cy*ycc.CStride + cx
			ycc.Cb[i] = uint8((int(ycc.Cb[i])*k + int(bcb)*(256-k)) >> 8)
			ycc.Cr[i] = uint8((int(ycc.Cr[i])*k + int(bcr)*(256-k)) >> 8)
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, ycc, &jpeg.Options{Quality: 82}); err != nil {
		return Artwork{}, errArtwork
	}
	return Artwork{Data: out.Bytes(), ContentType: "image/jpeg"}, nil
}

func chromaSize(m *image.YCbCr) (int, int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	switch m.SubsampleRatio {
	case image.YCbCrSubsampleRatio420:
		return (w + 1) / 2, (h + 1) / 2
	case image.YCbCrSubsampleRatio422:
		return (w + 1) / 2, h
	case image.YCbCrSubsampleRatio440:
		return w, (h + 1) / 2
	case image.YCbCrSubsampleRatio411:
		return (w + 3) / 4, h
	case image.YCbCrSubsampleRatio410:
		return (w + 3) / 4, (h + 1) / 2
	}
	return w, h
}
