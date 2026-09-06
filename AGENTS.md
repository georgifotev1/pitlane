# Project instructions

Pitlane is a server-rendered Go application for a garage owner. Read `ADR.md`
before changing architecture.

- Keep the runtime Go-only: stdlib HTML templates and embedded static assets.
- Do not add a SPA, Node build, JSON API, jobs or object storage. Email is
  limited to the welcome and password-reset account messages; do not add offer
  email.
- Follow the Alex Edwards shape already used in `cmd/web`: application
  struct dependency injection, handler methods, explicit middleware,
  server-side forms, and post/redirect/get.
- Use `net/http` ServeMux patterns and keep dependencies minimal.
- Every tenant query MUST filter on `tenant_id` explicitly. The database does
  not enforce isolation, so a missing filter silently returns other tenants'
  rows. Go through `store.WithTenant` and use SQL placeholders only.
- Money is integer cents; tax is basis points and is recomputed server-side.
- Run `make audit` before considering work complete.
