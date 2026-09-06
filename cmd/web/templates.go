package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/ui"
)

var functions = template.FuncMap{
	"date": func(t time.Time) string { return t.Format("02.01.2006") },
	"datePtr": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format("02.01.2006")
	},
	"dateInput": func(t time.Time) string { return t.Format("2006-01-02") },
	"money":     pdf.FormatMoney,
	"percent": func(bps int) string {
		if bps%100 == 0 {
			return strconv.Itoa(bps/100) + "%"
		}
		return fmt.Sprintf("%d.%02d%%", bps/100, bps%100)
	},
	"active": func(path, prefix string) bool {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	},
	"errorFor": func(form *forms.Form, field string) string {
		if form == nil {
			return ""
		}
		return form.Errors[field]
	},
	"statusLabel": func(s string) string {
		switch s {
		case "", "draft":
			return "Чернова"
		case "sent":
			return "Изпратена"
		case "accepted":
			return "Приета"
		case "rejected":
			return "Отхвърлена"
		case "expired":
			return "Изтекла"
		case "open":
			return "Отворен"
		case "in_progress":
			return "В работа"
		case "completed":
			return "Завършен"
		default:
			return s
		}
	},
	// blankItemRow feeds the <template> element the add-row button clones, so
	// the editor's markup is written once and a new row can never drift from
	// the rows already on the page.
	"blankItemRow": func() []offerRow { return blankOfferRows(1) },
	"kindLabel": func(s string) string {
		switch s {
		case "part":
			return "Част"
		case "labor":
			return "Труд"
		default:
			return "Друго"
		}
	},
}

func newTemplateCache() (map[string]*template.Template, error) {
	pages, err := fs.Glob(ui.Files, "html/*.page.html")
	if err != nil {
		return nil, err
	}
	cache := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		name := filepath.Base(page)
		ts, err := template.New("base").Funcs(functions).ParseFS(ui.Files, "html/base.html", "html/partials.html", page)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		cache[name] = ts
	}
	return cache, nil
}

func (app *application) newTemplateData(r *http.Request) *templateData {
	data := &templateData{CurrentPath: r.URL.Path, Form: forms.New(nil)}
	if app.sessions.Exists(r.Context(), "userID") {
		data.CurrentUser = app.sessions.GetString(r.Context(), "userName")
		data.Flash = app.sessions.PopString(r.Context(), "flash")
		if tenant, err := app.tenants.GetByID(r.Context(), app.tenantID(r)); err == nil {
			data.GarageName = tenant.Name
			data.Tenant = tenant
		}
	}
	return data
}

func (app *application) render(w http.ResponseWriter, r *http.Request, page string, data *templateData) {
	ts, ok := app.templates[page]
	if !ok {
		app.serverError(w, r, fmt.Errorf("template %q not found", page))
		return
	}
	if data == nil {
		data = app.newTemplateData(r)
	}
	if data.Form == nil {
		data.Form = forms.New(nil)
	}
	if data.CurrentPath == "" {
		data.CurrentPath = r.URL.Path
	}
	if data.SiteURL == "" {
		data.SiteURL = strings.TrimRight(app.baseURL, "/")
	}
	if data.CanonicalURL == "" && data.SiteURL != "" {
		data.CanonicalURL = data.SiteURL + r.URL.Path
	}
	if data.CurrentUser == "" && app.sessions.Exists(r.Context(), "userID") {
		data.CurrentUser = app.sessions.GetString(r.Context(), "userName")
	}
	if data.GarageName == "" && data.Tenant != nil {
		data.GarageName = data.Tenant.Name
	}
	if data.Flash == "" && app.sessions.Exists(r.Context(), "userID") {
		data.Flash = app.sessions.PopString(r.Context(), "flash")
	}

	var buf bytes.Buffer
	if err := ts.ExecuteTemplate(&buf, "base", data); err != nil {
		app.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
