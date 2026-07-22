package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
)

// offerRenderer turns a loaded offer into PDF bytes. The interface lives here,
// with its consumer (ADR §106), so the api package does not depend on maroto;
// the concrete pdf.Renderer is wired in cmd/api. Defining it here also lets the
// handler test swap in a fake for the non-happy paths without generating a real
// document.
type offerRenderer interface {
	RenderOffer(pdf.OfferData) ([]byte, error)
}

// offerPDF streams an offer as a PDF. Nothing is stored — the document is
// regenerated on demand from the current row (ADR §13), so a sent offer's PDF
// is byte-stable because its content is frozen, not because it was cached.
//
// ?disposition=inline serves it for in-browser preview (the SPA iframe);
// anything else defaults to an attachment download. The endpoint overrides the
// middleware's X-Frame-Options: DENY with SAMEORIGIN so the same-origin iframe
// preview is allowed to frame it.
func (s *Server) offerPDF(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	id := r.PathValue("id")
	log := loggerFromContext(r.Context(), s.logger)

	offer, err := s.offers.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "offer not found")
			return
		}
		log.Error("pdf: get offer", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	// The offer's car, the car's customer, and the tenant are all needed for the
	// document header/parties. They are all tenant-scoped, so a dangling FK would
	// surface as ErrNotFound → 500 here (a data-integrity bug, not a client 404).
	car, err := s.cars.Get(r.Context(), tenantID, offer.CarID)
	if err != nil {
		log.Error("pdf: get car", "err", err, "carId", offer.CarID)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	customer, err := s.customers.Get(r.Context(), tenantID, car.CustomerID)
	if err != nil {
		log.Error("pdf: get customer", "err", err, "customerId", car.CustomerID)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	tenant, err := s.tenants.GetByID(r.Context(), tenantID)
	if err != nil {
		log.Error("pdf: get tenant", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	doc, err := s.pdf.RenderOffer(pdf.OfferData{
		Tenant:   tenant,
		Customer: customer,
		Car:      car,
		Offer:    offer,
	})
	if err != nil {
		log.Error("pdf: render offer", "err", err, "offerId", id)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	disposition := "attachment"
	if r.URL.Query().Get("disposition") == "inline" {
		disposition = "inline"
	}
	filename := "oferta-" + shortOfferID(offer.ID) + ".pdf"

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(doc)))
	// Override secureHeaders' DENY so the SPA's same-origin preview iframe renders.
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(doc); err != nil {
		// Response is already committed; nothing to render, just record it.
		log.Warn("pdf: write body", "err", err, "offerId", id)
	}
}

// shortOfferID returns the first UUID segment for a compact download filename.
func shortOfferID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}
