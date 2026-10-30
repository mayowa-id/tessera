package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/requestctx"
	"tessera/backend/internal/roster"
	"tessera/backend/internal/scheduling"
)

func exercisePublication(t *testing.T, ctx context.Context, pool *pgxpool.Pool, router http.Handler, manager, employee *http.Cookie, csrf, org, foreign string) {
	t.Helper()
	base := "/api/organizations/" + org
	rosters := base + "/rosters"
	zone, _ := time.LoadLocation("Africa/Lagos")
	week := scheduling.WeekStart(time.Now(), zone).AddDate(0, 0, 140)
	var site, role, sam string
	for _, query := range []struct {
		sql    string
		target *string
	}{{`SELECT id::text FROM locations WHERE org_id=$1 AND name='Main site'`, &site}, {`SELECT id::text FROM job_roles WHERE org_id=$1 AND name='Team member'`, &role}, {`SELECT id::text FROM employees WHERE org_id=$1 AND display_name='Sam Taylor'`, &sam}} {
		if err := pool.QueryRow(ctx, query.sql, org).Scan(query.target); err != nil {
			t.Fatal(err)
		}
	}
	auditCount := func() int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE org_id=$1`, org).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	createRoster := func(start string) string {
		t.Helper()
		response := request(router, "POST", rosters, fmt.Sprintf(`{"location_id":%q,"week_start":%q}`, site, start), manager, csrf)
		if response.Code != 201 {
			t.Fatalf("create lifecycle roster: %d %s", response.Code, response.Body)
		}
		var result roster.Roster
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.ID
	}
	createShift := func(rosterID string, start time.Time) string {
		t.Helper()
		response := request(router, "POST", rosters+"/"+rosterID+"/shifts", fmt.Sprintf(`{"job_role_id":%q,"starts_at":%q,"ends_at":%q}`, role, start.Format(time.RFC3339), start.Add(8*time.Hour).Format(time.RFC3339)), manager, csrf)
		if response.Code != 201 {
			t.Fatalf("create lifecycle shift: %d %s", response.Code, response.Body)
		}
		var result roster.Result
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Shift.ID
	}
	assign := func(rosterID, shiftID, employeeID string) int {
		return request(router, "PUT", rosters+"/"+rosterID+"/shifts/"+shiftID+"/assignment", fmt.Sprintf(`{"employee_id":%q}`, employeeID), manager, csrf).Code
	}
	id := createRoster(week.Format(time.DateOnly))
	publish := rosters + "/" + id + "/publish"
	count := auditCount()
	empty := request(router, "POST", publish, "{}", manager, csrf)
	if empty.Code != 409 || !strings.Contains(empty.Body.String(), "EMPTY_ROSTER") || auditCount() != count {
		t.Fatal("empty publication was allowed or audited")
	}
	first := createShift(id, week.Add(9*time.Hour))
	if result := request(router, "POST", publish, "{}", manager, csrf); result.Code != 409 || !strings.Contains(result.Body.String(), "UNASSIGNED_SHIFT") {
		t.Fatal("unassigned roster published")
	}
	employeeInput := func(active bool, roles []string) string {
		body, _ := json.Marshal(map[string]any{"display_name": "Publication candidate", "active": active, "job_role_ids": roles})
		return string(body)
	}
	created := request(router, "POST", base+"/employees", employeeInput(true, []string{role}), manager, csrf)
	if created.Code != 201 {
		t.Fatalf("candidate employee: %s", created.Body)
	}
	var candidate struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if assign(id, first, candidate.ID) != 200 {
		t.Fatal("candidate assignment failed")
	}
	if request(router, "PUT", base+"/employees/"+candidate.ID, employeeInput(true, []string{}), manager, csrf).Code != 200 {
		t.Fatal("draft qualification edit failed")
	}
	count = auditCount()
	invalid := request(router, "POST", publish, "{}", manager, csrf)
	if invalid.Code != 409 || !strings.Contains(invalid.Body.String(), "QUALIFICATION_REQUIRED") || auditCount() != count {
		t.Fatal("publish did not revalidate qualifications atomically")
	}
	if request(router, "PUT", base+"/employees/"+candidate.ID, employeeInput(true, []string{role}), manager, csrf).Code != 200 {
		t.Fatal("qualification restoration failed")
	}
	own := createShift(id, week.AddDate(0, 0, 1).Add(9*time.Hour))
	if assign(id, own, sam) != 200 {
		t.Fatal("employee's shift assignment failed")
	}
	rangeQuery := func(from, to time.Time) string {
		return "?from=" + url.QueryEscape(from.Format(time.RFC3339)) + "&to=" + url.QueryEscape(to.Format(time.RFC3339))
	}
	period := rangeQuery(week, week.AddDate(0, 0, 7))
	var warningRoster string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM rosters WHERE org_id=$1 AND location_id=$2 AND week_start='2026-11-30'`, org, site).Scan(&warningRoster); err != nil {
		t.Fatal(err)
	}
	warnings := request(router, "POST", rosters+"/"+warningRoster+"/publish", "{}", manager, csrf)
	if warnings.Code != 200 || !strings.Contains(warnings.Body.String(), "WEEKLY_HOURS") {
		t.Fatalf("warning-only publication should succeed: %d %s", warnings.Code, warnings.Body)
	}
	draftView := request(router, "GET", base+"/my-shifts"+period, "", employee, "")
	if draftView.Code != 200 || strings.Contains(draftView.Body.String(), own) {
		t.Fatalf("draft leaked to employee: %d %s", draftView.Code, draftView.Body)
	}
	last := createShift(id, week.AddDate(0, 0, 2).Add(9*time.Hour))
	race := make(chan int, 2)
	go func() { race <- request(router, "POST", publish, "{}", manager, csrf).Code }()
	go func() { race <- assign(id, last, candidate.ID) }()
	for range 2 {
		code := <-race
		if code != 200 && code != 409 {
			t.Fatalf("publication race returned %d", code)
		}
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM rosters WHERE id=$1`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state == "draft" {
		response := request(router, "POST", publish, "{}", manager, csrf)
		if response.Code != 200 {
			t.Fatalf("publish after assignment: %s", response.Body)
		}
	}
	var unassigned int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM shifts s LEFT JOIN assignments a ON a.org_id=s.org_id AND a.shift_id=s.id WHERE s.roster_id=$1 AND a.shift_id IS NULL`, id).Scan(&unassigned); err != nil || unassigned != 0 {
		t.Fatal("publication committed missing assignments")
	}
	count = auditCount()
	if request(router, "POST", publish, "{}", manager, csrf).Code != 409 || auditCount() != count {
		t.Fatal("repeat publication changed state or audit")
	}
	if request(router, "DELETE", rosters+"/"+id+"/shifts/"+own+"/assignment", "", manager, csrf).Code != 409 || auditCount() != count {
		t.Fatal("published assignment mutation committed")
	}
	ownView := request(router, "GET", base+"/my-shifts"+period+"&employee_id="+candidate.ID, "", employee, "")
	if ownView.Code != 200 || !strings.Contains(ownView.Body.String(), own) || strings.Contains(ownView.Body.String(), first) || strings.Contains(ownView.Body.String(), last) {
		t.Fatalf("employee schedule identity scoping failed: %s", ownView.Body)
	}
	managerView := request(router, "GET", base+"/my-shifts"+period, "", manager, "")
	if managerView.Code != 200 || strings.Contains(managerView.Body.String(), own) {
		t.Fatal("manager's personal view exposed someone else's assignment")
	}
	if request(router, "GET", "/api/organizations/"+foreign+"/my-shifts"+period, "", employee, "").Code != 404 {
		t.Fatal("employee crossed tenant schedule boundary")
	}
	if request(router, "PUT", base+"/employees/"+candidate.ID, employeeInput(false, []string{role}), manager, csrf).Code != 409 {
		t.Fatal("deactivated staff needed by future published shift")
	}
	if request(router, "PUT", base+"/employees/"+candidate.ID, employeeInput(true, []string{}), manager, csrf).Code != 409 {
		t.Fatal("removed qualification needed by future published shift")
	}
	if request(router, "PUT", base+"/job-roles/"+role, `{"name":"Team member","active":false}`, manager, csrf).Code != 409 {
		t.Fatal("deactivated role needed by future published shift")
	}
	if request(router, "PUT", base+"/locations/"+site, `{"name":"Main site","active":false}`, manager, csrf).Code != 409 {
		t.Fatal("deactivated location needed by future published shift")
	}
	if request(router, "POST", rosters+"/"+id+"/archive", "{}", manager, csrf).Code != 422 {
		t.Fatal("future roster archived")
	}
	past := createRoster("2020-01-06")
	pastShift := createShift(past, time.Date(2020, 1, 6, 9, 0, 0, 0, zone))
	if assign(past, pastShift, sam) != 200 {
		t.Fatal("historical assignment failed")
	}
	if request(router, "POST", rosters+"/"+past+"/publish", "{}", manager, csrf).Code != 200 {
		t.Fatal("historical roster publication failed")
	}
	if request(router, "POST", rosters+"/"+past+"/archive", "{}", manager, csrf).Code != 200 {
		t.Fatal("ended roster archival failed")
	}
	if request(router, "POST", rosters+"/"+past+"/archive", "{}", manager, csrf).Code != 409 {
		t.Fatal("repeat archive accepted")
	}
	history := request(router, "GET", base+"/my-shifts"+rangeQuery(time.Date(2020, 1, 6, 0, 0, 0, 0, zone), time.Date(2020, 1, 13, 0, 0, 0, 0, zone)), "", employee, "")
	if history.Code != 200 || !strings.Contains(history.Body.String(), pastShift) || !strings.Contains(history.Body.String(), "archived") {
		t.Fatal("archived schedule history unavailable")
	}
	activity := request(router, "GET", base+"/activity?limit=100", "", manager, "")
	if activity.Code != 200 || !strings.Contains(activity.Body.String(), "roster.published") || !strings.Contains(activity.Body.String(), "roster.archived") || !strings.Contains(activity.Body.String(), "Alex Morgan") {
		t.Fatalf("activity feed: %s", activity.Body)
	}
	if request(router, "GET", base+"/activity", "", employee, "").Code != 403 {
		t.Fatal("employee accessed manager activity feed")
	}
	for _, sql := range []string{`UPDATE audit_events SET action='tampered'`, `DELETE FROM audit_events`, `TRUNCATE audit_events`} {
		if _, err := pool.Exec(ctx, sql); err == nil {
			t.Fatalf("append-only audit accepted %s", sql)
		}
	}
	before := auditCount()
	var beforeRosters, afterRosters int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM rosters`).Scan(&beforeRosters); err != nil {
		t.Fatal(err)
	}
	_, err := roster.NewPostgresStore(pool).Create(requestctx.WithUser(ctx, ""), org, site, week.AddDate(0, 0, 14).Format(time.DateOnly))
	if err == nil {
		t.Fatal("unaudited business write accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM rosters`).Scan(&afterRosters); err != nil {
		t.Fatal(err)
	}
	if beforeRosters != afterRosters || auditCount() != before {
		t.Fatal("audit failure did not roll back its business write")
	}
}
