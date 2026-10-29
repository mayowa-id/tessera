package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This runs inside TestPostgresIdentity's isolated schema, using real authorization and SQL.
func exerciseStaffAdministration(t *testing.T, ctx context.Context, pool *pgxpool.Pool, router http.Handler, manager, employee *http.Cookie, csrf, orgID, foreignOrg, foreignRole string) {
	t.Helper()
	base := "/api/organizations/" + orgID
	create := func(path, body string) string {
		t.Helper()
		response := request(router, "POST", base+path, body, manager, csrf)
		if response.Code != 201 {
			t.Fatalf("create %s: %d %s", path, response.Code, response.Body)
		}
		var record struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		return record.ID
	}
	qualifiedBody := func(name string, active bool, roles []string) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{"display_name": name, "active": active, "job_role_ids": roles})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	list := request(router, "GET", base+"/employees?limit=1", "", manager, "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"has_more":true`) {
		t.Fatalf("employee pagination: %d %s", list.Code, list.Body)
	}
	role := create("/job-roles", `{"name":"Clinical","active":true}`)
	conflict := request(router, "POST", base+"/job-roles", `{"name":"clinical","active":true}`, manager, csrf)
	if conflict.Code != 409 {
		t.Fatalf("case-insensitive role uniqueness: %d %s", conflict.Code, conflict.Body)
	}
	person := create("/employees", qualifiedBody("Jamie Lane", true, []string{role}))
	if request(router, "POST", base+"/employees", qualifiedBody("CSRF bypass", true, []string{}), manager, "").Code != 403 {
		t.Fatal("staff mutation accepted missing CSRF")
	}
	if request(router, "GET", base+"/employees", "", employee, "").Code != 403 {
		t.Fatal("employee read manager staff list")
	}
	if request(router, "POST", base+"/employees", qualifiedBody("Unauthorized", true, []string{}), employee, csrfToken(employee.Value)).Code != 403 {
		t.Fatal("employee created staff record")
	}
	if request(router, "GET", "/api/organizations/"+foreignOrg+"/employees", "", manager, "").Code != 404 {
		t.Fatal("cross-tenant collection exposed")
	}
	var foreignEmployee string
	if err := pool.QueryRow(ctx, `INSERT INTO employees (org_id,display_name) VALUES ($1,'Foreign employee') RETURNING id::text`, foreignOrg).Scan(&foreignEmployee); err != nil {
		t.Fatal(err)
	}
	if request(router, "GET", base+"/employees/"+foreignEmployee, "", manager, "").Code != 404 {
		t.Fatal("cross-tenant direct ID exposed")
	}
	if request(router, "PUT", base+"/employees/"+foreignEmployee, qualifiedBody("Intrusion", true, []string{}), manager, csrf).Code != 404 {
		t.Fatal("cross-tenant update succeeded")
	}
	invalid := request(router, "PUT", base+"/employees/"+person, qualifiedBody("Should not persist", false, []string{foreignRole}), manager, csrf)
	if invalid.Code != 422 {
		t.Fatalf("cross-tenant qualification accepted: %d %s", invalid.Code, invalid.Body)
	}
	unchanged := request(router, "GET", base+"/employees/"+person, "", manager, "")
	if unchanged.Code != 200 || !strings.Contains(unchanged.Body.String(), `"display_name":"Jamie Lane"`) || !strings.Contains(unchanged.Body.String(), role) || !strings.Contains(unchanged.Body.String(), `"active":true`) {
		t.Fatalf("failed qualification edit was not atomic: %s", unchanged.Body)
	}
	var before, after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM employees WHERE org_id=$1`, orgID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if request(router, "POST", base+"/employees", qualifiedBody("Invalid creation", true, []string{foreignRole}), manager, csrf).Code != 422 {
		t.Fatal("creation accepted foreign job role")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM employees WHERE org_id=$1`, orgID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("failed creation left a partial employee")
	}
	if request(router, "PUT", base+"/job-roles/"+role, `{"name":"Clinical","active":false}`, manager, csrf).Code != 200 {
		t.Fatal("role deactivation failed")
	}
	retained := request(router, "PUT", base+"/employees/"+person, qualifiedBody("Jamie Lane", false, []string{role}), manager, csrf)
	if retained.Code != 200 || !strings.Contains(retained.Body.String(), role) {
		t.Fatalf("deactivation lost existing qualification: %s", retained.Body)
	}
	hidden := request(router, "GET", base+"/employees?q=Jamie", "", manager, "")
	if hidden.Code != 200 || strings.Contains(hidden.Body.String(), person) {
		t.Fatal("inactive employee appeared in default list")
	}
	visible := request(router, "GET", base+"/employees?q=Jamie&include_inactive=true", "", manager, "")
	if visible.Code != 200 || !strings.Contains(visible.Body.String(), person) {
		t.Fatal("inactive employee history unavailable")
	}
	if request(router, "POST", base+"/employees", qualifiedBody("New inactive qualification", true, []string{role}), manager, csrf).Code != 422 {
		t.Fatal("inactive job role newly assigned")
	}
	if request(router, "PUT", base+"/employees/"+person, qualifiedBody("Jamie Lane", false, []string{}), manager, csrf).Code != 200 {
		t.Fatal("qualification removal failed")
	}
	if request(router, "PUT", base+"/employees/"+person, qualifiedBody("Jamie Lane", false, []string{role}), manager, csrf).Code != 422 {
		t.Fatal("removed inactive qualification was newly reassigned")
	}
	location := create("/locations", `{"name":"North site","active":true}`)
	if request(router, "POST", base+"/locations", `{"name":"NORTH SITE","active":true}`, manager, csrf).Code != 409 {
		t.Fatal("location uniqueness not enforced")
	}
	if request(router, "PUT", base+"/locations/"+location, `{"name":"North site","active":false}`, manager, csrf).Code != 200 {
		t.Fatal("location deactivation failed")
	}
	archivedLocation := request(router, "GET", base+"/locations/"+location, "", manager, "")
	if archivedLocation.Code != 200 || !strings.Contains(archivedLocation.Body.String(), `"active":false`) {
		t.Fatal("deactivated location record was deleted")
	}
}
