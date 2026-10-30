package audit

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/db"
)

type Handler struct {
	queries *db.Queries
	gate    func(http.Handler) http.Handler
	logger  *slog.Logger
}

func NewHandler(pool *pgxpool.Pool, gate func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{db.New(pool), gate, logger}
}
func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) { r.Use(h.gate); r.Get("/api/organizations/{orgID}/activity", h.list) })
}

type Event struct {
	ID          string          `json:"id"`
	Action      string          `json:"action"`
	EntityType  string          `json:"entity_type"`
	EntityID    string          `json:"entity_id"`
	EntityLabel string          `json:"entity_label"`
	ActorUserID string          `json:"actor_user_id"`
	ActorName   string          `json:"actor_name"`
	CreatedAt   time.Time       `json:"created_at"`
	Before      json.RawMessage `json:"before"`
	After       json.RawMessage `json:"after"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset := int64(25), int64(0)
	for name, target := range map[string]*int64{"limit": &limit, "offset": &offset} {
		if value := r.URL.Query().Get(name); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				h.fail(w, r, 422, "INVALID_PAGE", "Invalid pagination.")
				return
			}
			*target = parsed
		}
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		h.fail(w, r, 422, "INVALID_PAGE", "Use limit 1-100 and offset 0-100000.")
		return
	}
	rows, err := h.queries.PageActivity(r.Context(), db.PageActivityParams{OrgID: chi.URLParam(r, "orgID"), PageLimit: int32(limit + 1), PageOffset: int32(offset)})
	if err != nil {
		h.logger.Error("activity query failed", "error", err)
		h.fail(w, r, 503, "SERVICE_UNAVAILABLE", "The service is unavailable. Please try again.")
		return
	}
	more := len(rows) > int(limit)
	if more {
		rows = rows[:limit]
	}
	events := []Event{}
	for _, row := range rows {
		events = append(events, Event{row.ID, row.Action, row.EntityType, row.EntityID, row.EntityLabel, row.ActorUserID, row.ActorName, row.CreatedAt.Time, row.BeforeData, row.AfterData})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": events, "limit": limit, "offset": offset, "has_more": more})
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message, "request_id": middleware.GetReqID(r.Context())})
}
