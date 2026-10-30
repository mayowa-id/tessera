package staff

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
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Handler struct {
	store     Store
	authorize func(http.Handler) http.Handler
	logger    *slog.Logger
}

func NewHandler(store Store, authorize func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{store, authorize, logger}
}
func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.authorize)
		base := "/api/organizations/{orgID}"
		r.Get(base+"/employees", h.listEmployees)
		r.Get(base+"/employees/{id}", h.getEmployee)
		r.Post(base+"/employees", h.saveEmployee)
		r.Put(base+"/employees/{id}", h.saveEmployee)
		for _, kind := range []string{"locations", "job-roles"} {
			r.Get(base+"/"+kind, func(w http.ResponseWriter, r *http.Request) { h.listCatalog(w, r, kind) })
			r.Get(base+"/"+kind+"/{id}", func(w http.ResponseWriter, r *http.Request) { h.getCatalog(w, r, kind) })
			r.Post(base+"/"+kind, func(w http.ResponseWriter, r *http.Request) { h.saveCatalog(w, r, kind) })
			r.Put(base+"/"+kind+"/{id}", func(w http.ResponseWriter, r *http.Request) { h.saveCatalog(w, r, kind) })
		}
	})
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) (Page, bool) {
	page := Page{Limit: 25, Search: strings.TrimSpace(r.URL.Query().Get("q"))}
	if utf8.RuneCountInString(page.Search) > 120 {
		h.fail(w, r, 422, "INVALID_PAGE", "Search must be at most 120 characters.")
		return page, false
	}
	for name, target := range map[string]*int32{"offset": &page.Offset, "limit": &page.Limit} {
		if text := r.URL.Query().Get(name); text != "" {
			value, err := strconv.ParseInt(text, 10, 32)
			if err != nil {
				h.fail(w, r, 422, "INVALID_PAGE", "Use an offset of 0 to 100000 and a limit of 1 to 100.")
				return page, false
			}
			*target = int32(value)
		}
	}
	if page.Offset < 0 || page.Offset > 100000 || page.Limit < 1 || page.Limit > 100 {
		h.fail(w, r, 422, "INVALID_PAGE", "Use an offset of 0 to 100000 and a limit of 1 to 100.")
		return page, false
	}
	if text := r.URL.Query().Get("include_inactive"); text != "" {
		value, err := strconv.ParseBool(text)
		if err != nil {
			h.fail(w, r, 422, "INVALID_PAGE", "include_inactive must be true or false.")
			return page, false
		}
		page.IncludeInactive = value
	}
	return page, true
}
func pageResponse[T any](w http.ResponseWriter, items []T, page Page) {
	hasMore := len(items) > int(page.Limit)
	if hasMore {
		items = items[:page.Limit]
	}
	if items == nil {
		items = []T{}
	}
	respond(w, 200, map[string]any{"items": items, "offset": page.Offset, "limit": page.Limit, "has_more": hasMore})
}
func (h *Handler) itemID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !uuidPattern.MatchString(id) {
		h.fail(w, r, 404, "NOT_FOUND", "Record not found.")
		return "", false
	}
	return strings.ToLower(id), true
}
func (h *Handler) decode(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send a JSON object.")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send one JSON object with supported fields.")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send one JSON object.")
		return false
	}
	return true
}
func validName(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 120
}

func (h *Handler) listEmployees(w http.ResponseWriter, r *http.Request) {
	page, ok := h.page(w, r)
	if !ok {
		return
	}
	rows, err := h.store.Employees(r.Context(), chi.URLParam(r, "orgID"), page)
	if err != nil {
		h.storeFailure(w, r, err)
		return
	}
	pageResponse(w, rows, page)
}
func (h *Handler) getEmployee(w http.ResponseWriter, r *http.Request) {
	id, ok := h.itemID(w, r)
	if !ok {
		return
	}
	employee, err := h.store.Employee(r.Context(), chi.URLParam(r, "orgID"), id)
	if err != nil {
		h.storeFailure(w, r, err)
		return
	}
	respond(w, 200, employee)
}
func (h *Handler) saveEmployee(w http.ResponseWriter, r *http.Request) {
	id := ""
	if r.Method == http.MethodPut {
		var ok bool
		id, ok = h.itemID(w, r)
		if !ok {
			return
		}
	}
	var input struct {
		DisplayName string    `json:"display_name"`
		Active      *bool     `json:"active"`
		JobRoleIDs  *[]string `json:"job_role_ids"`
	}
	if !h.decode(w, r, &input) {
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !validName(input.DisplayName) || input.Active == nil || input.JobRoleIDs == nil || len(*input.JobRoleIDs) > 100 {
		h.fail(w, r, 422, "INVALID_EMPLOYEE", "Provide a name of 1 to 120 characters, active, and up to 100 job_role_ids.")
		return
	}
	seen := map[string]bool{}
	for i, roleID := range *input.JobRoleIDs {
		roleID = strings.ToLower(roleID)
		if !uuidPattern.MatchString(roleID) || seen[roleID] {
			h.fail(w, r, 422, "INVALID_QUALIFICATIONS", "Job role IDs must be unique UUIDs.")
			return
		}
		seen[roleID] = true
		(*input.JobRoleIDs)[i] = roleID
	}
	employee, err := h.store.SaveEmployee(r.Context(), chi.URLParam(r, "orgID"), id, EmployeeInput{input.DisplayName, *input.Active, *input.JobRoleIDs})
	if err != nil {
		h.storeFailure(w, r, err)
		return
	}
	status := 200
	if id == "" {
		status = 201
		w.Header().Set("Location", r.URL.Path+"/"+employee.ID)
	}
	respond(w, status, employee)
}
func (h *Handler) listCatalog(w http.ResponseWriter, r *http.Request, kind string) {
	page, ok := h.page(w, r)
	if !ok {
		return
	}
	rows, err := h.store.Catalog(r.Context(), chi.URLParam(r, "orgID"), kind, page)
	if err != nil {
		h.storeFailure(w, r, err)
		return
	}
	pageResponse(w, rows, page)
}
func (h *Handler) getCatalog(w http.ResponseWriter, r *http.Request, kind string) {
	id, ok := h.itemID(w, r)
	if !ok {
		return
	}
	item, err := h.store.CatalogItem(r.Context(), chi.URLParam(r, "orgID"), kind, id)
	if err != nil {
		h.storeFailure(w, r, err)
		return
	}
	respond(w, 200, item)
}
func (h *Handler) saveCatalog(w http.ResponseWriter, r *http.Request, kind string) {
	id := ""
	if r.Method == http.MethodPut {
		var ok bool
		id, ok = h.itemID(w, r)
		if !ok {
			return
		}
	}
	var input struct {
		Name   string `json:"name"`
		Active *bool  `json:"active"`
	}
	if !h.decode(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || input.Active == nil {
		h.fail(w, r, 422, "INVALID_RECORD", "Provide a name of 1 to 120 characters and active.")
		return
	}
	item, err := h.store.SaveCatalogItem(r.Context(), chi.URLParam(r, "orgID"), kind, id, CatalogInput{input.Name, *input.Active})
	if err != nil {
		h.storeFailure(w, r, err)
		return
	}
	status := 200
	if id == "" {
		status = 201
		w.Header().Set("Location", r.URL.Path+"/"+item.ID)
	}
	respond(w, status, item)
}
func (h *Handler) storeFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		h.fail(w, r, 404, "NOT_FOUND", "Record not found.")
	case errors.Is(err, ErrRole):
		h.fail(w, r, 422, "INVALID_QUALIFICATIONS", "Choose roles from this organization. Inactive roles cannot be newly assigned.")
	case errors.Is(err, ErrNameInUse):
		h.fail(w, r, 409, "NAME_IN_USE", "That name is already in use in this organization.")
	case errors.Is(err, ErrPublishedAssignment):
		h.fail(w, r, 409, "PUBLISHED_ASSIGNMENT_PROTECTED", "A future published shift needs this record or qualification. Keep it active and retain the qualification until those shifts end.")
	default:
		h.logger.Error("staff request failed", "request_id", middleware.GetReqID(r.Context()), "error", err)
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
