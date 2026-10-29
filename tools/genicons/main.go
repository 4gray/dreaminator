// genicons renders the Modular Stream SVG, PNG home-screen icons and
// the 100x40 Enigma2 plugin browser icon. Run: go run ./tools/genicons
package main

import (
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"strconv"

	"dreaminator/internal/brand"
)

func main() {
	if err := os.WriteFile("web/static/icon.svg", []byte(brand.SVG), 0o644); err != nil {
		log.Fatal(err)
	}
	for _, size := range []int{192, 512} {
		if err := write("web/static/icon-"+strconv.Itoa(size)+".png", brand.Render(size)); err != nil {
			log.Fatal(err)
		}
	}
	// Keep the square mark centred in Enigma2's wide transparent slot.
	img := image.NewNRGBA(image.Rect(0, 0, 100, 40))
	draw.Draw(img, image.Rect(30, 0, 70, 40), brand.Render(40), image.Point{}, draw.Src)
	if err := write("plugin/Dreaminator/plugin.png", img); err != nil {
		log.Fatal(err)
	}
}

func write(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
