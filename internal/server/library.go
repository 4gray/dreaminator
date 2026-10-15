package server

import (
	"net/http"
	"strconv"
	"strings"

	"dreaminator/internal/app"
)

// libraryRoutes serve the TV catalog's "Избранное" and "Недавние" (see
// app/library.go). Favorite channels are the channel list "Избранное":
// the TV sends it to the receiver with POST /api/apply when the catalog
// closes.
func (s *server) libraryRoutes(mux *http.ServeMux) {
	a := s.app
	mux.HandleFunc("GET /api/catalog/home", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.CatalogHome())
	})
	mux.HandleFunc("GET /api/catalog/home/items", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page := 1
		if q.Has("page") {
			var err error
			if page, err = strconv.Atoi(q.Get("page")); err != nil {
				catalogReply(w, nil, app.ErrCatalogInput)
				return
			}
		}
		v, err := a.CatalogHomeItems(q.Get("list"), q.Get("category"), page)
		catalogReply(w, v, err)
	})
	favorite := func(on bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			p := struct {
				Source string `json:"source"`
				Key    string `json:"key"`
				ID     string `json:"id"`
			}{q.Get("source"), q.Get("key"), q.Get("id")}
			if r.ContentLength != 0 && !readJSON(w, r, &p) {
				return
			}
			if p.Source == "" {
				// A persistent id names its source.
				p.Source, _, _ = strings.Cut(p.ID, "/")
			}
			if p.Source == "" || p.Key == "" && p.ID == "" {
				catalogReply(w, nil, app.ErrCatalogInput)
				return
			}
			v, err := a.SetFavorite(p.Source, p.Key, p.ID, on)
			catalogReply(w, v, err)
		}
	}
	mux.HandleFunc("POST /api/catalog/favorites", favorite(true))
	mux.HandleFunc("DELETE /api/catalog/favorites", favorite(false))
	// The TV catalog closed: what it changed in the library reaches the
	// flash now instead of within the minute.
	mux.HandleFunc("POST /api/catalog/close", func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, a.SaveLibrary())
	})
}
