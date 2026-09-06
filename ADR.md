# Architecture Decision Record — Server-rendered Pitlane

**Status:** Accepted
**Supersedes:** JSON API + React SPA architecture

## Goal

Pitlane is intentionally a small owner-operated garage application. An owner
creates a garage and manages customers, cars, service-history notes, repair
offers and the repairs created when offers are accepted. Completed repairs feed
the service history and revenue total. Offers have a print view and a
server-generated PDF. Signup sends a welcome email and owners can reset a lost
password by email. Team management, attachments, offer email and background
jobs are out of scope.

## Decisions

1. **Go and server-rendered HTML.** `net/http` method/pattern routing and
   `html/template`; no frontend framework and no JSON application API.
2. **Alex Edwards structure.** A small `application` struct holds dependencies;
   handlers are methods; middleware is explicit; form values/errors are posted,
   validated and rendered server-side; POST success uses redirect-after-post.
3. **Embedded UI.** `ui` embeds templates and CSS with `go:embed`. The
   production image contains one static Go executable.
4. **PostgreSQL.** Keep the existing tenant/customer/car/history/offer schema,
   raw pgx stores and RLS so deployed data remains compatible.
5. **Authentication.** Owner-only bcrypt credentials and opaque server-side SCS
   sessions in PostgreSQL. Cookies are HttpOnly, SameSite=Lax and Secure in
   production. Go's `CrossOriginProtection` protects unsafe requests. Welcome
   and password-reset messages are rendered from embedded templates and sent
   directly over SMTP; reset tokens are hashed, single-use and valid for one
   hour. These low-volume account emails do not introduce a job queue.
6. **Offers and repairs.** VAT-inclusive prices use integer cents; the fixed
   20% VAT and included tax are computed server-side. Draft offers can be
   edited and acceptance atomically copies their lines into an open repair. Repairs can be edited before work starts,
   moved through open/in-progress/completed, and completion records mileage.
   Only completed repair totals count as revenue. UUIDs remain internal identifiers;
   offers and repairs receive separate tenant-scoped annual document numbers (for
   example `OF-2026-000001` and `RP-2026-000001`). Maroto generates downloadable
   offer PDFs on demand. A dedicated print stylesheet supports browser printing
   and “Save as PDF”. Offers are never emailed.
7. **Margin.** Each line also records the VAT-inclusive purchase price paid to
   the supplier, and each document snapshots the gross and net totals of those
   costs beside its own. Profit is net revenue less net cost, because VAT is
   collected for the state on one side and deductible on the other. Purchase
   prices and margin are internal: they appear on the boards, the document
   pages and the dashboard, and never on the print sheet or in the PDF.
8. **Demonstration data.** `pitlane demo seed|reset|drop` builds a throwaway
   garage with a year of history so the product can be shown on a real
   deployment. It writes through the ordinary stores and only rewrites
   timestamps afterwards, so the result is data the application could have
   produced. Tenants carry an `is_demo` flag: it is what allows the reset path
   to delete rows, and pointed at a real garage that path refuses.
9. **Deployment.** The Docker build has only a Go build stage and distroless
   runtime. Migrations are embedded and run explicitly with
   `pitlane migrate up`. PostgreSQL and an SMTP provider are the only external
   services.

## Layout

```
cmd/web/            # main, routes, handlers, middleware and views
internal/forms/     # Edwards-style form validation
internal/domain/    # business types and offer calculations
internal/store/     # tenant-scoped PostgreSQL access
internal/mailer/    # welcome/reset templates and SMTP delivery
internal/pdf/       # offer PDF renderer
migrations/         # embedded goose SQL
ui/html/            # embedded templates
ui/static/          # embedded CSS
Dockerfile
Makefile
```
