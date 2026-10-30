package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/requestctx"
	"tessera/backend/internal/roster"
	"tessera/backend/internal/scheduling"
	"tessera/backend/internal/timeoff"
)

func exerciseTimeOff(t *testing.T, ctx context.Context, pool *pgxpool.Pool, router http.Handler, admin, employee *http.Cookie, org, foreign string) {
	t.Helper()
	base := "/api/organizations/" + org
	adminCSRF, employeeCSRF := csrfToken(admin.Value), csrfToken(employee.Value)
	var sam, site, role, adminID string
	for _, v := range []struct {
		query  string
		target *string
	}{{`SELECT id::text FROM employees WHERE org_id=$1 AND display_name='Sam Taylor'`, &sam}, {`SELECT id::text FROM locations WHERE org_id=$1 AND name='Main site'`, &site}, {`SELECT id::text FROM job_roles WHERE org_id=$1 AND name='Team member'`, &role}, {`SELECT user_id::text FROM employees WHERE org_id=$1 AND display_name='Alex Morgan'`, &adminID}} {
		if err := pool.QueryRow(ctx, v.query, org).Scan(v.target); err != nil {
			t.Fatal(err)
		}
	}
	zone, _ := time.LoadLocation("Africa/Lagos")
	week := scheduling.WeekStart(time.Now(), zone).AddDate(0, 0, 280)
	start := week.Add(9 * time.Hour)
	createTimeOff := func(kind string, at time.Time) timeoff.Record {
		t.Helper()
		w := request(router, "POST", base+"/my-time-off", fmt.Sprintf(`{"kind":%q,"starts_at":%q,"ends_at":%q,"reason":"private medical note"}`, kind, at.Format(time.RFC3339), at.Add(8*time.Hour).Format(time.RFC3339)), employee, employeeCSRF)
		if w.Code != 201 {
			t.Fatalf("time off create %d %s", w.Code, w.Body)
		}
		var r timeoff.Record
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if r.EmployeeID != sam {
			t.Fatal("personal create used another employee")
		}
		return r
	}
	decision := func(id, action string, cookie *http.Cookie, csrf string) int {
		return request(router, "POST", base+"/time-off/"+id+"/"+action, `{"note":"Reviewed"}`, cookie, csrf).Code
	}
	r := createTimeOff("leave", start)
	if r.Status != "pending" {
		t.Fatal("leave was auto-approved")
	}
	if decision(r.ID, "approve", employee, employeeCSRF) != 403 {
		t.Fatal("employee approved leave")
	}
	if decision(r.ID, "approve", admin, "bad-token") != 403 {
		t.Fatal("CSRF bypass")
	}
	if decision(r.ID, "approve", admin, adminCSRF) != 200 || decision(r.ID, "approve", admin, adminCSRF) != 409 {
		t.Fatal("approval state transition")
	}
	if request(router, "POST", "/api/organizations/"+foreign+"/time-off/"+r.ID+"/cancel", `{}`, admin, adminCSRF).Code != 404 {
		t.Fatal("cross-tenant decision")
	}
	if request(router, "POST", base+"/my-time-off", fmt.Sprintf(`{"kind":"leave","employee_id":%q,"starts_at":%q,"ends_at":%q}`, sam, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)), employee, employeeCSRF).Code != 400 {
		t.Fatal("employee ID injection")
	}
	query := "?from=" + week.UTC().Format(time.RFC3339) + "&to=" + week.AddDate(0, 0, 90).UTC().Format(time.RFC3339)
	if request(router, "GET", base+"/time-off"+query, "", employee, "").Code != 403 {
		t.Fatal("employee read team leave")
	}
	adminOwn := request(router, "GET", base+"/my-time-off"+query, "", admin, "")
	if adminOwn.Code != 200 || strings.Contains(adminOwn.Body.String(), r.ID) {
		t.Fatal("personal list leaked coworker details")
	}
	if request(router, "GET", base+"/time-off"+query, "", nil, "").Code != 401 {
		t.Fatal("anonymous leave read")
	}
	var foreignEmployee string
	if err := pool.QueryRow(ctx, `INSERT INTO employees(org_id,display_name) VALUES($1,'Foreign employee') RETURNING id::text`, foreign).Scan(&foreignEmployee); err != nil {
		t.Fatal(err)
	}
	wForeign := request(router, "POST", base+"/employees/"+foreignEmployee+"/time-off", fmt.Sprintf(`{"kind":"leave","starts_at":%q,"ends_at":%q}`, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)), admin, adminCSRF)
	if wForeign.Code != 404 {
		t.Fatal("cross-tenant staff relationship accepted")
	}
	var countBefore, countAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM time_off WHERE org_id=$1`, org).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	_, auditErr := timeoff.NewPostgresStore(pool).Create(requestctx.WithUser(ctx, ""), org, adminID, sam, timeoff.Input{Kind: "leave", Start: start, End: start.Add(time.Hour)})
	if auditErr == nil {
		t.Fatal("time off write without audit actor accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM time_off WHERE org_id=$1`, org).Scan(&countAfter); err != nil || countAfter != countBefore {
		t.Fatal("audit failure did not roll back time off")
	}
	// Set up an empty draft at a distant week to isolate this feature's rules.
	w := request(router, "POST", base+"/rosters", fmt.Sprintf(`{"location_id":%q,"week_start":%q}`, site, week.Format(time.DateOnly)), admin, adminCSRF)
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	var draft roster.Roster
	_ = json.Unmarshal(w.Body.Bytes(), &draft)
	addShift := func(at time.Time) string {
		t.Helper()
		w := request(router, "POST", base+"/rosters/"+draft.ID+"/shifts", fmt.Sprintf(`{"job_role_id":%q,"starts_at":%q,"ends_at":%q}`, role, at.Format(time.RFC3339), at.Add(8*time.Hour).Format(time.RFC3339)), admin, adminCSRF)
		if w.Code != 201 {
			t.Fatal(w.Body)
		}
		var result roster.Result
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return result.Shift.ID
	}
	assign := func(id string) int {
		return request(router, "PUT", base+"/rosters/"+draft.ID+"/shifts/"+id+"/assignment", fmt.Sprintf(`{"employee_id":%q}`, sam), admin, adminCSRF).Code
	}
	first := addShift(start)
	if assign(first) != 409 {
		t.Fatal("assignment ignored approved leave")
	}
	if decision(r.ID, "cancel", employee, employeeCSRF) != 200 || assign(first) != 200 {
		t.Fatal("cancellation did not restore availability")
	}
	pending := createTimeOff("leave", start)
	if decision(pending.ID, "approve", admin, adminCSRF) != 409 {
		t.Fatal("approved leave over existing assignment")
	}
	if decision(pending.ID, "reject", admin, adminCSRF) != 200 || decision(pending.ID, "cancel", employee, employeeCSRF) != 409 {
		t.Fatal("rejection lifecycle")
	}
	// Conflicting unavailability is rejected, not silently allowed to invalidate a shift.
	w = request(router, "POST", base+"/my-time-off", fmt.Sprintf(`{"kind":"unavailable","starts_at":%q,"ends_at":%q}`, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)), employee, employeeCSRF)
	if w.Code != 409 || strings.Contains(w.Body.String(), first) {
		t.Fatal("personal availability must reject the conflict without exposing draft shift details")
	}
	w = request(router, "POST", base+"/employees/"+sam+"/time-off", fmt.Sprintf(`{"kind":"unavailable","starts_at":%q,"ends_at":%q}`, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)), admin, adminCSRF)
	if w.Code != 409 || !strings.Contains(w.Body.String(), first) {
		t.Fatal("manager was not shown the conflicting assignment")
	}
	unavailable := createTimeOff("unavailable", start.AddDate(0, 0, 1))
	second := addShift(start.AddDate(0, 0, 1))
	if assign(second) != 409 {
		t.Fatal("unavailability ignored")
	}
	if decision(unavailable.ID, "cancel", admin, adminCSRF) != 200 {
		t.Fatal("manager could not clear unavailability")
	}
	// Competing approval and assignment must not both commit.
	raceAt := start.AddDate(0, 0, 2)
	raceLeave := createTimeOff("leave", raceAt)
	third := addShift(raceAt)
	var codes [2]int
	gate := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	go func() { defer group.Done(); <-gate; codes[0] = decision(raceLeave.ID, "approve", admin, adminCSRF) }()
	go func() { defer group.Done(); <-gate; codes[1] = assign(third) }()
	close(gate)
	group.Wait()
	if !((codes[0] == 200 && codes[1] == 409) || (codes[0] == 409 && codes[1] == 200)) {
		t.Fatalf("approval/assignment race: %v", codes)
	}
	// Simulate an out-of-band block to prove publication always revalidates.
	_, err := pool.Exec(ctx, `INSERT INTO time_off(org_id,employee_id,kind,status,starts_at,ends_at,created_by)VALUES($1,$2,'leave','approved',$3,$4,$5)`, org, sam, start, start.Add(time.Hour), adminID)
	if err != nil {
		t.Fatal(err)
	}
	w = request(router, "POST", base+"/rosters/"+draft.ID+"/publish", `{}`, admin, adminCSRF)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "APPROVED_LEAVE") {
		t.Fatalf("publish did not recheck leave: %d %s", w.Code, w.Body)
	}
	var leaked int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE org_id=$1 AND (before_data::text LIKE '%private medical note%' OR after_data::text LIKE '%private medical note%')`, org).Scan(&leaked)
	if err != nil || leaked != 0 {
		t.Fatal("reason leaked into activity log")
	}
}
