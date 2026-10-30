package roster

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)
import "github.com/go-chi/chi/v5"

func chiRouter(store Store) http.Handler {
	router := chi.NewRouter()
	NewHandler(store, func(next http.Handler) http.Handler { return next }, slog.New(slog.NewJSONHandler(io.Discard, nil))).Register(router)
	return router
}

const testID = "11111111-1111-4111-8111-111111111111"

type fakeStore struct {
	command Command
	calls   int
}

func (s *fakeStore) List(context.Context, string, string, int32, int32) ([]Roster, error) {
	s.calls++
	return []Roster{}, nil
}
func (s *fakeStore) Create(context.Context, string, string, string) (Roster, error) {
	s.calls++
	return Roster{ID: testID}, nil
}
func (s *fakeStore) Get(context.Context, string, string) (Detail, error) {
	s.calls++
	return Detail{}, nil
}
func (s *fakeStore) Change(_ context.Context, org, id string, command Command) (Result, error) {
	s.calls++
	s.command = command
	return Result{}, nil
}
func TestDraftRouteValidation(t *testing.T) {
	store := &fakeStore{}
	router := chiRouter(store)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	root := "/api/organizations/org-a/rosters"
	if call("GET", root+"?week_start=2026-11-03", "").Code != 422 {
		t.Fatal("non-Monday accepted")
	}
	if call("POST", root, `{"location_id":"`+testID+`","week_start":"2026-11-02","org_id":"foreign"}`).Code != 400 {
		t.Fatal("tenant override accepted")
	}
	path := root + "/" + testID + "/shifts/" + testID
	if call("PUT", path, `{"job_role_id":"`+testID+`","starts_at":"2026-11-02T10:00:00Z","ends_at":"2026-11-02T09:00:00Z"}`).Code != 422 {
		t.Fatal("reversed interval accepted")
	}
	if store.calls != 0 {
		t.Fatal("invalid input reached database")
	}
	response := call("PUT", path+"/assignment", `{"employee_id":"`+testID+`"}`)
	if response.Code != 200 || store.command.Action != "assign" || store.command.EmployeeID != testID {
		t.Fatalf("assignment route: %d %+v", response.Code, store.command)
	}
	if call("DELETE", path+"/assignment", "").Code != 200 || store.command.Action != "unassign" {
		t.Fatal("unassign route incorrect")
	}
	if call("DELETE", path, "").Code != 204 || store.command.Action != "delete" {
		t.Fatal("delete route incorrect")
	}
}
