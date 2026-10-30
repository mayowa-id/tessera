package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/roster"
	"tessera/backend/internal/scheduling"
)

func exerciseDraftRosters(t *testing.T, ctx context.Context, pool *pgxpool.Pool, router http.Handler, manager, employee *http.Cookie, csrf, org, foreignOrg, foreignRole string) {
	t.Helper()
	base := "/api/organizations/" + org + "/rosters"
	var location, role, person string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM locations WHERE org_id=$1 AND name='Main site'`, org).Scan(&location); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM job_roles WHERE org_id=$1 AND name='Team member'`, org).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM employees WHERE org_id=$1 AND display_name='Sam Taylor'`, org).Scan(&person); err != nil {
		t.Fatal(err)
	}
	createRoster := func(week string) roster.Roster {
		t.Helper()
		response := request(router, "POST", base, fmt.Sprintf(`{"location_id":%q,"week_start":%q}`, location, week), manager, csrf)
		if response.Code != 201 {
			t.Fatalf("create roster: %d %s", response.Code, response.Body)
		}
		var result roster.Roster
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	createShift := func(rosterID, roleID, start, end string) roster.Shift {
		t.Helper()
		response := request(router, "POST", base+"/"+rosterID+"/shifts", fmt.Sprintf(`{"job_role_id":%q,"starts_at":%q,"ends_at":%q}`, roleID, start, end), manager, csrf)
		if response.Code != 201 {
			t.Fatalf("create shift: %d %s", response.Code, response.Body)
		}
		var result roster.Result
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Shift
	}
	shiftPath := func(rosterID, id string) string { return base + "/" + rosterID + "/shifts/" + id }
	assignBody := fmt.Sprintf(`{"employee_id":%q}`, person)
	week := createRoster("2026-11-02")
	duplicate := request(router, "POST", base, fmt.Sprintf(`{"location_id":%q,"week_start":"2026-11-02"}`, location), manager, csrf)
	if duplicate.Code != 409 {
		t.Fatal("duplicate weekly roster accepted")
	}
	if request(router, "GET", base+"?week_start=2026-11-02", "", employee, "").Code != 403 {
		t.Fatal("employee saw draft collection")
	}
	if request(router, "GET", base+"/"+week.ID, "", employee, "").Code != 403 {
		t.Fatal("employee saw draft detail")
	}
	if request(router, "GET", "/api/organizations/"+foreignOrg+"/rosters?week_start=2026-11-02", "", manager, "").Code != 404 {
		t.Fatal("foreign roster collection exposed")
	}
	list := request(router, "GET", base+"?week_start=2026-11-02", "", manager, "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), week.ID) {
		t.Fatal("weekly roster listing failed")
	}
	first := createShift(week.ID, role, "2026-11-02T09:00:00+01:00", "2026-11-02T17:00:00+01:00")
	second := createShift(week.ID, role, "2026-11-02T12:00:00+01:00", "2026-11-02T20:00:00+01:00")
	type assignmentResult struct {
		id   string
		code int
	}
	results := make(chan assignmentResult, 2)
	for _, shift := range []roster.Shift{first, second} {
		go func(id string) {
			response := request(router, "PUT", shiftPath(week.ID, id)+"/assignment", assignBody, manager, csrf)
			results <- assignmentResult{id, response.Code}
		}(shift.ID)
	}
	winner := ""
	success, conflict := 0, 0
	for range 2 {
		result := <-results
		if result.code == 200 {
			success++
			winner = result.id
		} else if result.code == 409 {
			conflict++
		} else {
			t.Fatalf("unexpected concurrent status %d", result.code)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("overlap race: successes=%d conflicts=%d", success, conflict)
	}
	thursday := createShift(week.ID, role, "2026-11-05T09:00:00+01:00", "2026-11-05T17:00:00+01:00")
	if response := request(router, "PUT", shiftPath(week.ID, thursday.ID)+"/assignment", assignBody, manager, csrf); response.Code != 200 {
		t.Fatalf("Thursday assignment: %s", response.Body)
	}
	edit := fmt.Sprintf(`{"job_role_id":%q,"starts_at":"2026-11-05T09:00:00+01:00","ends_at":"2026-11-05T17:00:00+01:00"}`, role)
	if request(router, "PUT", shiftPath(week.ID, winner), edit, manager, csrf).Code != 409 {
		t.Fatal("assigned shift edit bypassed overlap validation")
	}
	edit = fmt.Sprintf(`{"job_role_id":%q,"starts_at":"2026-11-03T09:00:00+01:00","ends_at":"2026-11-03T17:00:00+01:00"}`, role)
	if request(router, "PUT", shiftPath(week.ID, winner), edit, manager, csrf).Code != 200 {
		t.Fatal("valid assigned shift edit rejected")
	}
	var from, to time.Time
	if err := pool.QueryRow(ctx, `SELECT lower(during),upper(during) FROM assignments WHERE org_id=$1 AND shift_id=$2`, org, winner).Scan(&from, &to); err != nil {
		t.Fatal(err)
	}
	if !from.Equal(parseTime("2026-11-03T09:00:00+01:00")) || !to.Equal(parseTime("2026-11-03T17:00:00+01:00")) {
		t.Fatal("assignment interval did not track shift edit")
	}
	_, err := pool.Exec(ctx, `UPDATE shifts SET starts_at=$1,ends_at=$2 WHERE org_id=$3 AND id=$4`, thursday.StartsAt, thursday.EndsAt, org, winner)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("database shift-update overlap protection: %v", err)
	}
	forged := createShift(week.ID, role, "2026-11-05T10:00:00+01:00", "2026-11-05T18:00:00+01:00")
	_, err = pool.Exec(ctx, `INSERT INTO assignments(org_id,shift_id,employee_id,during) VALUES($1,$2,$3,tstzrange('2030-01-01T00:00Z','2030-01-01T01:00Z'))`, org, forged.ID, person)
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("forged interval bypassed database protection: %v", err)
	}
	adjacent := createShift(week.ID, role, "2026-11-05T17:00:00+01:00", "2026-11-05T18:00:00+01:00")
	// Database half-open ranges allow adjacency; application rest policy remains stricter.
	if _, err = pool.Exec(ctx, `INSERT INTO assignments(org_id,shift_id,employee_id) VALUES($1,$2,$3)`, org, adjacent.ID, person); err != nil {
		t.Fatalf("database rejected adjacent interval: %v", err)
	}
	if response := request(router, "DELETE", shiftPath(week.ID, adjacent.ID)+"/assignment", "", manager, csrf); response.Code != 200 || !strings.Contains(response.Body.String(), `"employee_id":null`) {
		t.Fatalf("unassign failed: %s", response.Body)
	}
	if request(router, "DELETE", shiftPath(week.ID, adjacent.ID), "", manager, csrf).Code != 204 {
		t.Fatal("draft shift deletion failed")
	}
	overnight := createShift(week.ID, role, "2026-11-08T23:00:00+01:00", "2026-11-09T02:00:00+01:00")
	if request(router, "PUT", shiftPath(week.ID, overnight.ID)+"/assignment", assignBody, manager, csrf).Code != 200 {
		t.Fatal("overnight Sunday assignment failed")
	}
	nextWeek := createRoster("2026-11-09")
	early := createShift(nextWeek.ID, role, "2026-11-09T03:00:00+01:00", "2026-11-09T05:00:00+01:00")
	denied := request(router, "PUT", shiftPath(nextWeek.ID, early.ID)+"/assignment", assignBody, manager, csrf)
	if denied.Code != 409 || !strings.Contains(denied.Body.String(), "MINIMUM_REST") {
		t.Fatalf("cross-week rest ignored: %d %s", denied.Code, denied.Body)
	}
	if request(router, "PUT", shiftPath(nextWeek.ID, first.ID), edit, manager, csrf).Code != 404 {
		t.Fatal("wrong-parent shift update permitted")
	}
	var unqualifiedRole string
	if err = pool.QueryRow(ctx, `INSERT INTO job_roles(org_id,name) VALUES($1,'Supervisor') RETURNING id::text`, org).Scan(&unqualifiedRole); err != nil {
		t.Fatal(err)
	}
	unqualified := createShift(week.ID, unqualifiedRole, "2026-11-04T09:00:00+01:00", "2026-11-04T17:00:00+01:00")
	var foreignEmployee string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM employees WHERE org_id=$1 LIMIT 1`, foreignOrg).Scan(&foreignEmployee); err != nil {
		t.Fatal(err)
	}
	foreignAssignment := request(router, "PUT", shiftPath(week.ID, unqualified.ID)+"/assignment", fmt.Sprintf(`{"employee_id":%q}`, foreignEmployee), manager, csrf)
	if foreignAssignment.Code != 404 {
		t.Fatalf("cross-tenant employee assignment accepted: %d %s", foreignAssignment.Code, foreignAssignment.Body)
	}
	denied = request(router, "PUT", shiftPath(week.ID, unqualified.ID)+"/assignment", assignBody, manager, csrf)
	if denied.Code != 409 || !strings.Contains(denied.Body.String(), "QUALIFICATION_REQUIRED") {
		t.Fatal("qualification rule ignored")
	}
	var foreignLocation string
	if err = pool.QueryRow(ctx, `INSERT INTO locations(org_id,name) VALUES($1,'Foreign roster site') RETURNING id::text`, foreignOrg).Scan(&foreignLocation); err != nil {
		t.Fatal(err)
	}
	store := roster.NewPostgresStore(pool)
	foreignRoster, err := store.Create(ctx, foreignOrg, foreignLocation, "2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	if request(router, "GET", base+"/"+foreignRoster.ID, "", manager, "").Code != 404 {
		t.Fatal("foreign roster direct ID exposed")
	}
	// Non-overlapping writes must also serialize aggregate business-rule checks.
	hardStore := roster.NewPostgresStore(pool, scheduling.Policy{WeeklyHours: 8 * time.Hour, HardWeeklyLimit: true})
	hardWeek, err := hardStore.Create(ctx, org, location, "2026-11-16")
	if err != nil {
		t.Fatal(err)
	}
	hardIDs := []string{}
	for _, day := range []string{"2026-11-16", "2026-11-17"} {
		created, err := hardStore.Change(ctx, org, hardWeek.ID, roster.Command{Action: "create", Input: roster.ShiftInput{JobRoleID: role, Start: parseTime(day + "T09:00:00+01:00"), End: parseTime(day + "T15:00:00+01:00")}})
		if err != nil {
			t.Fatal(err)
		}
		hardIDs = append(hardIDs, created.Shift.ID)
	}
	failures := make(chan error, 2)
	for _, id := range hardIDs {
		go func(id string) {
			_, err := hardStore.Change(ctx, org, hardWeek.ID, roster.Command{Action: "assign", ShiftID: id, EmployeeID: person})
			failures <- err
		}(id)
	}
	success, conflict = 0, 0
	for range 2 {
		err := <-failures
		if err == nil {
			success++
		} else {
			var ruleError *roster.Conflict
			if !errors.As(err, &ruleError) || len(ruleError.Violations) != 1 || ruleError.Violations[0].Code != "WEEKLY_HOURS" {
				t.Fatalf("aggregate rule race: %v", err)
			}
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent non-overlapping writes bypassed weekly hard limit")
	}
	warningWeek := createRoster("2026-11-30")
	for i, day := range []string{"2026-11-30", "2026-12-02", "2026-12-04"} {
		end := "20:00:00"
		if i == 2 {
			end = "04:00:00"
		}
		shift := createShift(warningWeek.ID, role, day+"T00:00:00+01:00", day+"T"+end+"+01:00")
		response := request(router, "PUT", shiftPath(warningWeek.ID, shift.ID)+"/assignment", assignBody, manager, csrf)
		if response.Code != 200 {
			t.Fatalf("warning policy blocked assignment: %s", response.Body)
		}
		if i == 2 && !strings.Contains(response.Body.String(), "WEEKLY_HOURS") {
			t.Fatal("weekly warning missing")
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE rosters SET state='published' WHERE org_id=$1 AND id=$2`, org, week.ID); err != nil {
		t.Fatal(err)
	}
	if request(router, "DELETE", shiftPath(week.ID, winner), "", manager, csrf).Code != 409 {
		t.Fatal("published roster was mutable")
	}
}
func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
