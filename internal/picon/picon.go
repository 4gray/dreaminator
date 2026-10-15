// Package picon turns channel logos into Enigma2 picons: PNG files named
// after the service reference, which skins show in the channel list and
// infobar.
package picon

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Standard picon size; skins scale it to their own widgets.
const (
	Width  = 220
	Height = 132
	margin = 6
)

// FileName is the picon name Enigma2 looks for: the first ten fields of the
// reference with ':' replaced by '_'. Enigma2 retries IPTV references
// (4097:…) with the type set to 1, so 1_0_1_… covers every player type.
func FileName(ref string) string {
	fields := strings.Split(ref, ":")
	if len(fields) > 10 {
		fields = fields[:10]
	}
	if len(fields) > 0 {
		fields[0] = "1"
	}
	return strings.Join(fields, "_") + ".png"
}

// Candidates are where Enigma2 images look for picons, in the order a
// person would expect them to be used.
var Candidates = []string{
	"/media/hdd/picon", "/media/usb/picon", "/media/mmc/picon", "/media/cf/picon",
	"/usr/share/enigma2/picon", "/picon",
}

// Default is used when no picon folder exists yet: it is on the flash,
// always searched, and needs no extra storage.
const Default = "/usr/share/enigma2/picon"

// DetectDir picks the folder Enigma2 will actually use. Enigma2 remembers
// the folder of the first picon it finds and then looks only there, so
// ours must go next to existing (satellite) picons.
func DetectDir(candidates []string) string {
	firstExisting := ""
	for _, dir := range candidates {
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			continue
		}
		if firstExisting == "" {
			firstExisting = dir
		}
		if matches, _ := filepath.Glob(filepath.Join(dir, "*.png")); len(matches) > 0 {
			return dir
		}
	}
	if firstExisting != "" {
		return firstExisting
	}
	return Default
}

const maxPixels = 4096 * 4096

// Render fits a logo into a transparent 220×132 picon.
func Render(data []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		if bytes.Contains(data[:min(len(data), 512)], []byte("<svg")) {
			return nil, errors.New("логотип в SVG — не поддерживается")
		}
		return nil, fmt.Errorf("не картинка: %w", err)
	}
	if cfg.Width*cfg.Height > maxPixels || cfg.Width == 0 || cfg.Height == 0 {
		return nil, fmt.Errorf("слишком большая картинка %dx%d", cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s не читается: %w", format, err)
	}

	// Trim fully transparent borders so small logos with big padding fill
	// the picon.
	b := src.Bounds()
	if b.Dx()*b.Dy() <= 1024*1024 { // pixel walk is slow on the receiver's CPU
		b = opaqueBounds(src)
	}
	boxW, boxH := Width-2*margin, Height-2*margin
	scale := min(float64(boxW)/float64(b.Dx()), float64(boxH)/float64(b.Dy()))
	w, h := max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))
	dst := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	if isDark(src, b) {
		// Dark logos vanish on the dark skins receivers use: give them a
		// light plate, the way picon packs do.
		plate(dst)
	}
	target := image.Rect((Width-w)/2, (Height-h)/2, (Width-w)/2+w, (Height-h)/2+h)
	draw.CatmullRom.Scale(dst, target, src, b, draw.Over, nil)

	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// isDark reports whether the visible pixels of a logo are mostly dark.
func isDark(img image.Image, b image.Rectangle) bool {
	step := max(1, max(b.Dx(), b.Dy())/200)
	var sum, n float64
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0x8000 {
				continue
			}
			// Un-premultiply, then relative luminance.
			fa := float64(a)
			sum += (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(bl)) / fa
			n++
		}
	}
	return n > 0 && sum/n < 0.25 // saturated reds and greens stay above
}

func plate(dst *image.NRGBA) {
	const radius = 16
	c := color.NRGBA{0xf2, 0xf2, 0xf2, 0xf0}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			dx := max(radius-x, x-(Width-1-radius), 0)
			dy := max(radius-y, y-(Height-1-radius), 0)
			if dx*dx+dy*dy <= radius*radius {
				dst.SetNRGBA(x, y, c)
			}
		}
	}
}

func opaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x0800 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x+1), max(maxY, y+1)
			}
		}
	}
	if minX >= maxX || minY >= maxY {
		return b
	}
	return image.Rect(minX, minY, maxX, maxY)
}
