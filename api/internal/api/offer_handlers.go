package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/validator"
	"github.com/google/uuid"
)

// Offer input bounds. Money is integer cents; the caps keep obviously bogus
// payloads out and guard the int64 → number JSON round-trip stays well within
// range. maxTaxRateBps is 100% (10000 bps).
const (
	maxOfferItems      = 200
	maxOfferDescLen    = 500
	maxOfferNotesLen   = 5000
	maxOfferQuantity   = 1_000_000
	maxUnitPriceCents  = 100_000_000 // 1,000,000.00 in cents
	maxOfferTaxRateBps = 10_000      // 100%
)

// listOffers returns a page of one car's offers (newest first) with pagination
// metadata. The car must exist in this tenant (404 otherwise) so a bad carId is
// not silently rendered as an empty list.
func (s *Server) listOffers(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	carID := r.PathValue("carId")

	if !s.carExists(w, r, tenantID, carID) {
		return
	}

	page := clampAtLeast(queryInt(r, "page", 1), 1)
	pageSize := clampRange(queryInt(r, "pageSize", defaultPageSize), 1, maxPageSize)

	offers, total, err := s.offers.List(r.Context(), tenantID, carID, store.OfferListParams{
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list offers", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.OfferResponse, 0, len(offers))
	for _, o := range offers {
		items = append(items, offerResponse(o))
	}
	renderJSON(w, http.StatusOK, envelope{
		"offers":   items,
		"metadata": dto.ListMetadata{Page: page, PageSize: pageSize, Total: total},
	})
}

// listAllOffers returns a tenant-wide page of offers (the board), newest
// first, optionally filtered by status. Each row is enriched with car plate +
// customer name. An unknown status filter is a 422 rather than a silent empty
// list (same discipline as the repairs board).
func (s *Server) listAllOffers(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())

	status := r.URL.Query().Get("status")
	if status != "" && !domain.IsValidOfferStatus(status) {
		renderValidation(w, r, map[string]string{"status": validator.CodeInvalid})
		return
	}

	page := clampAtLeast(queryInt(r, "page", 1), 1)
	pageSize := clampRange(queryInt(r, "pageSize", defaultPageSize), 1, maxPageSize)

	summaries, total, err := s.offers.ListAll(r.Context(), tenantID, store.OfferBoardParams{
		Status: status,
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list offers board", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.OfferSummaryResponse, 0, len(summaries))
	for _, sm := range summaries {
		items = append(items, offerSummaryResponse(sm))
	}
	renderJSON(w, http.StatusOK, envelope{
		"offers":   items,
		"metadata": dto.ListMetadata{Page: page, PageSize: pageSize, Total: total},
	})
}

// getOffer returns one offer with its line items, or 404.
func (s *Server) getOffer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	id := r.PathValue("id")

	o, err := s.offers.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "offer not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("get offer", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	renderJSON(w, http.StatusOK, envelope{"offer": offerResponse(o)})
}

// createOffer validates and inserts a draft offer under the car named in the
// path. The tax rate is snapshotted from the tenant default so a later change
// to that default never re-prices this quote. Totals are computed server-side.
func (s *Server) createOffer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	carID := r.PathValue("carId")

	if !s.carExists(w, r, tenantID, carID) {
		return
	}

	var req dto.CreateOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	normalizeOfferItems(req.Items)

	if errs := validateOffer(req.Notes, 0, req.Items, false); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	// Snapshot the tax rate from the tenant default.
	tenant, err := s.tenants.GetByID(r.Context(), tenantID)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("load tenant for offer tax rate", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	o := &domain.Offer{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		CarID:      carID,
		TaxRateBps: int(tenant.DefaultTaxRate),
		Notes:      req.Notes,
		Items:      offerItemsFromRequest(req.Items),
	}
	if err := s.offers.Create(r.Context(), o); err != nil {
		loggerFromContext(r.Context(), s.logger).Error("create offer", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "offer.create", "offer", o.ID, map[string]any{
		"carId":      o.CarID,
		"totalCents": o.TotalCents,
	})

	renderJSON(w, http.StatusCreated, envelope{"offer": offerResponse(o)})
}

// updateOffer replaces a draft offer's editable fields and line items (PUT).
// A sent (or later) offer is immutable → 409; a missing offer → 404.
func (s *Server) updateOffer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.UpdateOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	normalizeOfferItems(req.Items)

	if errs := validateOffer(req.Notes, req.TaxRateBps, req.Items, true); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	o := &domain.Offer{
		ID:         id,
		TenantID:   tenantID,
		TaxRateBps: req.TaxRateBps,
		Notes:      req.Notes,
		Items:      offerItemsFromRequest(req.Items),
	}
	if err := s.offers.Update(r.Context(), o); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "offer not found")
		case errors.Is(err, store.ErrOfferNotDraft):
			renderProblem(w, r, http.StatusConflict, CodeConflict, "offer is not a draft")
		default:
			loggerFromContext(r.Context(), s.logger).Error("update offer", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	// Re-read so the response carries the full current row (timestamps, items).
	updated, err := s.offers.Get(r.Context(), tenantID, id)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("reload offer after update", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "offer.update", "offer", id, map[string]any{
		"totalCents": updated.TotalCents,
	})

	renderJSON(w, http.StatusOK, envelope{"offer": offerResponse(updated)})
}

// updateOfferStatus advances an offer through its lifecycle. An unknown status
// string is a 422; a disallowed transition (from the current state) is a 409.
func (s *Server) updateOfferStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.UpdateOfferStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	if !domain.IsValidOfferStatus(req.Status) {
		renderValidation(w, r, map[string]string{"status": validator.CodeInvalid})
		return
	}

	o, err := s.offers.SetStatus(r.Context(), tenantID, id, domain.OfferStatus(req.Status))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "offer not found")
		case errors.Is(err, store.ErrInvalidStatusTransition):
			renderProblem(w, r, http.StatusConflict, CodeConflict, "invalid status transition")
		default:
			loggerFromContext(r.Context(), s.logger).Error("set offer status", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "offer.status", "offer", id, map[string]any{
		"status": string(o.Status),
	})

	renderJSON(w, http.StatusOK, envelope{"offer": offerResponse(o)})
}

// offerEmailEnqueuer binds a per-send Reply-To and hands back a
// store.OfferEmailEnqueuer that MarkSending drives inside its transaction. The
// concrete implementation (internal/jobs) wraps the River client; defining the
// seam here keeps the api package independent of jobs and River (ADR §106).
type offerEmailEnqueuer interface {
	WithReplyTo(replyTo string) store.OfferEmailEnqueuer
}

// sendOffer emails an offer's PDF to the customer and, on the initial send,
// freezes it as sent. It flips the offer to sent + send_status=pending + sent_to
// and enqueues the delivery job in ONE transaction (ADR §17): the offer is
// never marked sent without a dispatched job, nor a job without the state.
//
// This is the only path from draft → sent (the generic status endpoint cannot
// send), so every sent offer has a recipient and a real dispatch. The recipient
// is prefilled from the customer on the client but editable, so it is validated
// here. Reply-To is the sending user's email (ADR §14: From is the app domain,
// replies reach the garage). A non-sendable offer is 409; an unknown one is 404.
func (s *Server) sendOffer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")
	log := loggerFromContext(r.Context(), s.logger)

	var req dto.SendOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	req.Recipient = strings.TrimSpace(req.Recipient)

	v := validator.New()
	v.Email("recipient", req.Recipient)
	if !v.Valid() {
		renderValidation(w, r, v.Errors())
		return
	}

	// Reply-To is the staff member who sent it (tenants carry no contact email
	// yet). Best-effort: a lookup miss just drops the header, never blocks send.
	replyTo := ""
	if user, err := s.users.GetByID(r.Context(), tenantID, userID); err == nil {
		replyTo = user.Email
	} else {
		log.Warn("send offer: load sender for reply-to", "err", err)
	}

	o, err := s.offers.MarkSending(r.Context(), tenantID, id, req.Recipient, s.sendEnqueuer.WithReplyTo(replyTo))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "offer not found")
		case errors.Is(err, store.ErrOfferNotSendable):
			renderProblem(w, r, http.StatusConflict, CodeConflict, "offer is not in a sendable state")
		default:
			log.Error("send offer", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "offer.send", "offer", id, map[string]any{
		"recipient": req.Recipient,
	})

	renderJSON(w, http.StatusOK, envelope{"offer": offerResponse(o)})
}

// acceptOffer accepts a sent offer BY converting it into a repair: in one
// transaction it copies the offer's (frozen) line items into a new open repair,
// links provenance, and flips the offer to `accepted`. This is the sole accept
// path — the generic status endpoint deliberately cannot reach `accepted` (see
// domain.offerTransitions) — so an accepted offer always has exactly one
// repair. Returns 201 with the created repair. A non-sent offer is 409; an
// unknown one is 404.
func (s *Server) acceptOffer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	repair, err := s.repairs.CreateFromOffer(r.Context(), tenantID, id)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "offer not found")
		case errors.Is(err, store.ErrOfferNotAcceptable):
			renderProblem(w, r, http.StatusConflict, CodeConflict, "offer is not in an acceptable state")
		default:
			loggerFromContext(r.Context(), s.logger).Error("accept offer", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	// One user action, two provenance-linked records: audit both.
	_ = s.audit.Insert(r.Context(), tenantID, userID, "offer.accept", "offer", id, map[string]any{
		"repairId": repair.ID,
	})
	_ = s.audit.Insert(r.Context(), tenantID, userID, "repair.create", "repair", repair.ID, map[string]any{
		"offerId":    id,
		"totalCents": repair.TotalCents,
	})

	renderJSON(w, http.StatusCreated, envelope{"repair": repairResponse(repair)})
}

// carExists guards the nested offer routes: it confirms the path's car belongs
// to this tenant, rendering a 404 (and returning false) if not. Mirrors
// customerExists for the cars slice.
func (s *Server) carExists(w http.ResponseWriter, r *http.Request, tenantID, carID string) bool {
	_, err := s.cars.Get(r.Context(), tenantID, carID)
	if err == nil {
		return true
	}
	if errors.Is(err, store.ErrNotFound) {
		renderProblem(w, r, http.StatusNotFound, CodeNotFound, "car not found")
		return false
	}
	loggerFromContext(r.Context(), s.logger).Error("check car for offer", "err", err)
	renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
	return false
}

func offerResponse(o *domain.Offer) dto.OfferResponse {
	items := make([]dto.OfferItemResponse, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, dto.OfferItemResponse{
			ID:             it.ID,
			Kind:           string(it.Kind),
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			LineTotalCents: it.LineTotalCents,
			SortOrder:      it.SortOrder,
		})
	}
	return dto.OfferResponse{
		ID:            o.ID,
		CarID:         o.CarID,
		Status:        string(o.Status),
		SendStatus:    string(o.SendStatus),
		SentTo:        o.SentTo,
		SentAt:        o.SentAt,
		TaxRateBps:    o.TaxRateBps,
		SubtotalCents: o.SubtotalCents,
		TaxCents:      o.TaxCents,
		TotalCents:    o.TotalCents,
		Notes:         o.Notes,
		Items:         items,
		CreatedAt:     o.CreatedAt,
		UpdatedAt:     o.UpdatedAt,
	}
}

func offerSummaryResponse(sm store.OfferSummary) dto.OfferSummaryResponse {
	o := sm.Offer
	return dto.OfferSummaryResponse{
		ID:           o.ID,
		CarID:        o.CarID,
		CarPlate:     sm.CarPlate,
		CustomerName: sm.CustomerName,
		Status:       string(o.Status),
		SendStatus:   string(o.SendStatus),
		TotalCents:   o.TotalCents,
		Notes:        o.Notes,
		CreatedAt:    o.CreatedAt,
	}
}

// offerItemsFromRequest maps request lines to domain items. IDs, tenant/offer
// linkage, sort order, and line totals are all assigned by the store, not here.
func offerItemsFromRequest(items []dto.OfferItemRequest) []domain.OfferItem {
	out := make([]domain.OfferItem, 0, len(items))
	for _, it := range items {
		out = append(out, domain.OfferItem{
			Kind:           domain.OfferItemKind(it.Kind),
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	return out
}

// normalizeOfferItems trims each line's text fields in place.
func normalizeOfferItems(items []dto.OfferItemRequest) {
	for i := range items {
		items[i].Description = strings.TrimSpace(items[i].Description)
		items[i].Kind = strings.TrimSpace(items[i].Kind)
	}
}

// validateOffer enforces the shared create/update rules, emitting react-hook-
// form-compatible field keys (items.<i>.<field>) so the client can attach each
// error to its line. taxRateBps is only validated on update (checkTax), since
// create snapshots it from the tenant.
func validateOffer(notes string, taxRateBps int, items []dto.OfferItemRequest, checkTax bool) map[string]string {
	v := validator.New()
	v.MaxLength("notes", notes, maxOfferNotesLen)
	if checkTax {
		v.Range("taxRateBps", taxRateBps, 0, maxOfferTaxRateBps)
	}

	// An offer must carry at least one line, and not an unbounded number.
	if len(items) == 0 {
		v.NotEmpty("items", "")
	}
	if len(items) > maxOfferItems {
		v.Range("items", len(items), 0, maxOfferItems)
	}

	for i, it := range items {
		prefix := fmt.Sprintf("items.%d.", i)
		v.OneOf(prefix+"kind", it.Kind,
			string(domain.OfferItemKindPart),
			string(domain.OfferItemKindLabor),
			string(domain.OfferItemKindOther),
		)
		v.NotEmpty(prefix+"description", it.Description)
		v.MaxLength(prefix+"description", it.Description, maxOfferDescLen)
		v.Range(prefix+"quantity", it.Quantity, 1, maxOfferQuantity)
		// Cents is int64; on the 64-bit target platform the cast is lossless for
		// values within the cap, and anything above it is flagged invalid anyway.
		v.Range(prefix+"unitPriceCents", int(it.UnitPriceCents), 0, maxUnitPriceCents)
	}
	return v.Errors()
}
