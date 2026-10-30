package roster

import (
	"context"
	"tessera/backend/internal/db"
	"tessera/backend/internal/scheduling"
)

func timeOffViolations(ctx context.Context, q *db.Queries, org string, proposed scheduling.Proposed) ([]scheduling.Violation, error) {
	rows, err := q.SchedulingTimeOff(ctx, db.SchedulingTimeOffParams{OrgID: org, EmployeeID: proposed.EmployeeID, FromTime: timestamp(proposed.Start), ToTime: timestamp(proposed.End)})
	if err != nil {
		return nil, err
	}
	blocks := []scheduling.Block{}
	for _, row := range rows {
		blocks = append(blocks, scheduling.Block{Interval: scheduling.Interval{Start: row.StartsAt.Time, End: row.EndsAt.Time}, Kind: row.Kind})
	}
	return scheduling.CheckTimeOff(proposed, blocks), nil
}
