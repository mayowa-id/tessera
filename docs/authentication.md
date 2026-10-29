# Authentication decisions

## Request flow

1. Angular restores the session with `GET /api/auth/me`. A 401 is a normal signed-out state; a service failure is shown separately.
2. Login submits JSON to `/api/auth/login`, including `X-Tessera-Request: 1`. The browser supplies Origin. The backend verifies the configured origin, applies a database-backed login limit, and checks the password.
3. The backend generates 32 random bytes, sends their base64url representation as an HttpOnly cookie, and stores only a SHA-256 hash with a 12-hour expiry. Reauthentication invalidates the browser's old token.
4. Login and session restoration return user details, authorized memberships, and a CSRF token derived with HMAC-SHA256 from the secret session token. JavaScript cannot read the session cookie. Identity and the CSRF token are kept in memory.
5. Logout checks the origin, custom request header, session, and CSRF token, deletes the database session, and expires the cookie. Auth responses use `Cache-Control: no-store`.

The API allows no cross-origin credential requests. The intended ingress is a same-origin frontend rewrite to the independently deployed backend. The configured frontend origin is authoritative; arbitrary forwarded host headers are never trusted for CSRF validation. Missing or `null` origins on mutation requests are rejected.

## Policies

- Password hashing: bcrypt cost 12; 72-byte maximum. Unknown emails perform a dummy hash comparison and return the same credential error.
- Throttling: ten login attempts per normalized email per fixed 15-minute window, including successful attempts. Atomic PostgreSQL upserts serialize increments across Vercel instances. This is an account-level limit; add ingress/IP abuse protection before exposing a public demo to arbitrary traffic.
- Cookies: host-only, HttpOnly, `/api`, SameSite=Lax, Secure by default. Insecure cookies are permitted only for localhost origins. Production configuration requires HTTPS.
- Authorization: each organization request loads current memberships. The manager summary permits admin/manager and rejects employee access. An unknown/nonmember organization is indistinguishable from a nonexistent one.
- Revocation: logout, reauthentication of the current browser, and reseeding of development users. Expired database sessions are never accepted. There is no password reset or public registration yet.
- Cleanup: expired session and login-limit rows are inert but retained. Before production, add a scheduled bounded cleanup of expired rows; throttling keys reset their window on the next attempt.

The database integration suite uses a unique temporary schema, validates repeatable seed data, checks session expiry and rate windows, and attempts cross-organization relationship writes. CI runs it with `TEST_DATABASE_URL`; ordinary unit tests use a store fake and do not claim to validate PostgreSQL behavior.

References: [OWASP CSRF prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html), [Go bcrypt implementation](https://github.com/golang/crypto/blob/master/bcrypt/bcrypt.go).
