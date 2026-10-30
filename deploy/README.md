# Separate Vercel deployments

Connect the same Git repository to two Vercel projects. Each application's root contains its own deployment configuration; neither build needs files from the other application.

| Setting | Frontend project | Backend project |
| --- | --- | --- |
| Root Directory | `frontend` | `backend` |
| Framework | Angular | Go |
| Install | Configured in `frontend/vercel.json` | Go preset handles modules |
| Build | `npm run build` | Go preset detects `cmd/api/main.go` |
| Output | `dist/tessera-web/browser` | Go preset manages output |
| Environment | No database credentials | Production `DATABASE_URL`, frontend HTTPS `APP_ORIGIN` |

The backend honors Vercel's `PORT` automatically. Leave `HTTP_ADDR` unset on Vercel. Use a managed PostgreSQL database with TLS and an appropriate pooled connection endpoint and connection budget. Run Goose migrations once as a release step, not on every server start or request.

Set `APP_ORIGIN` to the exact frontend origin, with no trailing slash, and keep `SESSION_COOKIE_SECURE=true` (the default). The backend refuses insecure production origins. Configure preview deployments with their exact intended frontend origin and a non-production database. The development seed is intentionally disabled on Vercel; demo users need a separate, explicitly controlled provisioning step.

## Connect the frontend to the backend

Deploy the backend first and check `https://YOUR-BACKEND.vercel.app/api/healthz` and `/api/readyz`. Readiness requires the configured PostgreSQL database.

The committed `frontend/vercel.json` targets `https://tessera-backend.vercel.app`. For another deployment, change the destination:

```json
"rewrites": [
  { "source": "/api/:path*", "destination": "https://YOUR-BACKEND.vercel.app/api/:path*" },
  { "source": "/(.*)", "destination": "/index.html" }
]
```

Preserve the existing framework, install, build, and output settings. Do not deploy the placeholder hostname. Keep the API rewrite before the SPA fallback. Angular's local proxy is not used in production.

This proxy keeps browser requests on the frontend origin. Verify session cookies, origin forwarding, and CSRF through that production origin before release; the complete flow is tested locally through Angular's proxy. If backend deployment protection is enabled, configure permitted server-to-server access; a protected backend will otherwise block the frontend proxy. Keep preview deployments paired with a non-production database and the intended backend.

Future background work must use bounded scheduled invocations or a separate worker service rather than assuming an API process runs continuously.

The user has deployed `tessera-ui.vercel.app` and `tessera-backend.vercel.app`. See [production onboarding](../docs/production-onboarding.md) for the Neon prefix, email configuration, migration 7, and release checks. These local changes have not been deployed from this workspace.

References: [Go preset and entry points](https://vercel.com/docs/functions/runtimes/go), [Angular on Vercel](https://vercel.com/docs/frameworks/frontend).
