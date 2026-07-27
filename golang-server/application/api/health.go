package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func RegisterHealthRoutes(mux *chi.Mux) {
	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
