package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"
)

const cookieName = "tessera_session"
const sessionLifetime = 12 * time.Hour

type Handler struct {
	store     Store
	origin    string
	secure    bool
	logger    *slog.Logger
	dummyHash []byte
}

func NewHandler(store Store, origin string, secure bool, logger *slog.Logger) (*Handler, error) {
	// Equal-cost password work for unknown accounts reduces account enumeration.
	dummy, err := bcrypt.GenerateFromPassword([]byte("unused-dummy-password"), 12)
	if err != nil {
		return nil, err
	}
	return &Handler{store: store, origin: origin, secure: secure, logger: logger, dummyHash: dummy}, nil
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, req)
			})
		})
		r.Post("/api/auth/login", h.login)
		r.Get("/api/auth/me", h.me)
		r.Post("/api/auth/logout", h.logout)
		r.Get("/api/organizations/{orgID}/summary", h.summary)
	})
}

func tokenHash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func csrfToken(token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte("tessera-csrf-v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Handler) mutationAllowed(w http.ResponseWriter, r *http.Request) bool {
	// Fixed configuration, never an untrusted forwarded Host, defines the browser origin.
	if r.Header.Get("Origin") != h.origin || r.Header.Get("X-Tessera-Request") != "1" {
		h.fail(w, r, 403, "CSRF_REJECTED", "The request origin could not be verified.")
		return false
	}
	return true
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.mutationAllowed(w, r) {
		return
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send a JSON email and password.")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send a JSON email and password.")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		h.fail(w, r, 400, "INVALID_REQUEST", "Send one JSON object.")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 || len(input.Password) < 1 || len(input.Password) > 72 {
		h.fail(w, r, 422, "INVALID_CREDENTIAL_FIELDS", "Enter a valid email and a password of at most 72 bytes.")
		return
	}
	attempts, err := h.store.LoginAttempt(r.Context(), tokenHash(input.Email))
	if err != nil {
		h.internal(w, r, err)
		return
	}
	if attempts > 10 {
		w.Header().Set("Retry-After", "900")
		h.fail(w, r, 429, "LOGIN_LIMITED", "Too many attempts. Try again in 15 minutes.")
		return
	}
	user, err := h.store.FindUser(r.Context(), input.Email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		h.internal(w, r, err)
		return
	}
	hash := h.dummyHash
	if err == nil {
		hash = []byte(user.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(input.Password))
	if err != nil || passwordErr != nil {
		h.fail(w, r, 401, "INVALID_CREDENTIALS", "Email or password is incorrect.")
		return
	}
	memberships, err := h.store.Memberships(r.Context(), user.ID)
	if err != nil {
		h.internal(w, r, err)
		return
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		h.internal(w, r, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	expires := time.Now().Add(sessionLifetime)
	if err := h.store.CreateSession(r.Context(), tokenHash(token), user.ID, expires); err != nil {
		h.internal(w, r, err)
		return
	}
	// Reauthentication rotates and invalidates the browser's previous session.
	if previous, err := r.Cookie(cookieName); err == nil {
		if err := h.store.DeleteSession(r.Context(), tokenHash(previous.Value)); err != nil {
			_ = h.store.DeleteSession(r.Context(), tokenHash(token))
			h.internal(w, r, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/api", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionLifetime.Seconds()), Expires: expires})
	writeJSON(w, 200, map[string]any{"user": user, "memberships": memberships, "csrf_token": csrfToken(token)})
}

func (h *Handler) identity(w http.ResponseWriter, r *http.Request) (User, string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || len(cookie.Value) != 43 {
		h.fail(w, r, 401, "UNAUTHENTICATED", "Sign in to continue.")
		return User{}, "", false
	}
	user, err := h.store.SessionUser(r.Context(), tokenHash(cookie.Value))
	if errors.Is(err, ErrNotFound) {
		h.fail(w, r, 401, "UNAUTHENTICATED", "Your session has expired. Sign in again.")
		return User{}, "", false
	}
	if err != nil {
		h.internal(w, r, err)
		return User{}, "", false
	}
	return user, cookie.Value, true
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, token, ok := h.identity(w, r)
	if !ok {
		return
	}
	memberships, err := h.store.Memberships(r.Context(), user.ID)
	if err != nil {
		h.internal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": user, "memberships": memberships, "csrf_token": csrfToken(token)})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !h.mutationAllowed(w, r) {
		return
	}
	_, token, ok := h.identity(w, r)
	if !ok {
		return
	}
	if !hmac.Equal([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrfToken(token))) {
		h.fail(w, r, 403, "CSRF_REJECTED", "Refresh the page and try again.")
		return
	}
	if err := h.store.DeleteSession(r.Context(), tokenHash(token)); err != nil {
		h.internal(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/api", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.identity(w, r)
	if !ok {
		return
	}
	memberships, err := h.store.Memberships(r.Context(), user.ID)
	if err != nil {
		h.internal(w, r, err)
		return
	}
	orgID := chi.URLParam(r, "orgID")
	var membership *Membership
	for i := range memberships {
		if memberships[i].OrgID == orgID {
			membership = &memberships[i]
			break
		}
	}
	if membership == nil {
		h.fail(w, r, 404, "NOT_FOUND", "Organization not found.")
		return
	}
	if membership.Role != "admin" && membership.Role != "manager" {
		h.fail(w, r, 403, "FORBIDDEN", "Manager access is required.")
		return
	}
	summary, err := h.store.Summary(r.Context(), orgID)
	if err != nil {
		h.internal(w, r, err)
		return
	}
	writeJSON(w, 200, summary)
}

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("identity request failed", "request_id", middleware.GetReqID(r.Context()), "error", err)
	h.fail(w, r, 503, "SERVICE_UNAVAILABLE", "The service is unavailable. Please try again.")
}
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message, "request_id": middleware.GetReqID(r.Context())})
}
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
