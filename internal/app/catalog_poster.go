package app

import (
	"bytes"
	"context"
	"dreaminator/internal/fetch"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"time"

	_ "golang.org/x/image/webp"
)

// Catalog artwork reaches the TV as the provider's own JPEG or PNG whenever it
// is a reasonable size: Enigma2's ePicLoad decodes and scales it in C++ on a
// background thread. Re-encoding in Go is the slow path (a soft-float MIPS
// needs tens of seconds to scale one large poster), kept for formats ePicLoad
// cannot read and for oversized pictures.
const (
	artworkMaxDownload = 4 << 20
	artworkMaxPass     = 1536 << 10
	artworkMaxPassSide = 2000
	artworkMaxPixels   = 4 * 1024 * 1024 // a decoded fallback must fit beside Enigma2
	artworkCacheBytes  = 6 << 20
	artworkFetchers    = 3
)

var errArtwork = errors.New("обложка недоступна")

// Artwork is a picture ready for the plugin.
type Artwork struct {
	Data        []byte
	ContentType string
}

// prepareArtwork validates a picture by its header and passes it through
// when possible. maxW/maxH bound the fallback re-encode.
func prepareArtwork(data []byte, maxW, maxH int) (Artwork, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 {
		return Artwork{}, errArtwork
	}
	if (format == "jpeg" || format == "png") && len(data) <= artworkMaxPass && cfg.Width <= artworkMaxPassSide && cfg.Height <= artworkMaxPassSide {
		return Artwork{Data: data, ContentType: "image/" + format}, nil
	}
	if cfg.Width*cfg.Height > artworkMaxPixels {
		return Artwork{}, errArtwork
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Artwork{}, errArtwork
	}
	dst := shrink(src, maxW, maxH)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return Artwork{}, errArtwork
	}
	return Artwork{Data: out.Bytes(), ContentType: "image/jpeg"}, nil
}

// shrink fits src into maxW x maxH with integer box sampling over a dark
// matte: no floating point, which the receiver's soft-float build pays for
// dearly. Pictures already small enough keep their size.
func shrink(src image.Image, maxW, maxH int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	w, h := sw, sh
	if w > maxW || h > maxH {
		if sw*maxH > sh*maxW {
			w, h = maxW, max(1, sh*maxW/sw)
		} else {
			w, h = max(1, sw*maxH/sh), maxH
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	matte := color.RGBA{R: 24, G: 30, B: 40, A: 255}
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*sh/h, b.Min.Y+max((y+1)*sh/h, y*sh/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*sw/w, b.Min.X+max((x+1)*sw/w, x*sw/w+1)
			// Sample at most a 3x3 lattice of each source box: enough to avoid
			// aliasing on posters, and bounded work per output pixel.
			var r, g, bl, a, n uint32
			for sy := y0; sy < y1; sy += max(1, (y1-y0)/3) {
				for sx := x0; sx < x1; sx += max(1, (x1-x0)/3) {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, bl, a, n = r+cr, g+cg, bl+cb, a+ca, n+1
				}
			}
			r, g, bl, a = r/n>>8, g/n>>8, bl/n>>8, a/n>>8
			// Composite over the matte so transparent logos stay readable.
			inv := 255 - a
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(r + uint32(matte.R)*inv/255),
				G: uint8(g + uint32(matte.G)*inv/255),
				B: uint8(bl + uint32(matte.B)*inv/255),
				A: 255,
			})
		}
	}
	return dst
}

// CatalogPoster returns the provider artwork of a catalog entry.
func (a *App) CatalogPoster(ctx context.Context, id, key string) (Artwork, error) {
	a.mu.Lock()
	cache, client, err := a.catalogLocked(id)
	if err != nil {
		a.mu.Unlock()
		return Artwork{}, err
	}
	if art, ok := cache.posters[key]; ok {
		a.mu.Unlock()
		return art, nil
	}
	rec, ok := a.catalogRecordLocked(id, cache, key)
	settings := a.st.Settings
	a.mu.Unlock()
	if !ok {
		return Artwork{}, ErrCatalogExpired
	}
	u, err := url.Parse(rec.item.Poster)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Artwork{}, ErrNotFound
	}
	// A few downloads may overlap; a grid page asks for several at once.
	select {
	case a.artworkSlots <- struct{}{}:
		defer func() { <-a.artworkSlots }()
	case <-ctx.Done():
		return Artwork{}, ctx.Err()
	}
	hc := fetch.Client(a.fetchOptions(settings, 10*time.Second))
	// Recompute credential scope at each redirect; never forward portal auth to a CDN.
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("слишком много перенаправлений")
		}
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		if client != nil {
			for k, v := range client.PlaybackHeaders(req.URL.String()) {
				req.Header.Set(k, v)
			}
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return Artwork{}, ErrNotFound
	}
	if client != nil {
		for k, v := range client.PlaybackHeaders(u.String()) {
			req.Header.Set(k, v)
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Artwork{}, errArtwork
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Artwork{}, ErrNotFound
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, artworkMaxDownload+1))
	if err != nil || len(data) > artworkMaxDownload {
		return Artwork{}, errArtwork
	}
	// The slow path decodes; keep it to one picture at a time.
	a.catalogPosterMu.Lock()
	art, err := prepareArtwork(data, 400, 600)
	a.catalogPosterMu.Unlock()
	if err != nil {
		return Artwork{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.catalogCurrent(id, cache) {
		return Artwork{}, ErrCatalogExpired
	}
	if cache.posterBytes+len(art.Data) > artworkCacheBytes {
		cache.posters = map[string]Artwork{}
		cache.posterBytes = 0
	}
	if _, exists := cache.posters[key]; !exists {
		cache.posters[key] = art
		cache.posterBytes += len(art.Data)
	}
	return art, nil
}
