// genui draws the plugin's interface artwork: focus rings, rounded plates,
// poster masks, fades and icons. The list and every size come from
// plugin/Dreaminator/ui_theme.py, so the PNGs always match the skin.
//
//	go run ./tools/genui
//
// Full-HD artwork goes to ui/fhd, the same shapes at 2/3 to ui/hd.
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"dreaminator/internal/brand"

	"golang.org/x/image/vector"
)

type spec struct {
	Name   string  `json:"name"`
	Part   string  `json:"part"` // crop of the full shape: top|bottom|left|right|tl|tr|bl|br
	Side   float64 `json:"side"` // strip thickness or corner size for Part
	Kind   string  `json:"kind"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	R      float64 `json:"r"`
	Stroke float64 `json:"stroke"`
	Color  string  `json:"color"`
	FG     string  `json:"fg"`
	Icon   string  `json:"icon"`
	From   float64 `json:"from"`
	To     float64 `json:"to"`
}

func main() {
	plugin := "plugin/Dreaminator"
	out, err := exec.Command("python3", filepath.Join(plugin, "ui_theme.py")).Output()
	if err != nil {
		log.Fatalf("ui_theme.py: %v", err)
	}
	var specs []spec
	if err := json.Unmarshal(out, &specs); err != nil {
		log.Fatal(err)
	}
	for _, target := range []struct {
		dir   string
		scale float64
	}{{"fhd", 1}, {"hd", 2.0 / 3}} {
		dir := filepath.Join(plugin, "ui", target.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
		old, _ := filepath.Glob(filepath.Join(dir, "*.png"))
		for _, f := range old {
			os.Remove(f)
		}
		for _, s := range specs {
			img := render(s, target.scale)
			if s.Part != "" {
				img = crop(img, s.Part, px(s.Side, target.scale))
			}
			if err := write(filepath.Join(dir, s.Name+".png"), img); err != nil {
				log.Fatal(err)
			}
		}
	}
	fmt.Printf("%d pictures in %s/ui/{fhd,hd}\n", len(specs), plugin)
}

// px mirrors Skin.px in ui_theme.py (Python rounds halves to even; sizes in
// the theme never land on .5 at 2/3, so plain rounding matches).
func px(v, scale float64) int { return int(math.Round(v * scale)) }

func render(s spec, scale float64) *image.NRGBA {
	w, h := px(s.W, scale), px(s.H, scale)
	if s.Kind == "brand" {
		return brand.Render(w)
	}
	r := s.R * scale
	c := parse(s.Color)
	switch s.Kind {
	case "rrect":
		return paint(w, h, c, cover(w, h, func(z *vector.Rasterizer) { rrect(z, 0, 0, float64(w), float64(h), r, false) }))
	case "ring":
		stroke := math.Max(2, math.Round(s.Stroke*scale))
		return paint(w, h, c, cover(w, h, func(z *vector.Rasterizer) {
			rrect(z, 0, 0, float64(w), float64(h), r, false)
			rrect(z, stroke, stroke, float64(w)-2*stroke, float64(h)-2*stroke, math.Max(0, r-stroke), true)
		}))
	case "mask":
		return paint(w, h, c, cover(w, h, func(z *vector.Rasterizer) {
			rect(z, 0, 0, float64(w), float64(h), false)
			rrect(z, 0, 0, float64(w), float64(h), r, true)
		}))
	case "placeholder":
		img := paint(w, h, c, cover(w, h, func(z *vector.Rasterizer) { rrect(z, 0, 0, float64(w), float64(h), r, false) }))
		side := math.Round(math.Min(float64(w), float64(h)) / 3)
		over(img, icon(s.Icon, int(side), parse(s.FG)), (w-int(side))/2, (h-int(side))/2)
		return img
	case "hgradient", "vgradient":
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				t := float64(x) / math.Max(1, float64(w-1))
				if s.Kind == "vgradient" {
					t = float64(y) / math.Max(1, float64(h-1))
				}
				// Ease the fade so the picture does not end in a visible band.
				t = t * t * (3 - 2*t)
				a := s.From + (s.To-s.From)*t
				img.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(math.Round(a))})
			}
		}
		return img
	case "icon":
		return icon(s.Icon, w, c)
	case "rimmed":
		// An icon over a larger copy of itself in FG: readable on the dark
		// list and on the accent pill of the focused row alike.
		img := icon(s.Icon, w, parse(s.FG))
		side := int(math.Round(float64(w) * 0.78))
		over(img, icon(s.Icon, side, c), (w-side)/2, (h-side)/2)
		return img
	case "badge":
		// An opaque tile with a large icon: drawn over posters without blending.
		img := paint(w, h, c, cover(w, h, func(z *vector.Rasterizer) { rrect(z, 0, 0, float64(w), float64(h), r, false) }))
		side := int(math.Round(math.Min(float64(w), float64(h)) * 0.76))
		over(img, icon(s.Icon, side, parse(s.FG)), (w-side)/2, (h-side)/2)
		return img
	}
	log.Fatalf("%s: unknown kind %q", s.Name, s.Kind)
	return nil
}

// crop keeps one edge strip or corner: blending a full-size overlay over a
// decoded poster is far too slow on the receiver, a few strips are not.
func crop(img *image.NRGBA, part string, side int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	r := map[string]image.Rectangle{
		"top": image.Rect(0, 0, w, side), "bottom": image.Rect(0, h-side, w, h),
		"left": image.Rect(0, side, side, h-side), "right": image.Rect(w-side, side, w, h-side),
		"tl": image.Rect(0, 0, side, side), "tr": image.Rect(w-side, 0, w, side),
		"bl": image.Rect(0, h-side, side, h), "br": image.Rect(w-side, h-side, w, h),
	}[part]
	if r.Empty() {
		log.Fatalf("unknown part %q", part)
	}
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()*4], img.Pix[(r.Min.Y+y)*img.Stride+r.Min.X*4:(r.Min.Y+y)*img.Stride+r.Max.X*4])
	}
	return out
}

func parse(hex string) color.NRGBA {
	if len(hex) != 7 || hex[0] != '#' {
		log.Fatalf("colour %q", hex)
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		log.Fatal(err)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// cover rasterises one shape into an anti-aliased coverage mask.
func cover(w, h int, draw func(*vector.Rasterizer)) *image.Alpha {
	z := vector.NewRasterizer(w, h)
	draw(z)
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return mask
}

// union keeps the strongest coverage: strokes overlap at joints.
func union(dst, src *image.Alpha) {
	for i, a := range src.Pix {
		if a > dst.Pix[i] {
			dst.Pix[i] = a
		}
	}
}

func paint(w, h int, c color.NRGBA, mask *image.Alpha) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i, a := range mask.Pix {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = c.R, c.G, c.B, uint8(uint32(a)*uint32(c.A)/255)
	}
	return img
}

// over composites src onto dst at (x, y).
func over(dst, src *image.NRGBA, x0, y0 int) {
	b := src.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			s := src.NRGBAAt(x, y)
			if s.A == 0 {
				continue
			}
			d := dst.NRGBAAt(x0+x, y0+y)
			a := float64(s.A) / 255
			mix := func(sv, dv uint8) uint8 { return uint8(math.Round(float64(sv)*a + float64(dv)*(1-a))) }
			dst.SetNRGBA(x0+x, y0+y, color.NRGBA{mix(s.R, d.R), mix(s.G, d.G), mix(s.B, d.B), uint8(math.Max(float64(d.A), float64(s.A)))})
		}
	}
}

func rect(z *vector.Rasterizer, x, y, w, h float64, reverse bool) {
	pts := [][2]float64{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}
	if reverse {
		pts[1], pts[3] = pts[3], pts[1]
	}
	z.MoveTo(float32(pts[0][0]), float32(pts[0][1]))
	for _, p := range pts[1:] {
		z.LineTo(float32(p[0]), float32(p[1]))
	}
	z.ClosePath()
}

const kappa = 0.5522847498

// rrect adds a rounded rectangle; reverse winding cuts a hole.
func rrect(z *vector.Rasterizer, x, y, w, h, r float64, reverse bool) {
	r = math.Min(r, math.Min(w, h)/2)
	k := r * kappa
	f := func(v float64) float32 { return float32(v) }
	if !reverse {
		z.MoveTo(f(x+r), f(y))
		z.LineTo(f(x+w-r), f(y))
		z.CubeTo(f(x+w-r+k), f(y), f(x+w), f(y+r-k), f(x+w), f(y+r))
		z.LineTo(f(x+w), f(y+h-r))
		z.CubeTo(f(x+w), f(y+h-r+k), f(x+w-r+k), f(y+h), f(x+w-r), f(y+h))
		z.LineTo(f(x+r), f(y+h))
		z.CubeTo(f(x+r-k), f(y+h), f(x), f(y+h-r+k), f(x), f(y+h-r))
		z.LineTo(f(x), f(y+r))
		z.CubeTo(f(x), f(y+r-k), f(x+r-k), f(y), f(x+r), f(y))
	} else {
		z.MoveTo(f(x+r), f(y))
		z.CubeTo(f(x+r-k), f(y), f(x), f(y+r-k), f(x), f(y+r))
		z.LineTo(f(x), f(y+h-r))
		z.CubeTo(f(x), f(y+h-r+k), f(x+r-k), f(y+h), f(x+r), f(y+h))
		z.LineTo(f(x+w-r), f(y+h))
		z.CubeTo(f(x+w-r+k), f(y+h), f(x+w), f(y+h-r+k), f(x+w), f(y+h-r))
		z.LineTo(f(x+w), f(y+r))
		z.CubeTo(f(x+w), f(y+r-k), f(x+w-r+k), f(y), f(x+w-r), f(y))
	}
	z.ClosePath()
}

func circle(z *vector.Rasterizer, cx, cy, r float64, reverse bool) {
	rrect(z, cx-r, cy-r, 2*r, 2*r, r, reverse)
}

// icon draws the line icons of the mockups (24-unit grid, 2-unit strokes).
func icon(name string, side int, c color.NRGBA) *image.NRGBA {
	s := float64(side) / 24
	sw := 2 * s
	mask := image.NewAlpha(image.Rect(0, 0, side, side))
	add := func(draw func(*vector.Rasterizer)) { union(mask, cover(side, side, draw)) }
	line := func(x1, y1, x2, y2 float64) {
		x1, y1, x2, y2 = x1*s, y1*s, x2*s, y2*s
		dx, dy := x2-x1, y2-y1
		n := math.Hypot(dx, dy)
		if n > 0 {
			ox, oy := -dy/n*sw/2, dx/n*sw/2
			add(func(z *vector.Rasterizer) {
				z.MoveTo(float32(x1+ox), float32(y1+oy))
				z.LineTo(float32(x2+ox), float32(y2+oy))
				z.LineTo(float32(x2-ox), float32(y2-oy))
				z.LineTo(float32(x1-ox), float32(y1-oy))
				z.ClosePath()
			})
		}
		add(func(z *vector.Rasterizer) { circle(z, x1, y1, sw/2, false) })
		add(func(z *vector.Rasterizer) { circle(z, x2, y2, sw/2, false) })
	}
	poly := func(closed bool, pts ...float64) {
		for i := 0; i+3 < len(pts); i += 2 {
			line(pts[i], pts[i+1], pts[i+2], pts[i+3])
		}
		if closed {
			line(pts[len(pts)-2], pts[len(pts)-1], pts[0], pts[1])
		}
	}
	box := func(x, y, w, h, r float64) {
		add(func(z *vector.Rasterizer) {
			rrect(z, (x-1)*s, (y-1)*s, (w+2)*s, (h+2)*s, (r+1)*s, false)
			rrect(z, (x+1)*s, (y+1)*s, (w-2)*s, (h-2)*s, math.Max(0, r-1)*s, true)
		})
	}
	ring := func(cx, cy, r float64) {
		add(func(z *vector.Rasterizer) {
			circle(z, cx*s, cy*s, (r+1)*s, false)
			circle(z, cx*s, cy*s, (r-1)*s, true)
		})
	}
	dot := func(x, y float64) { add(func(z *vector.Rasterizer) { circle(z, x*s, y*s, sw/2, false) }) }
	// A five-pointed star around (12, 12.6), the first point up.
	star := func(outer, inner float64) []float64 {
		pts := []float64{}
		for i := 0; i < 10; i++ {
			r, a := outer, -math.Pi/2+float64(i)*math.Pi/5
			if i%2 == 1 {
				r = inner
			}
			pts = append(pts, 12+r*math.Cos(a), 12.6+r*math.Sin(a))
		}
		return pts
	}
	switch name {
	case "star":
		poly(true, star(9, 3.9)...)
	case "star_filled":
		pts := star(10.5, 4.4)
		add(func(z *vector.Rasterizer) {
			z.MoveTo(float32(pts[0]*s), float32(pts[1]*s))
			for i := 2; i < len(pts); i += 2 {
				z.LineTo(float32(pts[i]*s), float32(pts[i+1]*s))
			}
			z.ClosePath()
		})
	case "recent":
		ring(12, 12, 9)
		line(12, 7.5, 12, 12)
		line(12, 12, 15.5, 14)
	case "portal":
		box(3, 4, 18, 6, 2)
		box(3, 14, 18, 6, 2)
		dot(7, 7)
		dot(7, 17)
	case "live":
		box(3, 7, 18, 13, 2)
		poly(false, 8, 3, 12, 7, 16, 3)
	case "film":
		box(3, 3, 18, 18, 2)
		line(7, 3, 7, 21)
		line(17, 3, 17, 21)
		line(3, 8, 7, 8)
		line(3, 16, 7, 16)
		line(17, 8, 21, 8)
		line(17, 16, 21, 16)
		line(3, 12, 21, 12)
	case "series":
		poly(true, 12, 3, 21, 8, 12, 13, 3, 8)
		poly(false, 3, 13, 12, 18, 21, 13)
	case "search":
		ring(11, 11, 7)
		line(20, 20, 16.5, 16.5)
	case "settings":
		line(4, 6, 13, 6)
		line(19, 6, 20, 6)
		line(4, 12, 7, 12)
		line(13, 12, 20, 12)
		line(4, 18, 15, 18)
		line(21, 18, 21, 18)
		ring(16, 6, 2.5)
		ring(10, 12, 2.5)
		ring(18, 18, 2.5)
	case "info":
		ring(12, 12, 9)
		line(12, 11, 12, 16)
		dot(12, 8)
	case "play":
		add(func(z *vector.Rasterizer) {
			z.MoveTo(float32(7*s), float32(4.5*s))
			z.LineTo(float32(19.5*s), float32(12*s))
			z.LineTo(float32(7*s), float32(19.5*s))
			z.ClosePath()
		})
	case "text":
		line(5, 6, 19, 6)
		line(5, 12, 19, 12)
		line(5, 18, 14, 18)
	case "back":
		poly(false, 15, 5, 8, 12, 15, 19)
	case "backspace":
		poly(true, 9, 6, 21, 6, 21, 18, 9, 18, 3, 12)
		line(12, 9, 18, 15)
		line(18, 9, 12, 15)
	default:
		log.Fatalf("unknown icon %q", name)
	}
	return paint(side, side, c, mask)
}

func write(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
