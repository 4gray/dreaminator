package server

import (
	"bytes"
	"context"
	"dreaminator/internal/app"
	"dreaminator/internal/stalker"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func catalogReply(w http.ResponseWriter, v any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, v)
		return
	}
	// "code" lets the TV tell a dead network from a portal that refused.
	status, code := http.StatusBadGateway, "provider"
	switch {
	case errors.Is(err, app.ErrNotFound):
		status, code = http.StatusNotFound, "missing"
	case errors.Is(err, app.ErrCatalogArchive):
		status, code = http.StatusNotFound, "archive"
	case errors.Is(err, app.ErrCatalogExpired):
		status, code = http.StatusGone, "expired"
	case errors.Is(err, app.ErrCatalogInput):
		status, code = http.StatusBadRequest, "input"
	case errors.Is(err, stalker.ErrCatalogNetwork), errors.Is(err, context.DeadlineExceeded):
		code = "network"
	case errors.Is(err, stalker.ErrCatalogUnsupported):
		code = "unsupported"
	}
	msg := err.Error()
	if code == "network" {
		msg = stalker.ErrCatalogNetwork.Error()
	}
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}
func (s *server) catalogRoutes(mux *http.ServeMux) {
	a := s.app
	mux.HandleFunc("GET /api/catalog/sources", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, a.CatalogSources()) })
	mux.HandleFunc("GET /api/catalog/{source}/categories", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		q := r.URL.Query()
		v, err := a.CatalogCategories(ctx, r.PathValue("source"), q.Get("type"), q.Get("refresh") == "1")
		catalogReply(w, v, err)
	})
	mux.HandleFunc("GET /api/catalog/{source}/items", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		q := r.URL.Query()
		page := 1
		if q.Has("page") {
			var err error
			page, err = strconv.Atoi(q.Get("page"))
			if err != nil {
				catalogReply(w, nil, app.ErrCatalogInput)
				return
			}
		}
		v, err := a.CatalogItemsSorted(ctx, r.PathValue("source"), q.Get("type"), q.Get("category"), q.Get("parent"), q.Get("sort"), page, q.Get("refresh") == "1")
		cards := a.CatalogCards(r.PathValue("source"), v)
		a.MarkLibrary(&cards)
		catalogReply(w, cards, err)
	})
	// Programme of a channel: now and next with descriptions; with day=N
	// (0 today, 1 tomorrow…) also that day's programmes. tz is the TV's
	// offset from UTC in minutes, so days are the TV's days. id, the
	// persistent id, finds the channel once its key expired.
	mux.HandleFunc("GET /api/catalog/{source}/epg", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		q := r.URL.Query()
		var day *int
		if q.Has("day") {
			n, err := strconv.Atoi(q.Get("day"))
			if err != nil {
				catalogReply(w, nil, app.ErrCatalogInput)
				return
			}
			day = &n
		}
		var zone *time.Location
		if q.Has("tz") {
			m, err := strconv.Atoi(q.Get("tz"))
			if err != nil || m < -14*60 || m > 14*60 {
				catalogReply(w, nil, app.ErrCatalogInput)
				return
			}
			zone = time.FixedZone("TV", m*60)
		}
		key := a.CurrentKey(r.PathValue("source"), q.Get("key"), q.Get("id"))
		v, err := a.CatalogGuide(ctx, r.PathValue("source"), key, day, zone)
		catalogReply(w, v, err)
	})
	// What is on now on the rows on screen: keys=k1,k2,…
	mux.HandleFunc("GET /api/catalog/{source}/now", func(w http.ResponseWriter, r *http.Request) {
		var keys []string
		if k := r.URL.Query().Get("keys"); k != "" {
			keys = strings.Split(k, ",")
		}
		v, err := a.CatalogNow(r.PathValue("source"), keys)
		catalogReply(w, map[string]any{"now": v}, err)
	})
	mux.HandleFunc("GET /api/catalog/{source}/search", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		q := r.URL.Query()
		page := 1
		if q.Has("page") {
			var err error
			if page, err = strconv.Atoi(q.Get("page")); err != nil {
				catalogReply(w, nil, app.ErrCatalogInput)
				return
			}
		}
		v, err := a.CatalogSearch(ctx, r.PathValue("source"), q.Get("type"), q.Get("q"), page)
		a.MarkLibrary(&v.CatalogCards)
		catalogReply(w, v, err)
	})
	mux.HandleFunc("GET /api/catalog/{source}/image", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		q := r.URL.Query()
		n, _ := strconv.Atoi(q.Get("n"))
		art, err := a.CatalogImage(ctx, r.PathValue("source"), q.Get("key"), q.Get("kind"), n)
		if err != nil {
			catalogReply(w, nil, err)
			return
		}
		writeArtwork(w, art)
	})
	mux.HandleFunc("GET /api/catalog/{source}/episodes", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		eps, err := a.CatalogEpisodes(ctx, r.PathValue("source"), r.URL.Query().Get("key"))
		catalogReply(w, map[string]any{"episodes": eps}, err)
	})
	mux.HandleFunc("GET /api/catalog/{source}/tmdb", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		info, err := a.CatalogTMDB(ctx, r.PathValue("source"), r.URL.Query().Get("key"))
		catalogReply(w, map[string]any{"tmdb": info}, err)
	})
	mux.HandleFunc("GET /api/catalog/{source}/details", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		v, err := a.CatalogDetails(ctx, r.PathValue("source"), r.URL.Query().Get("key"))
		catalogReply(w, v, err)
	})
	mux.HandleFunc("GET /api/catalog/{source}/poster", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		art, err := a.CatalogPoster(ctx, r.PathValue("source"), r.URL.Query().Get("key"))
		if err != nil {
			catalogReply(w, nil, err)
			return
		}
		writeArtwork(w, art)
	})
	// The playlist channel behind a service the receiver plays (ref is its
	// reference as Enigma2 prints it): the EPG key of a channel list opens
	// this channel's programme and archive.
	mux.HandleFunc("GET /api/catalog/service", func(w http.ResponseWriter, r *http.Request) {
		v, err := a.ChannelForService(r.URL.Query().Get("ref"))
		catalogReply(w, v, err)
	})
	mux.HandleFunc("POST /api/catalog/{source}/play", func(w http.ResponseWriter, r *http.Request) {
		// The key names the row; the persistent id plays it after a list refresh.
		// start (unix seconds) plays that programme from the channel's archive.
		var p struct {
			Key   string `json:"key"`
			ID    string `json:"id"`
			Start int64  `json:"start"`
		}
		if !readJSON(w, r, &p) {
			return
		}
		if p.Key == "" && p.ID == "" {
			catalogReply(w, nil, app.ErrCatalogInput)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		if p.Start != 0 {
			v, err := a.CatalogPlayArchive(ctx, r.PathValue("source"), p.Key, p.ID, p.Start)
			catalogReply(w, v, err)
			return
		}
		v, err := a.CatalogPlayEntry(ctx, r.PathValue("source"), p.Key, p.ID)
		catalogReply(w, v, err)
	})
}

// writeArtwork sends a picture with its size, read from the header only, so
// the TV can ask its decoder for a size that fills the slot (no bars).
func writeArtwork(w http.ResponseWriter, art app.Artwork) {
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(art.Data)); err == nil {
		w.Header().Set("X-Image-Width", strconv.Itoa(cfg.Width))
		w.Header().Set("X-Image-Height", strconv.Itoa(cfg.Height))
	}
	w.Header().Set("Content-Type", art.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Write(art.Data)
}
