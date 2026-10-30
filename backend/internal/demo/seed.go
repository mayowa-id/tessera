// Package demo provides repeatable fixtures for explicitly enabled development databases.
package demo

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func Seed(ctx context.Context, pool *pgxpool.Pool, password string) error {
	if len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("DEMO_PASSWORD must contain 12 to 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('tessera-demo-seed'))`); err != nil {
		return err
	}
	var orgID, locationID, roleID string
	err = tx.QueryRow(ctx, `INSERT INTO organizations (slug,name,timezone) VALUES ('tessera-demo','Tessera Demo','Africa/Lagos') ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`).Scan(&orgID)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `INSERT INTO locations (org_id,name) VALUES ($1,'Main site') ON CONFLICT (org_id,name) DO UPDATE SET active=true RETURNING id::text`, orgID).Scan(&locationID)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `INSERT INTO job_roles (org_id,name) VALUES ($1,'Team member') ON CONFLICT (org_id,name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, orgID).Scan(&roleID)
	if err != nil {
		return err
	}
	for _, person := range []struct{ email, name, role string }{
		{"manager@tessera.local", "Alex Morgan", "manager"},
		{"employee@tessera.local", "Sam Taylor", "employee"},
	} {
		var userID, employeeID string
		err = tx.QueryRow(ctx, `INSERT INTO users (email,display_name,password_hash,email_verified) VALUES ($1,$2,$3,true) ON CONFLICT (email) DO UPDATE SET display_name=EXCLUDED.display_name,password_hash=EXCLUDED.password_hash,email_verified=true RETURNING id::text`, person.email, person.name, string(hash)).Scan(&userID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO memberships (org_id,user_id,role) VALUES ($1,$2,$3) ON CONFLICT (org_id,user_id) DO UPDATE SET role=EXCLUDED.role`, orgID, userID, person.role); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `INSERT INTO employees (org_id,user_id,display_name) VALUES ($1,$2,$3) ON CONFLICT (org_id,user_id) DO UPDATE SET display_name=EXCLUDED.display_name,active=true RETURNING id::text`, orgID, userID, person.name).Scan(&employeeID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO employee_roles (org_id,employee_id,job_role_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, orgID, employeeID, roleID); err != nil {
			return err
		}
		// Re-seeding changes demo passwords, so revoke existing sessions and reset throttles.
		if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID); err != nil {
			return err
		}
		emailHash := sha256.Sum256([]byte(person.email))
		if _, err = tx.Exec(ctx, `DELETE FROM login_limits WHERE key_hash=$1`, emailHash[:]); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
