package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/gfotev/pitlane/internal/demo"
	"github.com/gfotev/pitlane/internal/store"
)

const demoReadOnlyMessage = "Това е демо версия - може да разглеждате всичко, но промените не се запазват."

// demoLogin signs a visitor into the shared demonstration garage without a
// password. Every visitor shares the same garage, so requireAuthentication
// refuses any change while the tenant is flagged is_demo; one person's clicks
// can never alter what the next person sees.
func (app *application) demoLogin(w http.ResponseWriter, r *http.Request) {
	user, err := app.users.GetByEmail(r.Context(), demo.DefaultEmail)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		app.serverError(w, r, err)
		return
	}
	isDemo := false
	if err == nil {
		if isDemo, err = app.isDemoTenant(r.Context(), user.TenantID); err != nil {
			app.serverError(w, r, err)
			return
		}
	}
	if !isDemo {
		// Either the demo has not been seeded, or the address belongs to a real
		// garage; in both cases nobody gets signed in.
		app.sessions.Put(r.Context(), "flash", "Демото в момента не е налично. Опитайте отново след малко.")
		http.Redirect(w, r, "/account/login", http.StatusSeeOther)
		return
	}
	if err := app.startSession(r, user); err != nil {
		app.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (app *application) isDemoTenant(ctx context.Context, tenantID string) (bool, error) {
	tenant, err := app.tenants.GetByID(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return tenant.IsDemo, nil
}

// refuseDemoWrite turns a write against the demonstration garage into a
// message on the page the visitor came from. It reports whether it did.
func (app *application) refuseDemoWrite(w http.ResponseWriter, r *http.Request, tenantID string) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.URL.Path == "/account/logout" {
		return false
	}
	isDemo, err := app.isDemoTenant(r.Context(), tenantID)
	if err != nil {
		app.serverError(w, r, err)
		return true
	}
	if !isDemo {
		return false
	}
	app.flash(r, demoReadOnlyMessage)
	http.Redirect(w, r, sameOriginReferer(r, "/dashboard"), http.StatusSeeOther)
	return true
}

// sameOriginReferer returns the path of the page that submitted the request,
// or fallback when the referer is missing or points elsewhere.
func sameOriginReferer(r *http.Request, fallback string) string {
	ref, err := url.Parse(r.Referer())
	if err != nil || ref.Host != r.Host || !safeRedirect(ref.RequestURI()) {
		return fallback
	}
	return ref.RequestURI()
}
