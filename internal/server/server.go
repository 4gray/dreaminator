// Package server exposes the tablet UI and its JSON API.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"dreaminator/internal/app"
)

var started = time.Now()

type server struct {
	app *app.App
}

func New(a *app.App, static fs.FS) http.Handler {
	s := &server{app: a}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.View())
	})
	mux.HandleFunc("GET /api/box", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.Box(r.Context()))
	})
	// The on-TV Enigma2 plugin: status, a QR code for the tablet, and the
	// "refresh everything" button.
	mux.HandleFunc("GET /api/summary", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.Summary())
	})
	mux.HandleFunc("POST /api/refresh-all", func(w http.ResponseWriter, r *http.Request) {
		started := a.RefreshAll(context.Background())
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true, "started": started})
	})
	mux.HandleFunc("GET /api/qr.png", func(w http.ResponseWriter, r *http.Request) {
		urls := a.TabletURLs()
		if len(urls) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "у ресивера нет адреса в сети"})
			return
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		png, err := qrPNG(urls[0], size)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(png)
	})
	// Memory matters next to Enigma2 on a 2015 set-top box.
	mux.HandleFunc("GET /api/debug", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		writeJSON(w, http.StatusOK, map[string]any{
			"heapMB":     float64(m.HeapAlloc) / (1 << 20),
			"sysMB":      float64(m.Sys) / (1 << 20),
			"goroutines": runtime.NumGoroutine(),
			"uptime":     time.Since(started).Round(time.Second).String(),
			"arch":       runtime.GOARCH,
		})
	})

	mux.HandleFunc("POST /api/sources", s.createSource)
	mux.HandleFunc("POST /api/sources/upload", func(w http.ResponseWriter, r *http.Request) {
		src, err := readUpload(w, r, func(filename string, f io.Reader, fields map[string]string) (any, error) {
			return a.AddUploadedSource(fields["name"], fields["epgUrl"], filename, f)
		})
		reply(w, src, err)
	})
	mux.HandleFunc("POST /api/sources/{id}/upload", func(w http.ResponseWriter, r *http.Request) {
		src, err := readUpload(w, r, func(_ string, f io.Reader, _ map[string]string) (any, error) {
			return a.ReplaceUpload(r.PathValue("id"), f)
		})
		reply(w, src, err)
	})
	mux.HandleFunc("PUT /api/sources/{id}", s.updateSource)
	mux.HandleFunc("DELETE /api/sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, a.DeleteSource(r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/sources/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		reply(w, nil, a.RefreshSource(ctx, r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/sources/{id}/check", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		acc, err := a.CheckSource(ctx, r.PathValue("id"))
		reply(w, acc, err)
	})
	mux.HandleFunc("POST /api/probe", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Source string `json:"source"`
			ID     string `json:"id"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
		defer cancel()
		res, err := a.ProbeChannel(ctx, body.Source, body.ID)
		reply(w, res, err)
	})
	mux.HandleFunc("GET /api/sources/{id}/groups", func(w http.ResponseWriter, r *http.Request) {
		groups, err := a.Groups(r.PathValue("id"))
		reply(w, groups, err)
	})
	mux.HandleFunc("GET /api/channels", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 500
		}
		writeJSON(w, http.StatusOK, a.Channels(q.Get("source"), q.Get("group"), q.Get("q"), limit))
	})

	mux.HandleFunc("POST /api/bouquets", s.createBouquet)
	mux.HandleFunc("PUT /api/bouquets/{id}", s.updateBouquet)
	mux.HandleFunc("POST /api/bouquets/{id}/channels", s.modifyBouquet)
	mux.HandleFunc("DELETE /api/bouquets/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, a.DeleteBouquet(r.PathValue("id")))
	})
	mux.HandleFunc("PUT /api/bouquet-order", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"ids"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		reply(w, nil, a.ReorderBouquets(body.IDs))
	})

	mux.HandleFunc("POST /api/apply", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		reply(w, nil, a.Apply(ctx))
	})
	mux.HandleFunc("POST /api/zap", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Source string `json:"source"`
			ID     string `json:"id"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		reply(w, nil, a.Zap(r.Context(), body.Source, body.ID))
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var body app.Settings
		if !readJSON(w, r, &body) {
			return
		}
		reply(w, nil, a.UpdateSettings(body))
	})
	mux.HandleFunc("POST /api/picons/refresh", func(w http.ResponseWriter, r *http.Request) {
		go a.SyncPicons(context.Background(), true)
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/epg/refresh", func(w http.ResponseWriter, r *http.Request) {
		go a.RebuildEPG(context.Background(), true)
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
	})

	// Stalker channels in bouquets point here: fetch a fresh temporary link
	// from the portal and send the player on to it.
	mux.HandleFunc("GET /stalker/{source}/{channel}", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
		defer cancel()
		target, err := a.ResolveStalker(ctx, r.PathValue("source"), r.PathValue("channel"))
		if errors.Is(err, app.ErrNotFound) {
			http.Error(w, "channel not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusFound)
	})
	// EPGImport on the receiver downloads the filtered guide from here.
	mux.HandleFunc("GET /epg.xml", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(a.EPGPath()); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		http.ServeFile(w, r, a.EPGPath())
	})

	files := http.FileServerFS(static)
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	return mux
}

func (s *server) createSource(w http.ResponseWriter, r *http.Request) {
	var in app.SourceInput
	if !readJSON(w, r, &in) {
		return
	}
	src, err := s.app.AddSource(in)
	reply(w, src, err)
}

func (s *server) updateSource(w http.ResponseWriter, r *http.Request) {
	var in app.SourceInput
	if !readJSON(w, r, &in) {
		return
	}
	src, err := s.app.UpdateSource(r.PathValue("id"), in)
	reply(w, src, err)
}

func (s *server) createBouquet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string           `json:"name"`
		Channels []app.ChannelRef `json:"channels"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	b, err := s.app.CreateBouquet(body.Name, body.Channels)
	reply(w, b, err)
}

func (s *server) updateBouquet(w http.ResponseWriter, r *http.Request) {
	var p app.BouquetPatch
	if !readJSON(w, r, &p) {
		return
	}
	reply(w, nil, s.app.UpdateBouquet(r.PathValue("id"), p))
}

func (s *server) modifyBouquet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Add    []app.ChannelRef `json:"add"`
		Remove []app.ChannelRef `json:"remove"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	reply(w, nil, s.app.ModifyBouquet(r.PathValue("id"), body.Add, body.Remove))
}

// readUpload streams a multipart form to disk-bound handle(): text fields
// must come before the "file" part, which is handed over without buffering
// the whole playlist in the receiver's scarce RAM.
func readUpload(w http.ResponseWriter, r *http.Request, handle func(filename string, f io.Reader, fields map[string]string) (any, error)) (any, error) {
	r.Body = http.MaxBytesReader(w, r.Body, app.MaxUpload+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, errors.New("ожидался файл плейлиста")
	}
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return nil, errors.New("файл не выбран")
		}
		if err != nil {
			return nil, fmt.Errorf("файл не дошёл до ресивера: %w", err)
		}
		if part.FormName() == "file" {
			return handle(part.FileName(), part, fields)
		}
		v, _ := io.ReadAll(io.LimitReader(part, 4096))
		fields[part.FormName()] = string(v)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный запрос: " + err.Error()})
		return false
	}
	return true
}

func reply(w http.ResponseWriter, v any, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case err != nil:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case v == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeJSON(w, http.StatusOK, v)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
