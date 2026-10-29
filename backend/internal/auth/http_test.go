package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type memorySession struct {
	userID  string
	expires time.Time
}
type memoryStore struct {
	users        map[string]User
	sessions     map[string]memorySession
	memberships  map[string][]Membership
	attempts     map[string]int32
	summaryCalls int
}

func (s *memoryStore) FindUser(_ context.Context, email string) (User, error) {
	u, ok := s.users[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
func (s *memoryStore) SessionUser(_ context.Context, hash []byte) (User, error) {
	session, ok := s.sessions[string(hash)]
	if ok && session.expires.After(time.Now()) {
		for _, u := range s.users {
			if u.ID == session.userID {
				return u, nil
			}
		}
	}
	return User{}, ErrNotFound
}
func (s *memoryStore) Memberships(_ context.Context, id string) ([]Membership, error) {
	return s.memberships[id], nil
}
func (s *memoryStore) CreateSession(_ context.Context, hash []byte, id string, expires time.Time) error {
	s.sessions[string(hash)] = memorySession{id, expires}
	return nil
}
func (s *memoryStore) DeleteSession(_ context.Context, hash []byte) error {
	delete(s.sessions, string(hash))
	return nil
}
func (s *memoryStore) LoginAttempt(_ context.Context, key []byte) (int32, error) {
	s.attempts[string(key)]++
	return s.attempts[string(key)], nil
}
func (s *memoryStore) Summary(context.Context, string) (Summary, error) {
	s.summaryCalls++
	return Summary{Employees: 2, Locations: 1, JobRoles: 1}, nil
}

func testHandler(t *testing.T) (http.Handler, *memoryStore) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{
		users:       map[string]User{"manager@example.com": {ID: "user-a", Email: "manager@example.com", DisplayName: "Alex", PasswordHash: string(hash)}},
		sessions:    make(map[string]memorySession),
		memberships: map[string][]Membership{"user-a": {{OrgID: "org-a", Name: "Team A", Role: "manager", Timezone: "Africa/Lagos"}}},
		attempts:    make(map[string]int32),
	}
	handler, err := NewHandler(store, "https://app.example.com", true, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	handler.Register(router)
	return router, store
}

func request(router http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tessera-Request", "1")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func login(t *testing.T, router http.Handler) (*http.Cookie, string) {
	t.Helper()
	w := request(router, "POST", "/api/auth/login", `{"email":"manager@example.com","password":"correct-password"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session cookie")
	}
	var body struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return cookies[0], body.CSRF
}

func TestSessionLifecycle(t *testing.T) {
	router, store := testHandler(t)
	cookie, csrf := login(t, router)
	if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/api" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe cookie settings: %+v", cookie)
	}
	if len(store.sessions) != 1 {
		t.Fatal("session not persisted")
	}
	if _, ok := store.sessions[cookie.Value]; ok {
		t.Fatal("raw cookie persisted instead of its hash")
	}
	me := request(router, "GET", "/api/auth/me", "", cookie, "")
	if me.Code != 200 || bytes.Contains(me.Body.Bytes(), []byte("password_hash")) {
		t.Fatalf("unexpected identity response: %d %s", me.Code, me.Body)
	}
	if request(router, "POST", "/api/auth/logout", "{}", cookie, "wrong-token").Code != 403 {
		t.Fatal("logout accepted invalid CSRF token")
	}
	if request(router, "POST", "/api/auth/logout", "{}", cookie, csrf).Code != 204 {
		t.Fatal("logout failed")
	}
	if request(router, "GET", "/api/auth/me", "", cookie, "").Code != 401 {
		t.Fatal("revoked session accepted")
	}
}

func TestOrganizationAuthorization(t *testing.T) {
	router, store := testHandler(t)
	cookie, _ := login(t, router)
	if request(router, "GET", "/api/organizations/org-a/summary", "", cookie, "").Code != 200 {
		t.Fatal("manager denied")
	}
	if request(router, "GET", "/api/organizations/org-b/summary", "", cookie, "").Code != 404 {
		t.Fatal("foreign organization exposed")
	}
	store.memberships["user-a"][0].Role = "employee"
	if request(router, "GET", "/api/organizations/org-a/summary", "", cookie, "").Code != 403 {
		t.Fatal("employee accessed manager endpoint")
	}
	if store.summaryCalls != 1 {
		t.Fatal("unauthorized request reached organization data query")
	}
	if request(router, "GET", "/api/organizations/org-a/summary", "", nil, "").Code != 401 {
		t.Fatal("anonymous access permitted")
	}
}

func TestLoginDefenses(t *testing.T) {
	router, store := testHandler(t)
	for _, origin := range []string{"", "null", "https://app.example.com.attacker.test"} {
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Tessera-Request", "1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatalf("accepted origin %q", origin)
		}
	}
	if len(store.attempts) != 0 {
		t.Fatal("untrusted origin reached login store")
	}
	for _, email := range []string{"manager@example.com", "unknown@example.com"} {
		w := request(router, "POST", "/api/auth/login", `{"email":"`+email+`","password":"wrong-password"}`, nil, "")
		if w.Code != 401 || !strings.Contains(w.Body.String(), "INVALID_CREDENTIALS") {
			t.Fatalf("account enumeration response: %d %s", w.Code, w.Body)
		}
	}
	for range 9 {
		request(router, "POST", "/api/auth/login", `{"email":"manager@example.com","password":"wrong-password"}`, nil, "")
	}
	limited := request(router, "POST", "/api/auth/login", `{"email":"manager@example.com","password":"correct-password"}`, nil, "")
	if limited.Code != 429 || limited.Header().Get("Retry-After") == "" {
		t.Fatal("missing rate limit")
	}
}

func TestExpiredAndRotatedSessions(t *testing.T) {
	router, store := testHandler(t)
	old, _ := login(t, router)
	rotated := request(router, "POST", "/api/auth/login", `{"email":"manager@example.com","password":"correct-password"}`, old, "")
	if rotated.Code != 200 {
		t.Fatalf("rotation failed: %s", rotated.Body)
	}
	if request(router, "GET", "/api/auth/me", "", old, "").Code != 401 {
		t.Fatal("old session survives reauthentication")
	}
	fresh := rotated.Result().Cookies()[0]
	session := store.sessions[string(tokenHash(fresh.Value))]
	session.expires = time.Now().Add(-time.Minute)
	store.sessions[string(tokenHash(fresh.Value))] = session
	if request(router, "GET", "/api/auth/me", "", fresh, "").Code != 401 {
		t.Fatal("expired session accepted")
	}
}

func TestManagerMiddlewareProtectsMutations(t *testing.T) {
	_, store := testHandler(t)
	handler, err := NewHandler(store, "https://app.example.com", true, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	handler.Register(router)
	calls := 0
	router.Group(func(r chi.Router) {
		r.Use(handler.RequireManager)
		r.Post("/api/organizations/{orgID}/protected", func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) })
	})
	cookie, csrf := login(t, router)
	if request(router, "POST", "/api/organizations/org-a/protected", "{}", cookie, "").Code != 403 {
		t.Fatal("mutation accepted without CSRF")
	}
	if request(router, "POST", "/api/organizations/org-b/protected", "{}", cookie, csrf).Code != 404 {
		t.Fatal("cross-tenant mutation accepted")
	}
	if request(router, "POST", "/api/organizations/org-a/protected", "{}", cookie, csrf).Code != 204 {
		t.Fatal("manager mutation rejected")
	}
	store.memberships["user-a"][0].Role = "employee"
	if request(router, "POST", "/api/organizations/org-a/protected", "{}", cookie, csrf).Code != 403 {
		t.Fatal("employee mutation accepted")
	}
	if calls != 1 {
		t.Fatal("unauthorized request reached mutation handler")
	}
}
