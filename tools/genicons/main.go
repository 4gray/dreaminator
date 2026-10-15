// genicons renders the PNG home-screen icons (Android wants PNG, not SVG).
// Run: go run ./tools/genicons
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"strconv"
)

func main() {
	for _, size := range []int{192, 512} {
		if err := write(size, "web/static/icon-"+strconv.Itoa(size)+".png"); err != nil {
			log.Fatal(err)
		}
	}
}

// inside reports whether point (x, y) in 0..64 icon space is white.
func inside(x, y float64) bool {
	// TV outline: rounded rect 12..52 x 16..43, stroke 4.
	outer := roundRect(x, y, 10, 14, 54, 45, 7)
	inner := roundRect(x, y, 14, 18, 50, 41, 3)
	if outer && !inner {
		return true
	}
	// Stand.
	if math.Abs(y-50) <= 2 && x >= 22 && x <= 42 {
		return true
	}
	if math.Hypot(x-22, y-50) <= 2 || math.Hypot(x-42, y-50) <= 2 {
		return true
	}
	// Play triangle (28,23.5) (28,35.5) (38,29.5).
	return x >= 28 && (y-23.5) >= (x-28)*0.6 && (35.5-y) >= (x-28)*0.6
}

func roundRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	return math.Hypot(x-cx, y-cy) <= r
}

func write(size int, path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	from := [3]float64{0x4f, 0x6e, 0xf7}
	to := [3]float64{0x8a, 0x4f, 0xe0}
	const ss = 4 // supersampling for smooth edges
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var white float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / float64(size) * 64
					y := (float64(py) + (float64(sy)+0.5)/ss) / float64(size) * 64
					if inside(x, y) {
						white++
					}
				}
			}
			white /= ss * ss
			t := (float64(px) + float64(py)) / float64(2*size)
			var c [3]float64
			for i := range c {
				bg := from[i] + (to[i]-from[i])*t
				c[i] = bg + (255-bg)*white
			}
			img.SetNRGBA(px, py, color.NRGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
