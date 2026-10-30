# Production onboarding

The production frontend is `https://tessera-ui.vercel.app`; its `/api/*` rewrite targets `https://tessera-backend.vercel.app/api/*`. Browser requests stay on the frontend origin, including session cookies. `mayowa.site` is the email sending domain only.

## Backend environment

Set these in the **backend** Vercel project, then redeploy:

| Variable | Value |
| --- | --- |
| `TS_POSTGRES_URL` | Existing Neon integration pooled URL, retaining TLS |
| `APP_ORIGIN` | `https://tessera-ui.vercel.app` (no trailing slash) |
| `SESSION_COOKIE_SECURE` | `true` |
| `RESEND_API_KEY` | Your private Resend key |
| `MAIL_FROM` | `Tessera <onboarding@mayowa.site>` |

`DATABASE_URL` takes precedence when set; otherwise the backend reads `TS_POSTGRES_URL`, then `TS_DATABASE_URL`. Remove a stale `DATABASE_URL` if it points elsewhere. Do not use `TS_POSTGRES_URL_NO_SSL`. No database URL or email key belongs in frontend variables or source control. Neon Auth variables are unused: Tessera manages its own sessions and memberships.

In Resend, verify the DNS records for `mayowa.site` before using that sender. The integration sends through Resend's HTTPS API; SMTP configuration is unnecessary. See [Resend domain verification](https://resend.com/docs/dashboard/domains/introduction) and [sending email](https://resend.com/docs/api-reference/emails/send-email).

## Release sequence

1. Back up the database and run Goose migrations through **version 5**, using the direct Neon connection with TLS. Use the commands below once before deploying the updated backend. Existing accounts retain access; new registrations require verification.
2. Deploy the backend with the environment above. Missing email credentials disable registration/invitation delivery; existing login remains available.
3. Deploy the frontend with its committed API rewrite. Vercel deployment protection must allow those API requests.
4. Visit `https://tessera-ui.vercel.app/api/readyz`; it must return connected PostgreSQL. Check the backend logs if it fails. Readiness checks connectivity, so also confirm migration version 5 with Goose before testing registration.
5. Open `/` in a private browser window. The sample roster must load without signing in. Create a team using an inbox you control, open the verification email, enter your chosen password, and sign in.
6. Open **Invite teammates** from the workspace. Invite another address you control. Accept that link, set a password for a new account (or use the existing account password), then sign in. Assign job qualifications in **Team & locations** before assigning shifts.
7. Test password recovery and sign out. Password reset invalidates previous sessions.

From the repository root, in PowerShell (replace the placeholder locally, never commit the URL):

```powershell
Set-Location backend
$env:DATABASE_URL='<Neon direct connection URL with TLS>'
go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir db/migrations postgres $env:DATABASE_URL status
go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir db/migrations postgres $env:DATABASE_URL up
go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir db/migrations postgres $env:DATABASE_URL version
Remove-Item Env:DATABASE_URL
```

Use the direct URL only in this migration terminal; keep the pooled URL configured for the deployed backend. Migrations contain changes only, not production demo seed data.

## Account behavior

Signup atomically creates an unverified owner, a separate organization, an admin membership, a first location, and a default Team member qualification. An organization name or matching email domain never grants access to an existing team. Email verification links expire after 24 hours; reset links after 30 minutes; invitations after 7 days. Tokens are stored only as hashes and consumed once. Link fragments keep secrets out of server URL logs. Clicking a link alone does not verify or accept anything: a form submission is required.

Invitations create pending employee records. Reinviting the same address reuses that record and replaces the old link. Admins can invite managers and employees; managers can invite employees. Existing accounts must provide their current password. Delivery failure leaves a recoverable pending record: use the verification resend page or send the invitation again. Requests are limited in PostgreSQL across application instances. The invitation screen accepts one person or up to 10 name/email lines, sends sequentially, and preserves unsent entries after a failure. CSV upload, invitation status/revocation controls, and automated bounce handling are subsequent improvements.

This checkout previously contained migrations 1–4. Its new migration 5 is onboarding. Reconcile migration numbering before merging another branch that already has its own migration 5. Publication and audit features from other branches are not included here; this release supports draft roster management. Employee accounts can join and sign in, but their personal published schedule remains a later phase.

Local tests use a capture mailer and never contact Resend. Actual production delivery and Vercel cookie/proxy behavior must be checked after deploying with the private credentials.
