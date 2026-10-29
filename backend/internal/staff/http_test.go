package staff

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const recordID = "11111111-1111-4111-8111-111111111111"
const roleID = "22222222-2222-4222-8222-222222222222"

type fakeStore struct {
	org, id, kind string
	input         EmployeeInput
	catalogInput  CatalogInput
	page          Page
	failure       error
	calls         int
}

func (s *fakeStore) Employees(_ context.Context, org string, page Page) ([]Employee, error) {
	s.org = org
	s.page = page
	s.calls++
	return []Employee{{ID: recordID, DisplayName: "Alex", JobRoleIDs: []string{}}, {ID: roleID, DisplayName: "Sam", JobRoleIDs: []string{}}}, s.failure
}
func (s *fakeStore) Employee(_ context.Context, org, id string) (Employee, error) {
	s.org = org
	s.id = id
	s.calls++
	return Employee{ID: id, DisplayName: "Alex", JobRoleIDs: []string{}}, s.failure
}
func (s *fakeStore) SaveEmployee(_ context.Context, org, id string, input EmployeeInput) (Employee, error) {
	s.org = org
	s.id = id
	s.input = input
	s.calls++
	return Employee{ID: recordID, DisplayName: input.DisplayName, Active: input.Active, JobRoleIDs: input.JobRoleIDs}, s.failure
}
func (s *fakeStore) Catalog(_ context.Context, org, kind string, page Page) ([]CatalogItem, error) {
	s.org = org
	s.kind = kind
	s.page = page
	s.calls++
	return []CatalogItem{}, s.failure
}
func (s *fakeStore) CatalogItem(_ context.Context, org, kind, id string) (CatalogItem, error) {
	s.org = org
	s.kind = kind
	s.id = id
	s.calls++
	return CatalogItem{ID: id, Name: "Main site", Active: true}, s.failure
}
func (s *fakeStore) SaveCatalogItem(_ context.Context, org, kind, id string, input CatalogInput) (CatalogItem, error) {
	s.org = org
	s.kind = kind
	s.id = id
	s.catalogInput = input
	s.calls++
	return CatalogItem{ID: recordID, Name: input.Name, Active: input.Active}, s.failure
}
func staffRouter(store *fakeStore) http.Handler {
	router := chi.NewRouter()
	NewHandler(store, func(next http.Handler) http.Handler { return next }, slog.New(slog.NewJSONHandler(io.Discard, nil))).Register(router)
	return router
}
func call(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
func TestEmployeeValidationAndScope(t *testing.T) {
	store := &fakeStore{}
	router := staffRouter(store)
	for _, body := range []string{
		`{"display_name":" ","active":true,"job_role_ids":[]}`,
		`{"display_name":"Alex","job_role_ids":[]}`,
		`{"display_name":"Alex","active":true,"job_role_ids":null}`,
		`{"display_name":"Alex","active":true,"job_role_ids":["bad-id"]}`,
		`{"display_name":"Alex","active":true,"job_role_ids":["` + roleID + `","` + roleID + `"]}`,
	} {
		if w := call(router, "POST", "/api/organizations/org-a/employees", body); w.Code != 422 {
			t.Fatalf("validation: %d %s", w.Code, w.Body)
		}
	}
	if store.calls != 0 {
		t.Fatal("invalid inputs reached persistence")
	}
	response := call(router, "POST", "/api/organizations/org-a/employees", `{"display_name":" Alex ","active":true,"job_role_ids":["`+roleID+`"]}`)
	if response.Code != 201 || store.org != "org-a" || store.input.DisplayName != "Alex" || len(store.input.JobRoleIDs) != 1 {
		t.Fatalf("create: %d %s %+v", response.Code, response.Body, store)
	}
	if response.Header().Get("Location") != "/api/organizations/org-a/employees/"+recordID {
		t.Fatal("missing created resource location")
	}
	response = call(router, "PUT", "/api/organizations/org-a/employees/"+recordID, `{"display_name":"Alex","active":false,"job_role_ids":[]}`)
	if response.Code != 200 || store.id != recordID || store.input.Active {
		t.Fatal("deactivation not persisted")
	}
}
func TestBoundedPagination(t *testing.T) {
	store := &fakeStore{}
	router := staffRouter(store)
	response := call(router, "GET", "/api/organizations/org-a/employees?limit=1&offset=3&q=Alex&include_inactive=true", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"has_more":true`) || strings.Contains(response.Body.String(), `"Sam"`) {
		t.Fatalf("page: %d %s", response.Code, response.Body)
	}
	if store.page.Limit != 1 || store.page.Offset != 3 || !store.page.IncludeInactive || store.page.Search != "Alex" {
		t.Fatalf("unexpected page %+v", store.page)
	}
	for _, query := range []string{"limit=0", "limit=101", "offset=-1", "offset=100001", "include_inactive=maybe", "limit=foo"} {
		if call(router, "GET", "/api/organizations/org-a/employees?"+query, "").Code != 422 {
			t.Fatalf("accepted invalid page %s", query)
		}
	}
}
func TestStaffErrorsAndCatalogRoutes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{ErrNotFound, 404, "NOT_FOUND"}, {ErrRole, 422, "INVALID_QUALIFICATIONS"}, {ErrNameInUse, 409, "NAME_IN_USE"}} {
		store := &fakeStore{failure: tc.err}
		response := call(staffRouter(store), "PUT", "/api/organizations/org-a/employees/"+recordID, `{"display_name":"Alex","active":true,"job_role_ids":[]}`)
		if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.code) {
			t.Fatalf("error mapping: %d %s", response.Code, response.Body)
		}
	}
	store := &fakeStore{}
	router := staffRouter(store)
	for _, kind := range []string{"locations", "job-roles"} {
		response := call(router, "POST", "/api/organizations/org-a/"+kind, `{"name":" Main ","active":true}`)
		if response.Code != 201 || store.kind != kind || store.org != "org-a" || store.catalogInput.Name != "Main" {
			t.Fatalf("catalog route %s: %d %s", kind, response.Code, response.Body)
		}
	}
	if call(router, "GET", "/api/organizations/org-a/employees/not-a-uuid", "").Code != 404 {
		t.Fatal("invalid record ID not rejected")
	}
	if call(router, "POST", "/api/organizations/org-a/employees", `{"display_name":"Alex","active":true,"job_role_ids":[],"org_id":"other-org"}`).Code != 400 {
		t.Fatal("tenant override in body accepted")
	}
}
