package auth

import (
	"crypto/hmac"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RequireManager applies the same tenant, role, and CSRF policy to every staff route.
func (h *Handler) RequireManager(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, token, ok := h.identity(w, r)
		if !ok {
			return
		}
		memberships, err := h.store.Memberships(r.Context(), user.ID)
		if err != nil {
			h.internal(w, r, err)
			return
		}
		var role string
		for _, membership := range memberships {
			if membership.OrgID == chi.URLParam(r, "orgID") {
				role = membership.Role
				break
			}
		}
		if role == "" {
			h.fail(w, r, 404, "NOT_FOUND", "Organization not found.")
			return
		}
		if role != "manager" && role != "admin" {
			h.fail(w, r, 403, "FORBIDDEN", "Manager access is required.")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if !h.mutationAllowed(w, r) {
				return
			}
			if !hmac.Equal([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrfToken(token))) {
				h.fail(w, r, 403, "CSRF_REJECTED", "Refresh the page and try again.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
