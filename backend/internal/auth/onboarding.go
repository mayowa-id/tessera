package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Onboarding struct {
	auth *Handler
	pool *pgxpool.Pool
	mail Mailer
}

func NewOnboarding(h *Handler, pool *pgxpool.Pool, mailer Mailer) *Onboarding {
	return &Onboarding{h, pool, mailer}
}
func (o *Onboarding) Register(r chi.Router) {
	r.Post("/api/auth/signup", o.signup)
	r.Post("/api/auth/verify", o.verify)
	r.Post("/api/auth/resend", o.resend)
	r.Post("/api/auth/forgot-password", o.forgot)
	r.Post("/api/auth/reset-password", o.reset)
	r.Post("/api/auth/join", o.join)
	r.Group(func(r chi.Router) {
		r.Use(o.auth.RequireManager)
		r.Post("/api/organizations/{orgID}/invitations", o.invite)
	})
}
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func validEmail(s string) bool {
	a, e := mail.ParseAddress(s)
	return e == nil && a.Address == s && len(s) <= 254
}
func validPassword(s string) bool { return len(s) >= 12 && len(s) <= 72 }
func (o *Onboarding) input(w http.ResponseWriter, r *http.Request, v any) bool {
	w.Header().Set("Cache-Control", "no-store")
	if !o.auth.mutationAllowed(w, r) {
		return false
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		o.auth.fail(w, r, 400, "INVALID_REQUEST", "Send JSON data.")
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		o.auth.fail(w, r, 400, "INVALID_REQUEST", "Check the submitted details.")
		return false
	}
	return true
}
func (o *Onboarding) ready(w http.ResponseWriter, r *http.Request) bool {
	if o.mail == nil {
		o.auth.fail(w, r, 503, "EMAIL_UNAVAILABLE", "Email setup is not complete. Please try again later.")
		return false
	}
	return true
}

// A shared database budget bounds requests across serverless instances. Raw addresses never enter limiter keys.
func (o *Onboarding) limit(w http.ResponseWriter, r *http.Request, key string, max int) bool {
	var attempts int
	err := o.pool.QueryRow(r.Context(), `INSERT INTO onboarding_limits(key_hash,attempts,window_end) VALUES($1,1,now()+interval '15 minutes') ON CONFLICT(key_hash) DO UPDATE SET attempts=CASE WHEN onboarding_limits.window_end<now() THEN 1 ELSE onboarding_limits.attempts+1 END,window_end=CASE WHEN onboarding_limits.window_end<now() THEN now()+interval '15 minutes' ELSE onboarding_limits.window_end END RETURNING attempts`, tokenHash(key)).Scan(&attempts)
	if err != nil {
		o.auth.internal(w, r, err)
		return false
	}
	if attempts > max {
		w.Header().Set("Retry-After", "900")
		o.auth.fail(w, r, 429, "RATE_LIMITED", "Too many requests. Try again in 15 minutes.")
		return false
	}
	return true
}
func (o *Onboarding) publicLimit(w http.ResponseWriter, r *http.Request, email string) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return o.limit(w, r, "public:"+host, 30) && o.limit(w, r, "email:"+email, 5)
}
func (o *Onboarding) deliver(w http.ResponseWriter, r *http.Request, email, subject, body string) bool {
	if err := o.mail.Send(r.Context(), email, subject, body); err != nil {
		o.auth.logger.Error("email delivery failed", "error", err)
		o.auth.fail(w, r, 503, "DELIVERY_FAILED", "Email delivery failed. Your request is saved; request another email to retry.")
		return false
	}
	return true
}
func (o *Onboarding) signup(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password, Name, Team, Timezone, Location string }
	if !o.input(w, r, &in) || !o.ready(w, r) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	in.Team = strings.TrimSpace(in.Team)
	in.Location = strings.TrimSpace(in.Location)
	if in.Timezone == "" {
		in.Timezone = "Africa/Lagos"
	}
	_, tzerr := time.LoadLocation(in.Timezone)
	if !validEmail(in.Email) || !validPassword(in.Password) || len(in.Name) < 1 || len(in.Name) > 120 || len(in.Team) < 1 || len(in.Team) > 120 || len(in.Location) < 1 || len(in.Location) > 120 || tzerr != nil {
		o.auth.fail(w, r, 400, "INVALID_REQUEST", "Enter your name, email, team, location, timezone, and a password of 12–72 bytes.")
		return
	}
	if !o.publicLimit(w, r, in.Email) {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	tx, err := o.pool.Begin(r.Context())
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var uid, org, role, employee string
	err = tx.QueryRow(r.Context(), `INSERT INTO users(email,display_name,password_hash,email_verified) VALUES($1,$2,$3,false) ON CONFLICT(email) DO NOTHING RETURNING id::text`, in.Email, in.Name, string(hash)).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 202, map[string]string{"message": "If this address is available, a verification email will arrive. Otherwise sign in or request a new verification email."})
		return
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO organizations(slug,name,timezone) VALUES($1,$2,$3) RETURNING id::text`, "team-"+randomToken(), in.Team, in.Timezone).Scan(&org)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO memberships(org_id,user_id,role) VALUES($1,$2,'admin')`, org, uid)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO locations(org_id,name) VALUES($1,$2)`, org, in.Location)
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO job_roles(org_id,name) VALUES($1,'Team member') RETURNING id::text`, org).Scan(&role)
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO employees(org_id,user_id,display_name) VALUES($1,$2,$3) RETURNING id::text`, org, uid, in.Name).Scan(&employee)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO employee_roles(org_id,employee_id,job_role_id) VALUES($1,$2,$3)`, org, employee, role)
	}
	token := randomToken()
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO auth_tokens(token_hash,user_id,purpose,expires_at) VALUES($1,$2,'verify',now()+interval '24 hours')`, tokenHash(token), uid)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	if o.deliver(w, r, in.Email, "Verify your Tessera team", o.auth.origin+"/verify-email#"+token+"\n\nOpen this link and enter your password to verify your account. This link expires in 24 hours.") {
		writeJSON(w, 202, map[string]string{"message": "Check your email to verify your account, then sign in."})
	}
}
func (o *Onboarding) resend(w http.ResponseWriter, r *http.Request) { o.requestToken(w, r, "verify") }
func (o *Onboarding) forgot(w http.ResponseWriter, r *http.Request) { o.requestToken(w, r, "reset") }
func (o *Onboarding) requestToken(w http.ResponseWriter, r *http.Request, purpose string) {
	var in struct{ Email string }
	if !o.input(w, r, &in) || !o.ready(w, r) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !validEmail(in.Email) {
		o.auth.fail(w, r, 400, "INVALID_REQUEST", "Enter a valid email address.")
		return
	}
	if !o.publicLimit(w, r, in.Email) {
		return
	}
	token := randomToken()
	var uid string
	err := o.pool.QueryRow(r.Context(), `INSERT INTO auth_tokens(token_hash,user_id,purpose,expires_at) SELECT $1,id,$2,now()+CASE WHEN $2='verify' THEN interval '24 hours' ELSE interval '30 minutes' END FROM users WHERE email=$3 AND ($2='reset' OR NOT email_verified) ON CONFLICT(user_id,purpose) DO UPDATE SET token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at RETURNING user_id::text`, tokenHash(token), purpose, in.Email).Scan(&uid)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		o.auth.internal(w, r, err)
		return
	}
	if err == nil {
		path := "/verify-email"
		if purpose == "reset" {
			path = "/reset-password"
		}
		if !o.deliver(w, r, in.Email, "Your Tessera account link", o.auth.origin+path+"#"+token+"\n\nIf you did not request this, ignore this email.") {
			return
		}
	}
	writeJSON(w, 202, map[string]string{"message": "If this account needs an email, a link will arrive shortly."})
}
func (o *Onboarding) verify(w http.ResponseWriter, r *http.Request) { o.consumeToken(w, r, false) }
func (o *Onboarding) reset(w http.ResponseWriter, r *http.Request)  { o.consumeToken(w, r, true) }
func (o *Onboarding) consumeToken(w http.ResponseWriter, r *http.Request, reset bool) {
	var in struct{ Token, Password string }
	if !o.input(w, r, &in) {
		return
	}
	if len(in.Token) != 43 || !validPassword(in.Password) {
		o.auth.fail(w, r, 400, "INVALID_REQUEST", "Enter the link token and a password of 12–72 bytes.")
		return
	}
	if !o.limit(w, r, "token:"+in.Token, 10) {
		return
	}
	purpose := "verify"
	if reset {
		purpose = "reset"
	}
	tx, err := o.pool.Begin(r.Context())
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var uid, hash string
	err = tx.QueryRow(r.Context(), `SELECT u.id::text,u.password_hash FROM users u JOIN auth_tokens t ON t.user_id=u.id WHERE t.token_hash=$1 AND t.purpose=$2 AND t.expires_at>now() FOR UPDATE OF u,t`, tokenHash(in.Token), purpose).Scan(&uid, &hash)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !reset && bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		o.auth.fail(w, r, 400, "INVALID_TOKEN", "The link or password is invalid, or the link has expired.")
		return
	}
	if err == nil && reset {
		var b []byte
		b, err = bcrypt.GenerateFromPassword([]byte(in.Password), 12)
		hash = string(b)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE users SET email_verified=true,password_hash=$2 WHERE id=$1`, uid, hash)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM auth_tokens WHERE user_id=$1`, uid)
	}
	if err == nil && reset {
		_, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, uid)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Your account is ready. Sign in to continue."})
}
func (o *Onboarding) invite(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Name, Role string }
	if !o.input(w, r, &in) || !o.ready(w, r) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if in.Role == "" {
		in.Role = "employee"
	}
	if !validEmail(in.Email) || len(in.Name) < 1 || len(in.Name) > 120 || (in.Role != "employee" && in.Role != "manager") {
		o.auth.fail(w, r, 400, "INVALID_REQUEST", "Enter a name, email, and employee or manager role.")
		return
	}
	actor, _, ok := o.auth.identity(w, r)
	if !ok {
		return
	}
	org := chi.URLParam(r, "orgID")
	var role string
	err := o.pool.QueryRow(r.Context(), `SELECT role FROM memberships WHERE org_id=$1 AND user_id=$2`, org, actor.ID).Scan(&role)
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	if in.Role == "manager" && role != "admin" {
		o.auth.fail(w, r, 403, "FORBIDDEN", "Only an administrator can invite managers.")
		return
	}
	if !o.limit(w, r, "invites:"+org, 20) || !o.limit(w, r, "invite-email:"+in.Email, 5) {
		return
	}
	tx, err := o.pool.Begin(r.Context())
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize invite rotation and acceptance by email, including across different teams.
	_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, in.Email)
	var member bool
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 AND u.email=$2)`, org, in.Email).Scan(&member)
	}
	if err == nil && member {
		o.auth.fail(w, r, 409, "ALREADY_MEMBER", "This person already belongs to the team.")
		return
	}
	var employee string
	var accepted *time.Time
	if err == nil {
		e := tx.QueryRow(r.Context(), `SELECT employee_id::text,accepted_at FROM team_invitations WHERE org_id=$1 AND email=$2 FOR UPDATE`, org, in.Email).Scan(&employee, &accepted)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			err = e
		}
	}
	if err == nil && accepted != nil {
		o.auth.fail(w, r, 409, "ALREADY_MEMBER", "This invitation was already accepted.")
		return
	}
	if err == nil && employee == "" {
		err = tx.QueryRow(r.Context(), `INSERT INTO employees(org_id,display_name) VALUES($1,$2) RETURNING id::text`, org, in.Name).Scan(&employee)
	}
	token := randomToken()
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO team_invitations(org_id,employee_id,email,display_name,role,token_hash,invited_by,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()+interval '7 days') ON CONFLICT(org_id,email) DO UPDATE SET token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at,display_name=EXCLUDED.display_name,role=EXCLUDED.role,revoked_at=NULL`, org, employee, in.Email, in.Name, in.Role, tokenHash(token), actor.ID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	if o.deliver(w, r, in.Email, "You have been invited to Tessera", fmt.Sprintf("%s invited you to their team.\n\n%s/join#%s\n\nThis link expires in 7 days. If you already use Tessera, enter your existing password.", actor.DisplayName, o.auth.origin, token)) {
		writeJSON(w, 202, map[string]string{"message": "Invitation sent. You can send it again to replace the link."})
	}
}
func (o *Onboarding) join(w http.ResponseWriter, r *http.Request) {
	var in struct{ Token, Password string }
	if !o.input(w, r, &in) {
		return
	}
	if len(in.Token) != 43 || !validPassword(in.Password) {
		o.auth.fail(w, r, 400, "INVALID_REQUEST", "Use the invitation link and a password of 12–72 bytes.")
		return
	}
	if !o.limit(w, r, "join:"+in.Token, 10) {
		return
	}
	tx, err := o.pool.Begin(r.Context())
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var email string
	err = tx.QueryRow(r.Context(), `SELECT email FROM team_invitations WHERE token_hash=$1`, tokenHash(in.Token)).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		o.auth.fail(w, r, 400, "INVALID_TOKEN", "Invitation expired or already used.")
		return
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, email)
	}
	var id, org, employee, name, role string
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT id::text,org_id::text,employee_id::text,display_name,role FROM team_invitations WHERE token_hash=$1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, tokenHash(in.Token)).Scan(&id, &org, &employee, &name, &role)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		o.auth.fail(w, r, 400, "INVALID_TOKEN", "Invitation expired or already used.")
		return
	}
	var uid, hash string
	if err == nil {
		e := tx.QueryRow(r.Context(), `SELECT id::text,password_hash FROM users WHERE email=$1 FOR UPDATE`, email).Scan(&uid, &hash)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			err = e
		}
	}
	if err == nil && uid != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		o.auth.fail(w, r, 400, "INVALID_PASSWORD", "This email already has an account. Enter its existing password, or reset it first.")
		return
	}
	if err == nil && uid == "" {
		var b []byte
		b, err = bcrypt.GenerateFromPassword([]byte(in.Password), 12)
		if err == nil {
			err = tx.QueryRow(r.Context(), `INSERT INTO users(email,display_name,password_hash,email_verified) VALUES($1,$2,$3,true) RETURNING id::text`, email, name, string(b)).Scan(&uid)
		}
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE users SET email_verified=true WHERE id=$1`, uid)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO memberships(org_id,user_id,role) VALUES($1,$2,$3)`, org, uid, role)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE employees SET user_id=$3,display_name=$4 WHERE org_id=$1 AND id=$2`, org, employee, uid, name)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE team_invitations SET accepted_at=now() WHERE id=$1`, id)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		o.auth.internal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "You have joined the team. Sign in to continue."})
}
