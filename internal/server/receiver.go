package server

import (
	"dreaminator/internal/enigma"
	"errors"
	"net/http"
)

func receiverReply(w http.ResponseWriter, v any, err error) {
	if errors.Is(err, enigma.ErrReceiverConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	reply(w, v, err)
}
func (s *server) receiverRoutes(mux *http.ServeMux) {
	a := s.app
	mux.HandleFunc("GET /api/receiver-bouquets", func(w http.ResponseWriter, r *http.Request) { v, err := a.ReceiverBouquets(); receiverReply(w, v, err) })
	mux.HandleFunc("GET /api/receiver-bouquets/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := a.ReceiverBouquet(r.Context(), r.PathValue("id"))
		receiverReply(w, v, err)
	})
	mux.HandleFunc("PUT /api/receiver-bouquets/{id}", func(w http.ResponseWriter, r *http.Request) {
		var p enigma.ReceiverPatch
		if !readJSON(w, r, &p) {
			return
		}
		v, err := a.SaveReceiverBouquet(r.Context(), r.PathValue("id"), p)
		receiverReply(w, v, err)
	})
	mux.HandleFunc("POST /api/receiver-bouquets/{id}/undo", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Revision string `json:"revision"`
		}
		if !readJSON(w, r, &p) {
			return
		}
		v, err := a.UndoReceiverBouquet(r.Context(), r.PathValue("id"), p.Revision)
		receiverReply(w, v, err)
	})
	mux.HandleFunc("POST /api/receiver-bouquets/{id}/copy", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Revision string `json:"revision"`
			Name     string `json:"name"`
		}
		if !readJSON(w, r, &p) {
			return
		}
		v, err := a.CopyReceiverBouquet(r.Context(), r.PathValue("id"), p.Revision, p.Name)
		receiverReply(w, v, err)
	})
	mux.HandleFunc("POST /api/receiver-bouquets/{id}/rename", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Revision string `json:"revision"`
			Name     string `json:"name"`
		}
		if !readJSON(w, r, &p) {
			return
		}
		v, err := a.RenameReceiverBouquet(r.Context(), r.PathValue("id"), p.Revision, p.Name)
		receiverReply(w, v, err)
	})
	mux.HandleFunc("DELETE /api/receiver-bouquets/{id}", func(w http.ResponseWriter, r *http.Request) {
		warning, err := a.DeleteReceiverBouquet(r.Context(), r.PathValue("id"), r.URL.Query().Get("revision"))
		receiverReply(w, map[string]string{"warning": warning}, err)
	})
	mux.HandleFunc("POST /api/receiver-bouquets/reload", func(w http.ResponseWriter, r *http.Request) {
		receiverReply(w, nil, a.ReloadReceiverBouquets(r.Context()))
	})
}
