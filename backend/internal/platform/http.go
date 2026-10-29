package platform

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Pinger interface{ Ping(context.Context) error }

func Router(db Pinger, logger *slog.Logger, register ...func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			started := time.Now()
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			w.Header().Set("X-Request-ID", middleware.GetReqID(req.Context()))
			defer func() {
				logger.Info("http request", "request_id", middleware.GetReqID(req.Context()), "method", req.Method, "path", req.URL.Path, "status", ww.Status(), "duration", time.Since(started))
			}()
			next.ServeHTTP(ww, req)
		})
	})
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))
	r.Get("/api/healthz", func(w http.ResponseWriter, req *http.Request) {
		respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/api/readyz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			logger.Warn("database readiness failed", "request_id", middleware.GetReqID(req.Context()), "error", err)
			respond(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "code": "DATABASE_UNAVAILABLE", "message": "The API is running, but PostgreSQL is unavailable.", "request_id": middleware.GetReqID(req.Context())})
			return
		}
		respond(w, http.StatusOK, map[string]string{"status": "ready", "database": "connected"})
	})
	for _, routes := range register {
		routes(r)
	}
	return r
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
