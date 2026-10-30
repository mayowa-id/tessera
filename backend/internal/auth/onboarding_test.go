package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type captureMailer struct {
	body  string
	fail  bool
	count int
}

func (m *captureMailer) Send(_ context.Context, _, _, body string) error {
	m.body = body
	m.count++
	if m.fail {
		return errors.New("test delivery failure")
	}
	return nil
}
func (m *captureMailer) token() string { return strings.Fields(strings.SplitN(m.body, "#", 2)[1])[0] }

func testOnboarding(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	h, err := NewHandler(NewPostgresStore(pool), "https://app.example.com", true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mail := &captureMailer{}
	router := chi.NewRouter()
	h.Register(router)
	NewOnboarding(h, pool, mail).Register(router)
	post := func(path, body string, want int, cookie *http.Cookie, csrf string) string {
		t.Helper()
		w := request(router, "POST", path, body, cookie, csrf)
		if w.Code != want {
			t.Fatalf("%s: got %d want %d: %s", path, w.Code, want, w.Body)
		}
		return w.Body.String()
	}
	post("/api/auth/signup", `{"email":"owner@example.com","password":"strong-password","name":"Owner","team":"Public Test","location":"First site","timezone":"Africa/Lagos"}`, 202, nil, "")
	token := mail.token()
	post("/api/auth/login", `{"email":"owner@example.com","password":"strong-password"}`, 403, nil, "")
	post("/api/auth/verify", `{"token":"`+token+`","password":"wrong-password"}`, 400, nil, "")
	post("/api/auth/verify", `{"token":"`+token+`","password":"strong-password"}`, 200, nil, "")
	post("/api/auth/verify", `{"token":"`+token+`","password":"strong-password"}`, 400, nil, "")
	w := request(router, "POST", "/api/auth/login", `{"email":"owner@example.com","password":"strong-password"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("verified login %s", w.Body)
	}
	var identity struct {
		Memberships []Membership `json:"memberships"`
		CSRF        string       `json:"csrf_token"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if len(identity.Memberships) != 1 || identity.Memberships[0].Role != "admin" {
		t.Fatal("owner membership missing")
	}
	org := identity.Memberships[0].OrgID
	cookie := w.Result().Cookies()[0]
	csrf := identity.CSRF
	var employees, locations, qualifications int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM employees WHERE org_id=$1),(SELECT count(*) FROM locations WHERE org_id=$1),(SELECT count(*) FROM employee_roles WHERE org_id=$1)`, org).Scan(&employees, &locations, &qualifications)
	if err != nil || employees != 1 || locations != 1 || qualifications != 1 {
		t.Fatalf("atomic team setup: %d %d %d %v", employees, locations, qualifications, err)
	}
	path := "/api/organizations/" + org + "/invitations"
	post(path, `{"email":"new-member@example.com","name":"New Member","role":"manager"}`, 401, nil, "")
	post(path, `{"email":"new-member@example.com","name":"New Member","role":"manager"}`, 403, cookie, "invalid")
	post(path, `{"email":"new-member@example.com","name":"New Member","role":"manager"}`, 202, cookie, csrf)
	old := mail.token()
	post(path, `{"email":"new-member@example.com","name":"New Member","role":"manager"}`, 202, cookie, csrf)
	invite := mail.token()
	post("/api/auth/join", `{"token":"`+old+`","password":"member-password"}`, 400, nil, "")
	post("/api/auth/join", `{"token":"`+invite+`","password":"member-password"}`, 200, nil, "")
	post("/api/auth/join", `{"token":"`+invite+`","password":"member-password"}`, 400, nil, "")
	memberLogin := request(router, "POST", "/api/auth/login", `{"email":"new-member@example.com","password":"member-password"}`, nil, "")
	if memberLogin.Code != 200 {
		t.Fatal(memberLogin.Body)
	}
	var memberIdentity struct {
		CSRF string `json:"csrf_token"`
	}
	_ = json.Unmarshal(memberLogin.Body.Bytes(), &memberIdentity)
	post(path, `{"email":"third@example.com","name":"Third","role":"manager"}`, 403, memberLogin.Result().Cookies()[0], memberIdentity.CSRF)
	// Existing users must know the existing password. Invitation acceptance never replaces it.
	var otherOrg string
	err = pool.QueryRow(ctx, `INSERT INTO organizations(slug,name)VALUES('onboarding-other','Other Team') RETURNING id::text`).Scan(&otherOrg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO memberships(org_id,user_id,role) SELECT $1,id,'admin' FROM users WHERE email='owner@example.com'`, otherOrg)
	if err != nil {
		t.Fatal(err)
	}
	post("/api/organizations/"+otherOrg+"/invitations", `{"email":"new-member@example.com","name":"New Member"}`, 202, cookie, csrf)
	invite = mail.token()
	post("/api/auth/join", `{"token":"`+invite+`","password":"different-password"}`, 400, nil, "")
	post("/api/auth/join", `{"token":"`+invite+`","password":"member-password"}`, 200, nil, "")
	post("/api/auth/forgot-password", `{"email":"new-member@example.com"}`, 202, nil, "")
	reset := mail.token()
	post("/api/auth/reset-password", `{"token":"`+reset+`","password":"replacement-password"}`, 200, nil, "")
	if request(router, "GET", "/api/auth/me", "", memberLogin.Result().Cookies()[0], "").Code != 401 {
		t.Fatal("reset did not revoke sessions")
	}
	post("/api/auth/login", `{"email":"new-member@example.com","password":"member-password"}`, 401, nil, "")
	post("/api/auth/login", `{"email":"new-member@example.com","password":"replacement-password"}`, 200, nil, "")
	// Failed delivery leaves a recoverable pending account, with no session or team access.
	mail.fail = true
	post("/api/auth/signup", `{"email":"retry@example.com","password":"strong-password","name":"Retry","team":"Retry Team","location":"Site"}`, 503, nil, "")
	mail.fail = false
	post("/api/auth/resend", `{"email":"retry@example.com"}`, 202, nil, "")
	token = mail.token()
	post("/api/auth/verify", `{"token":"`+token+`","password":"strong-password"}`, 200, nil, "")
	// Each address is throttled even when the account does not exist.
	for range 5 {
		post("/api/auth/resend", `{"email":"unknown@example.com"}`, 202, nil, "")
	}
	post("/api/auth/resend", `{"email":"unknown@example.com"}`, 429, nil, "")
	// Do not affect the foundation fixture's counts.
	_, err = pool.Exec(ctx, `DELETE FROM sessions WHERE user_id IN(SELECT id FROM users WHERE email IN('owner@example.com','new-member@example.com','retry@example.com'))`)
	if err != nil {
		t.Fatal(err)
	}
}
