package platform

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
)

type fakeDB struct {
	err   error
	calls int
}

func (db *fakeDB) Ping(context.Context) error { db.calls++; return db.err }

func TestHealthAndReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, path    string
		failure       error
		status, calls int
	}{
		{"liveness does not require database", "/api/healthz", errors.New("offline"), 200, 0},
		{"ready database", "/api/readyz", nil, 200, 1},
		{"unavailable database", "/api/readyz", errors.New("offline"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDB{err: tc.failure}
			w := httptest.NewRecorder()
			Router(db, slog.New(slog.NewJSONHandler(io.Discard, nil))).ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.status || db.calls != tc.calls {
				t.Fatalf("status=%d calls=%d", w.Code, db.calls)
			}
			if w.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing request ID")
			}
		})
	}
}
