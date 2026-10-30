package audit

import (
	"context"
	"encoding/json"
	"errors"
	"tessera/backend/internal/db"
	"tessera/backend/internal/requestctx"
)

// Record must receive the transaction's query instance so the event commits atomically.
func Record(ctx context.Context, q *db.Queries, org, kind, id, label, action string, before, after any) error {
	actor := requestctx.UserID(ctx)
	if actor == "" {
		return errors.New("an authenticated actor is required for audited writes")
	}
	previous, err := json.Marshal(before)
	if err != nil {
		return err
	}
	next, err := json.Marshal(after)
	if err != nil {
		return err
	}
	return q.RecordAudit(ctx, db.RecordAuditParams{OrgID: org, ActorUserID: actor, EntityType: kind, EntityID: id, EntityLabel: label, Action: action, BeforeData: previous, AfterData: next})
}
