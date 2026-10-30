package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/audit"
	"tessera/backend/internal/demo"
	"tessera/backend/internal/requestctx"
	"tessera/backend/internal/roster"
	"tessera/backend/internal/staff"
)

// CI supplies TEST_DATABASE_URL. Each run owns an isolated schema, never application tables.
func TestPostgresIdentity(t *testing.T) {
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("set TEST_DATABASE_URL to run real PostgreSQL identity and tenant tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	random := make([]byte, 8)
	if _, err = rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := "identity_test_" + hex.EncodeToString(random)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("clean schema: %v", err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	files, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.TrimPrefix(strings.Split(string(content), "-- +goose Down")[0], "-- +goose Up")
		if _, err = pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	t.Run("public team onboarding", func(t *testing.T) { testOnboarding(t, pool) })
	// Repeated seeding is intentional: no duplicate staff, roles, or memberships.
	for range 2 {
		if err = demo.Seed(ctx, pool, "correct-password"); err != nil {
			t.Fatal(err)
		}
	}
	store := NewPostgresStore(pool)
	handler, err := NewHandler(store, "https://app.example.com", true, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	handler.Register(router)
	staff.NewHandler(staff.NewPostgresStore(pool), handler.RequireManager, slog.New(slog.NewJSONHandler(io.Discard, nil))).Register(router)
	roster.NewHandler(roster.NewPostgresStore(pool), handler.RequireManager, handler.RequireMember, slog.New(slog.NewJSONHandler(io.Discard, nil))).Register(router)
	audit.NewHandler(pool, handler.RequireManager, slog.New(slog.NewJSONHandler(io.Discard, nil))).Register(router)
	w := request(router, "POST", "/api/auth/login", `{"email":"manager@tessera.local","password":"correct-password"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("database login: %d %s", w.Code, w.Body)
	}
	cookie := w.Result().Cookies()[0]
	user, err := store.FindUser(ctx, "manager@tessera.local")
	if err != nil {
		t.Fatal(err)
	}
	ctx = requestctx.WithUser(ctx, user.ID)
	memberships, err := store.Memberships(ctx, user.ID)
	if err != nil || len(memberships) != 1 {
		t.Fatalf("memberships: %+v %v", memberships, err)
	}
	orgID := memberships[0].OrgID
	summary := request(router, "GET", "/api/organizations/"+orgID+"/summary", "", cookie, "")
	if summary.Code != 200 || !strings.Contains(summary.Body.String(), `"employees":2`) {
		t.Fatalf("seed summary: %d %s", summary.Code, summary.Body)
	}
	var foreignOrg, foreignRole string
	if err = pool.QueryRow(ctx, `INSERT INTO organizations (slug,name) VALUES ('foreign','Other organization') RETURNING id::text`).Scan(&foreignOrg); err != nil {
		t.Fatal(err)
	}
	if request(router, "GET", "/api/organizations/"+foreignOrg+"/summary", "", cookie, "").Code != 404 {
		t.Fatal("cross-tenant read allowed")
	}
	if err = pool.QueryRow(ctx, `INSERT INTO job_roles (org_id,name) VALUES ($1,'Foreign role') RETURNING id::text`, foreignOrg).Scan(&foreignRole); err != nil {
		t.Fatal(err)
	}
	var employeeID string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM employees WHERE org_id=$1 LIMIT 1`, orgID).Scan(&employeeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO employee_roles (org_id,employee_id,job_role_id) VALUES ($1,$2,$3)`, orgID, employeeID, foreignRole); err == nil {
		t.Fatal("database accepted a cross-tenant qualification")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO employees (org_id,user_id,display_name) VALUES ($1,$2,'Intruder')`, foreignOrg, user.ID); err == nil {
		t.Fatal("database accepted staff linked to another tenant's membership")
	}
	employeeLogin := request(router, "POST", "/api/auth/login", `{"email":"employee@tessera.local","password":"correct-password"}`, nil, "")
	if employeeLogin.Code != 200 {
		t.Fatalf("employee login: %s", employeeLogin.Body)
	}
	if request(router, "GET", "/api/organizations/"+orgID+"/summary", "", employeeLogin.Result().Cookies()[0], "").Code != 403 {
		t.Fatal("employee manager access allowed")
	}
	exerciseStaffAdministration(t, ctx, pool, router, cookie, employeeLogin.Result().Cookies()[0], csrfToken(cookie.Value), orgID, foreignOrg, foreignRole)
	exerciseDraftRosters(t, ctx, pool, router, cookie, employeeLogin.Result().Cookies()[0], csrfToken(cookie.Value), orgID, foreignOrg, foreignRole)
	exercisePublication(t, ctx, pool, router, cookie, employeeLogin.Result().Cookies()[0], csrfToken(cookie.Value), orgID, foreignOrg)
	if _, err = pool.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, tokenHash(cookie.Value)); err != nil {
		t.Fatal(err)
	}
	if request(router, "GET", "/api/auth/me", "", cookie, "").Code != 401 {
		t.Fatal("database expiry ignored")
	}
	for range 11 {
		if _, err = store.LoginAttempt(ctx, tokenHash("limiter-test")); err != nil {
			t.Fatal(err)
		}
	}
	attempts, err := store.LoginAttempt(ctx, tokenHash("limiter-test"))
	if err != nil || attempts != 12 {
		t.Fatalf("database throttle: %d %v", attempts, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE login_limits SET window_end=now()-interval '1 second' WHERE key_hash=$1`, tokenHash("limiter-test")); err != nil {
		t.Fatal(err)
	}
	attempts, err = store.LoginAttempt(ctx, tokenHash("limiter-test"))
	if err != nil || attempts != 1 {
		t.Fatal("database throttle window did not reset")
	}
	// Confirm the handler also works through a normal HTTP server transport.
	server := httptest.NewServer(router)
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/api/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("anonymous transport request allowed")
	}
}
