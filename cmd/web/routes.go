package main

import (
	"io/fs"
	"net/http"

	"github.com/gfotev/pitlane/ui"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(ui.Files, "static")
	if err != nil {
		panic(err)
	}
	staticHandler := http.FileServerFS(staticFS)
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler))
	mux.Handle("GET /favicon.ico", staticHandler)
	mux.Handle("GET /site.webmanifest", staticHandler)
	mux.Handle("GET /robots.txt", staticHandler)

	mux.HandleFunc("GET /healthz", app.health)
	mux.HandleFunc("GET /", app.home)
	mux.HandleFunc("GET /account/signup", app.signupView)
	mux.HandleFunc("POST /account/signup", app.signup)
	mux.HandleFunc("GET /account/login", app.loginView)
	mux.HandleFunc("POST /account/login", app.login)
	mux.HandleFunc("GET /account/forgot-password", app.forgotPasswordView)
	mux.HandleFunc("POST /account/forgot-password", app.forgotPassword)
	mux.HandleFunc("GET /account/reset-password", app.resetPasswordView)
	mux.HandleFunc("POST /account/reset-password", app.resetPassword)
	mux.HandleFunc("POST /account/logout", app.requireAuthentication(app.logout))
	mux.HandleFunc("GET /account/profile", app.requireAuthentication(app.profileView))
	mux.HandleFunc("POST /account/profile/name", app.requireAuthentication(app.profileNameUpdate))
	mux.HandleFunc("POST /account/profile/password", app.requireAuthentication(app.profilePasswordUpdate))

	mux.HandleFunc("GET /dashboard", app.requireAuthentication(app.dashboard))
	mux.HandleFunc("GET /garage", app.requireAuthentication(app.garageView))
	mux.HandleFunc("POST /garage", app.requireAuthentication(app.garageUpdate))

	mux.HandleFunc("GET /customers", app.requireAuthentication(app.customersList))
	mux.HandleFunc("GET /customers/new", app.requireAuthentication(app.customerCreateView))
	mux.HandleFunc("POST /customers/new", app.requireAuthentication(app.customerCreate))
	mux.HandleFunc("GET /customers/{id}", app.requireAuthentication(app.customerView))
	mux.HandleFunc("GET /customers/{id}/edit", app.requireAuthentication(app.customerEditView))
	mux.HandleFunc("POST /customers/{id}/edit", app.requireAuthentication(app.customerEdit))
	mux.HandleFunc("POST /customers/{id}/archive", app.requireAuthentication(app.customerArchive))

	mux.HandleFunc("GET /cars", app.requireAuthentication(app.carsList))
	mux.HandleFunc("GET /customers/{id}/cars/new", app.requireAuthentication(app.carCreateView))
	mux.HandleFunc("POST /customers/{id}/cars/new", app.requireAuthentication(app.carCreate))
	mux.HandleFunc("GET /cars/{id}", app.requireAuthentication(app.carView))
	mux.HandleFunc("GET /cars/{id}/edit", app.requireAuthentication(app.carEditView))
	mux.HandleFunc("POST /cars/{id}/edit", app.requireAuthentication(app.carEdit))
	mux.HandleFunc("POST /cars/{id}/archive", app.requireAuthentication(app.carArchive))

	mux.HandleFunc("GET /cars/{id}/history/new", app.requireAuthentication(app.historyCreateView))
	mux.HandleFunc("POST /cars/{id}/history/new", app.requireAuthentication(app.historyCreate))
	mux.HandleFunc("GET /history/{id}/edit", app.requireAuthentication(app.historyEditView))
	mux.HandleFunc("POST /history/{id}/edit", app.requireAuthentication(app.historyEdit))
	mux.HandleFunc("POST /history/{id}/delete", app.requireAuthentication(app.historyDelete))

	mux.HandleFunc("GET /offers", app.requireAuthentication(app.offersList))
	mux.HandleFunc("GET /cars/{id}/offers/new", app.requireAuthentication(app.offerCreateView))
	mux.HandleFunc("POST /cars/{id}/offers/new", app.requireAuthentication(app.offerCreate))
	mux.HandleFunc("GET /offers/{id}", app.requireAuthentication(app.offerView))
	mux.HandleFunc("GET /offers/{id}/edit", app.requireAuthentication(app.offerEditView))
	mux.HandleFunc("POST /offers/{id}/edit", app.requireAuthentication(app.offerEdit))
	mux.HandleFunc("GET /offers/{id}/print", app.requireAuthentication(app.offerPrint))
	mux.HandleFunc("GET /offers/{id}/pdf", app.requireAuthentication(app.offerPDF))
	mux.HandleFunc("POST /offers/{id}/accept", app.requireAuthentication(app.offerAccept))

	mux.HandleFunc("GET /repairs", app.requireAuthentication(app.repairsList))
	mux.HandleFunc("GET /repairs/{id}", app.requireAuthentication(app.repairView))
	mux.HandleFunc("GET /repairs/{id}/edit", app.requireAuthentication(app.repairEditView))
	mux.HandleFunc("POST /repairs/{id}/edit", app.requireAuthentication(app.repairEdit))
	mux.HandleFunc("POST /repairs/{id}/mileage", app.requireAuthentication(app.repairMileageUpdate))
	mux.HandleFunc("POST /repairs/{id}/start", app.requireAuthentication(app.repairStart))
	mux.HandleFunc("POST /repairs/{id}/reopen", app.requireAuthentication(app.repairReopen))
	mux.HandleFunc("POST /repairs/{id}/complete", app.requireAuthentication(app.repairComplete))

	csrf := http.NewCrossOriginProtection()
	return app.recoverPanic(app.logRequest(secureHeaders(csrf.Handler(app.sessions.LoadAndSave(mux)))))
}
