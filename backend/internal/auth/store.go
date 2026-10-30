package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/db"
)

var ErrNotFound = errors.New("identity not found")

type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	PasswordHash string `json:"-"`
	Verified     bool   `json:"-"`
}

type Membership struct {
	OrgID    string `json:"org_id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Role     string `json:"role"`
}

type Summary struct {
	Employees        int64 `json:"employees"`
	Locations        int64 `json:"locations"`
	JobRoles         int64 `json:"job_roles"`
	Rosters          int64 `json:"rosters"`
	PublishedRosters int64 `json:"published_rosters"`
}

type Store interface {
	FindUser(context.Context, string) (User, error)
	SessionUser(context.Context, []byte) (User, error)
	Memberships(context.Context, string) ([]Membership, error)
	CreateSession(context.Context, []byte, string, time.Time) error
	DeleteSession(context.Context, []byte) error
	LoginAttempt(context.Context, []byte) (int32, error)
	Summary(context.Context, string) (Summary, error)
}

type PostgresStore struct{ queries *db.Queries }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{queries: db.New(pool)}
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *PostgresStore) FindUser(ctx context.Context, email string) (User, error) {
	row, err := s.queries.FindLoginUser(ctx, email)
	return User{ID: row.ID, Email: row.Email, DisplayName: row.DisplayName, PasswordHash: row.PasswordHash, Verified: row.EmailVerified}, notFound(err)
}
func (s *PostgresStore) SessionUser(ctx context.Context, hash []byte) (User, error) {
	row, err := s.queries.SessionUser(ctx, hash)
	return User{ID: row.ID, Email: row.Email, DisplayName: row.DisplayName}, notFound(err)
}
func (s *PostgresStore) Memberships(ctx context.Context, userID string) ([]Membership, error) {
	rows, err := s.queries.UserMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Membership, 0, len(rows))
	for _, row := range rows {
		result = append(result, Membership{OrgID: row.OrgID, Name: row.Name, Timezone: row.Timezone, Role: row.Role})
	}
	return result, nil
}
func (s *PostgresStore) CreateSession(ctx context.Context, hash []byte, userID string, expires time.Time) error {
	return s.queries.CreateSession(ctx, db.CreateSessionParams{TokenHash: hash, UserID: userID, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}})
}
func (s *PostgresStore) DeleteSession(ctx context.Context, hash []byte) error {
	return s.queries.DeleteSession(ctx, hash)
}
func (s *PostgresStore) LoginAttempt(ctx context.Context, hash []byte) (int32, error) {
	return s.queries.ConsumeLoginAttempt(ctx, hash)
}
func (s *PostgresStore) Summary(ctx context.Context, orgID string) (Summary, error) {
	row, err := s.queries.OrganizationSummary(ctx, orgID)
	return Summary{Employees: row.Employees, Locations: row.Locations, JobRoles: row.JobRoles, Rosters: row.Rosters, PublishedRosters: row.PublishedRosters}, err
}
