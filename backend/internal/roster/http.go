package roster

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
	authorize       func(http.Handler) http.Handler
	memberAuthorize func(http.Handler) http.Handler
	logger          *slog.Logger
}

func NewHandler(store Store, authorize, memberAuthorize func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{store: store, authorize: authorize, memberAuthorize: memberAuthorize, logger: logger}
}
func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.authorize)
		base := "/api/organizations/{orgID}/rosters"
		r.Get(base, h.list)
		r.Post(base, h.create)
		r.Get(base+"/{rosterID}", h.get)
		r.Post(base+"/{rosterID}/publish", h.transition)
		r.Post(base+"/{rosterID}/archive", h.transition)
		r.Post(base+"/{rosterID}/shifts", h.change)
		r.Put(base+"/{rosterID}/shifts/{id}", h.change)
		r.Delete(base+"/{rosterID}/shifts/{id}", h.change)
		r.Put(base+"/{rosterID}/shifts/{id}/assignment", h.change)
		r.Delete(base+"/{rosterID}/shifts/{id}/assignment", h.change)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.memberAuthorize)
		r.Get("/api/organizations/{orgID}/my-shifts", h.myShifts)
	})
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "rosterID")
	if !h.validID(w, r, id) {
		return
	}
	action := "publish"
	if strings.HasSuffix(r.URL.Path, "/archive") {
		action = "archive"
	}
	result, err := h.store.Transition(r.Context(), chi.URLParam(r, "orgID"), id, action)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	respond(w, 200, result)
}
func (h *Handler) myShifts(w http.ResponseWriter, r *http.Request) {
	from, first := time.Parse(time.RFC3339Nano, r.URL.Query().Get("from"))
	to, second := time.Parse(time.RFC3339Nano, r.URL.Query().Get("to"))
	if first != nil || second != nil || !to.After(from) || to.Sub(from) > 62*24*time.Hour {
		h.fail(w, r, 422, "INVALID_RANGE", "Choose an explicit-offset range of more than zero and at most 62 days.")
		return
	}
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
	result, err := h.store.MyShifts(r.Context(), chi.URLParam(r, "orgID"), requestctx.UserID(r.Context()), from, to, int32(limit), int32(offset))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	more := len(result) > int(limit)
	if more {
		result = result[:limit]
	}
	respond(w, 200, map[string]any{"items": result, "limit": limit, "offset": offset, "has_more": more})
}
func validWeek(value string) bool {
	parsed, err := time.Parse(time.DateOnly, value)
	return err == nil && parsed.Weekday() == time.Monday
}
func (h *Handler) decode(w http.ResponseWriter, r *http.Request, input any) bool {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send a JSON object.")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send supported JSON fields.")
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send one JSON object.")
		return false
	}
	return true
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	week := r.URL.Query().Get("week_start")
	if !validWeek(week) {
		h.fail(w, r, 422, "INVALID_WEEK", "Choose a Monday as week_start (YYYY-MM-DD).")
		return
	}
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
	rows, err := h.store.List(r.Context(), chi.URLParam(r, "orgID"), week, int32(limit), int32(offset))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	more := len(rows) > int(limit)
	if more {
		rows = rows[:limit]
	}
	respond(w, 200, map[string]any{"items": rows, "limit": limit, "offset": offset, "has_more": more})
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LocationID string `json:"location_id"`
		WeekStart  string `json:"week_start"`
	}
	if !h.decode(w, r, &input) {
		return
	}
	if !uuid.MatchString(input.LocationID) || !validWeek(input.WeekStart) {
		h.fail(w, r, 422, "INVALID_ROSTER", "Choose a location and a Monday week_start.")
		return
	}
	result, err := h.store.Create(r.Context(), chi.URLParam(r, "orgID"), input.LocationID, input.WeekStart)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	w.Header().Set("Location", r.URL.Path+"/"+result.ID)
	respond(w, 201, result)
}
func (h *Handler) validID(w http.ResponseWriter, r *http.Request, id string) bool {
	if !uuid.MatchString(id) {
		h.fail(w, r, 404, "NOT_FOUND", "Roster or shift not found.")
		return false
	}
	return true
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "rosterID")
	if !h.validID(w, r, id) {
		return
	}
	result, err := h.store.Get(r.Context(), chi.URLParam(r, "orgID"), id)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	respond(w, 200, result)
}
func (h *Handler) change(w http.ResponseWriter, r *http.Request) {
	rosterID, id := chi.URLParam(r, "rosterID"), chi.URLParam(r, "id")
	if !h.validID(w, r, rosterID) {
		return
	}
	if id != "" && !h.validID(w, r, id) {
		return
	}
	command := Command{ShiftID: id}
	assignment := chi.RouteContext(r.Context()).RoutePattern() == "/api/organizations/{orgID}/rosters/{rosterID}/shifts/{id}/assignment"
	if assignment {
		if r.Method == http.MethodDelete {
			command.Action = "unassign"
		} else {
			var input struct {
				EmployeeID string `json:"employee_id"`
			}
			if !h.decode(w, r, &input) {
				return
			}
			if !uuid.MatchString(input.EmployeeID) {
				h.fail(w, r, 422, "INVALID_EMPLOYEE", "Choose an employee.")
				return
			}
			command.Action = "assign"
			command.EmployeeID = input.EmployeeID
		}
	} else if r.Method == http.MethodDelete {
		command.Action = "delete"
	} else {
		var input struct {
			JobRoleID string `json:"job_role_id"`
			StartsAt  string `json:"starts_at"`
			EndsAt    string `json:"ends_at"`
		}
		if !h.decode(w, r, &input) {
			return
		}
		start, first := time.Parse(time.RFC3339Nano, input.StartsAt)
		end, second := time.Parse(time.RFC3339Nano, input.EndsAt)
		start = start.Truncate(time.Microsecond)
		end = end.Truncate(time.Microsecond)
		if !uuid.MatchString(input.JobRoleID) || first != nil || second != nil || !end.After(start) || end.Sub(start) > 24*time.Hour {
			h.fail(w, r, 422, "INVALID_SHIFT", "Choose a role and explicit-offset start/end times spanning more than zero and at most 24 hours.")
			return
		}
		command.Action = "update"
		if r.Method == http.MethodPost {
			command.Action = "create"
		}
		command.Input = ShiftInput{input.JobRoleID, start, end}
	}
	result, err := h.store.Change(r.Context(), chi.URLParam(r, "orgID"), rosterID, command)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if command.Action == "delete" {
		w.WriteHeader(204)
		return
	}
	status := 200
	if command.Action == "create" {
		status = 201
	}
	respond(w, status, result)
}
func (h *Handler) failure(w http.ResponseWriter, r *http.Request, err error) {
	var conflict *Conflict
	switch {
	case errors.As(err, &conflict):
		respond(w, 409, map[string]any{"code": "SCHEDULING_CONFLICT", "message": conflict.Error(), "violations": conflict.Violations, "request_id": middleware.GetReqID(r.Context())})
	case errors.Is(err, ErrNotFound):
		h.fail(w, r, 404, "NOT_FOUND", "Roster, shift, or related record not found.")
	case errors.Is(err, ErrDuplicate):
		h.fail(w, r, 409, "ROSTER_EXISTS", "A roster already exists for that location and week.")
	case errors.Is(err, ErrDraft):
		h.fail(w, r, 409, "ROSTER_IMMUTABLE", "Only a draft roster can be changed.")
	case errors.Is(err, ErrTransition):
		h.fail(w, r, 409, "INVALID_TRANSITION", "This roster cannot make that state transition.")
	case errors.Is(err, ErrNotEnded):
		h.fail(w, r, 422, "ROSTER_NOT_ENDED", "Archive only after the roster week and all its shifts have ended.")
	case errors.Is(err, ErrInvalid):
		h.fail(w, r, 422, "INVALID_SCHEDULE", "Check the active role, location, and shift times. Shifts must start within the roster week and last at most 24 hours.")
	default:
		h.logger.Error("roster request failed", "request_id", middleware.GetReqID(r.Context()), "error", err)
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
