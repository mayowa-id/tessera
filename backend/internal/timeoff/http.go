package timeoff

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"tessera/backend/internal/requestctx"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Handler struct {
	store           Store
	member, manager func(http.Handler) http.Handler
	logger          *slog.Logger
}

func NewHandler(store Store, member, manager func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{store, member, manager, logger}
}
func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.member)
		r.Get("/api/organizations/{orgID}/my-time-off", h.list)
		r.Post("/api/organizations/{orgID}/my-time-off", h.create)
		r.Post("/api/organizations/{orgID}/time-off/{id}/cancel", h.decide)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.manager)
		r.Get("/api/organizations/{orgID}/time-off", h.list)
		r.Post("/api/organizations/{orgID}/employees/{employeeID}/time-off", h.create)
		r.Post("/api/organizations/{orgID}/time-off/{id}/approve", h.decide)
		r.Post("/api/organizations/{orgID}/time-off/{id}/reject", h.decide)
	})
}
func (h *Handler) decode(w http.ResponseWriter, r *http.Request, input any) bool {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send a JSON object.")
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(input) != nil || d.Decode(new(any)) != io.EOF {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send one JSON object with supported fields.")
		return false
	}
	return true
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	from, e1 := time.Parse(time.RFC3339Nano, r.URL.Query().Get("from"))
	to, e2 := time.Parse(time.RFC3339Nano, r.URL.Query().Get("to"))
	if e1 != nil || e2 != nil || !to.After(from) || to.Sub(from) > 93*24*time.Hour {
		h.fail(w, r, 422, "INVALID_RANGE", "Choose a range of more than zero and at most 93 elapsed days.")
		return
	}
	limit, offset := int64(25), int64(0)
	for name, target := range map[string]*int64{"limit": &limit, "offset": &offset} {
		if value := r.URL.Query().Get(name); value != "" {
			parsed, e := strconv.ParseInt(value, 10, 32)
			if e != nil {
				h.fail(w, r, 422, "INVALID_PAGE", "Invalid pagination.")
				return
			}
			*target = parsed
		}
	}
	status := r.URL.Query().Get("status")
	employee := r.URL.Query().Get("employee_id")
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 || (employee != "" && !uuid.MatchString(employee)) || (status != "" && status != "pending" && status != "approved" && status != "rejected" && status != "cancelled") {
		h.fail(w, r, 422, "INVALID_FILTER", "Check the status, employee, and pagination filters.")
		return
	}
	team := !strings.HasSuffix(r.URL.Path, "/my-time-off")
	if !team && employee != "" {
		h.fail(w, r, 422, "INVALID_FILTER", "Personal requests cannot select another employee.")
		return
	}
	items, err := h.store.List(r.Context(), chi.URLParam(r, "orgID"), requestctx.UserID(r.Context()), team, Filter{from, to, status, employee, int32(limit), int32(offset)})
	if err != nil {
		h.failure(w, r, err)
		return
	}
	more := len(items) > int(limit)
	if more {
		items = items[:limit]
	}
	respond(w, 200, map[string]any{"items": items, "limit": limit, "offset": offset, "has_more": more})
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	employee := chi.URLParam(r, "employeeID")
	if employee != "" && !uuid.MatchString(employee) {
		h.fail(w, r, 422, "INVALID_ID", "Invalid employee ID.")
		return
	}
	var input Input
	if !h.decode(w, r, &input) {
		return
	}
	result, err := h.store.Create(r.Context(), chi.URLParam(r, "orgID"), requestctx.UserID(r.Context()), employee, input)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	respond(w, 201, result)
}
func (h *Handler) decide(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !uuid.MatchString(id) {
		h.fail(w, r, 422, "INVALID_ID", "Invalid request ID.")
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if !h.decode(w, r, &input) {
		return
	}
	path := strings.Split(r.URL.Path, "/")
	result, err := h.store.Decide(r.Context(), chi.URLParam(r, "orgID"), requestctx.UserID(r.Context()), id, path[len(path)-1], input.Note)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	respond(w, 200, result)
}
func (h *Handler) failure(w http.ResponseWriter, r *http.Request, err error) {
	var conflict *Conflict
	switch {
	case errors.As(err, &conflict):
		body := map[string]any{"code": "ASSIGNMENT_CONFLICT", "message": conflict.Error(), "request_id": middleware.GetReqID(r.Context())}
		if len(conflict.Shifts) > 0 {
			body["conflicting_shifts"] = conflict.Shifts
		}
		respond(w, 409, body)
	case errors.Is(err, ErrNotFound):
		h.fail(w, r, 404, "NOT_FOUND", "The request or linked employee was not found. Ask your manager to link your staff record if needed.")
	case errors.Is(err, ErrForbidden):
		h.fail(w, r, 403, "FORBIDDEN", "Manager access is required.")
	case errors.Is(err, ErrState):
		h.fail(w, r, 409, "INVALID_TRANSITION", "Only pending leave can be approved or rejected. Only pending or approved time off can be cancelled.")
	case errors.Is(err, ErrInvalid):
		h.fail(w, r, 422, "INVALID_TIME_OFF", "Choose an active employee and a valid interval ending in the future, lasting at most 366 elapsed days. Notes may contain at most 500 characters.")
	default:
		h.logger.Error("time off request failed", "request_id", middleware.GetReqID(r.Context()), "error", err)
		h.fail(w, r, 503, "SERVICE_UNAVAILABLE", "The service is unavailable. Please try again.")
	}
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	respond(w, status, map[string]string{"code": code, "message": message, "request_id": middleware.GetReqID(r.Context())})
}
func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
