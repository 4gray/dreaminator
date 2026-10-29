// Package brand renders the Modular Stream mark for the web and Enigma2.
package brand

import (
	"image"
	"image/color"
	"math"
)

// SVG uses the same 64-unit geometry and palette as Render.
const SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
  <rect width="64" height="64" rx="14" fill="#171b22"/>
  <path d="M27 12h13a12 12 0 0 1 12 12v16a12 12 0 0 1-12 12H27v-8h13a4 4 0 0 0 4-4V24a4 4 0 0 0-4-4H27z" fill="#f4f6f8"/>
  <path d="M14 12h9v12h-9zM14 26h9v12h-9zM14 40h9v12h-9zM30 24v16l11-8z" fill="#38bdf8"/>
</svg>
`

var (
	graphite = color.NRGBA{0x17, 0x1b, 0x22, 255}
	white    = color.NRGBA{0xf4, 0xf6, 0xf8, 255}
	cyan     = color.NRGBA{0x38, 0xbd, 0xf8, 255}
)

// Render supersamples the mark, including its transparent rounded corners.
func Render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a uint32
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) * 64 / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/ss) * 64 / float64(size)
					c := sample(x, y)
					if c.A != 0 {
						r += uint32(c.R)
						g += uint32(c.G)
						b += uint32(c.B)
						a++
					}
				}
			}
			if a != 0 {
				img.SetNRGBA(px, py, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a * 255 / (ss * ss))})
			}
		}
	}
	return img
}

func sample(x, y float64) color.NRGBA {
	if !roundRect(x, y, 0, 0, 64, 64, 14) {
		return color.NRGBA{}
	}
	// Three packets form the D's left stem; play sits inside its counter.
	stem := x >= 14 && x <= 23 && ((y >= 12 && y <= 24) || (y >= 26 && y <= 38) || (y >= 40 && y <= 52))
	play := x >= 30 && x <= 41 && math.Abs(y-32) <= (41-x)*8/11
	if stem || play {
		return cyan
	}
	outer := roundRect(x, y, 14, 12, 52, 52, 12)
	inner := roundRect(x, y, 22, 20, 44, 44, 4)
	if x >= 27 && outer && !inner {
		return white
	}
	return graphite
}

func roundRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	return math.Hypot(x-cx, y-cy) <= r
}
