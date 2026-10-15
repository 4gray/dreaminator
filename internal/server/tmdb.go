package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"dreaminator/internal/app"
	"dreaminator/internal/tmdb"
)

// TMDB settings and manual matching for the WebUI. Replies never contain the
// API key: settings come back as the masked app.TMDBStatus, and every TMDB
// error is one of the key-free sentinels of package tmdb.
func (s *server) tmdbRoutes(mux *http.ServeMux) {
	a := s.app
	mux.HandleFunc("GET /api/tmdb", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.TMDBStatus())
	})
	mux.HandleFunc("PUT /api/tmdb", func(w http.ResponseWriter, r *http.Request) {
		var in app.TMDBUpdate
		if !readJSON(w, r, &in) {
			return
		}
		st, err := a.UpdateTMDB(in)
		tmdbReply(w, st, err)
	})
	mux.HandleFunc("POST /api/tmdb/test", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			APIKey string `json:"apiKey"`
		}
		// An empty body tests the stored key.
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный запрос"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		writeJSON(w, http.StatusOK, a.TestTMDB(ctx, in.APIKey))
	})
	mux.HandleFunc("GET /api/tmdb/match", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
		defer cancel()
		q := r.URL.Query()
		v, err := a.TMDBMatch(ctx, q.Get("source"), q.Get("key"))
		tmdbReply(w, v, err)
	})
	mux.HandleFunc("PUT /api/tmdb/match", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Source string `json:"source"`
			Key    string `json:"key"`
			ID     int    `json:"id"`
			Type   string `json:"type"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		v, err := a.SetTMDBMatch(ctx, in.Source, in.Key, in.ID, in.Type)
		tmdbReply(w, v, err)
	})
	mux.HandleFunc("DELETE /api/tmdb/match", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		err := a.ResetTMDBMatch(r.Context(), q.Get("source"), q.Get("key"))
		tmdbReply(w, map[string]bool{"ok": true}, err)
	})
	mux.HandleFunc("GET /api/tmdb/search", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		q := r.URL.Query()
		v, err := a.TMDBSearch(ctx, q.Get("q"), q.Get("type"), q.Get("year"))
		tmdbReply(w, map[string]any{"candidates": v}, err)
	})
}

func tmdbReply(w http.ResponseWriter, v any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, v)
		return
	}
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, app.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, app.ErrCatalogExpired):
		status = http.StatusGone
	case errors.Is(err, app.ErrTMDBDisabled):
		status = http.StatusConflict
	case tmdb.IsServiceError(err), errors.Is(err, context.DeadlineExceeded):
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
