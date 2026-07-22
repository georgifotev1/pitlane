package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/validator"
)

// maxMileage bounds the odometer reading on completion, matching the car
// mileage cap in validateCar.
const maxMileage = 10_000_000

// listRepairs returns a tenant-wide page of repairs (the board), newest first,
// optionally filtered by status. Each row is enriched with car plate + customer
// name. An unknown status filter is a 422 rather than a silent empty list.
func (s *Server) listRepairs(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())

	status := r.URL.Query().Get("status")
	if status != "" && !domain.IsValidRepairStatus(status) {
		renderValidation(w, r, map[string]string{"status": validator.CodeInvalid})
		return
	}

	page := clampAtLeast(queryInt(r, "page", 1), 1)
	pageSize := clampRange(queryInt(r, "pageSize", defaultPageSize), 1, maxPageSize)

	summaries, total, err := s.repairs.List(r.Context(), tenantID, store.RepairListParams{
		Status: status,
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list repairs", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.RepairSummaryResponse, 0, len(summaries))
	for _, sm := range summaries {
		items = append(items, repairSummaryResponse(sm))
	}
	renderJSON(w, http.StatusOK, envelope{
		"repairs":  items,
		"metadata": dto.ListMetadata{Page: page, PageSize: pageSize, Total: total},
	})
}

// getRepair returns one repair with its line items, or 404.
func (s *Server) getRepair(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	id := r.PathValue("id")

	repair, err := s.repairs.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "repair not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("get repair", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	renderJSON(w, http.StatusOK, envelope{"repair": repairResponse(repair)})
}

// updateRepair replaces an open repair's editable fields and line items (PUT).
// A started (or completed) repair is immutable → 409; a missing one → 404. The
// validation rules are shared with offers (validateOffer): same item shape,
// same bounds, same react-hook-form field keys.
func (s *Server) updateRepair(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.UpdateRepairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	normalizeOfferItems(req.Items)

	if errs := validateOffer(req.Notes, req.TaxRateBps, req.Items, true); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	repair := &domain.Repair{
		ID:         id,
		TenantID:   tenantID,
		TaxRateBps: req.TaxRateBps,
		Notes:      req.Notes,
		Items:      repairItemsFromRequest(req.Items),
	}
	if err := s.repairs.Update(r.Context(), repair); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "repair not found")
		case errors.Is(err, store.ErrRepairNotOpen):
			renderProblem(w, r, http.StatusConflict, CodeConflict, "repair is not open")
		default:
			loggerFromContext(r.Context(), s.logger).Error("update repair", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	// Re-read so the response carries the full current row (timestamps, items).
	updated, err := s.repairs.Get(r.Context(), tenantID, id)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("reload repair after update", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "repair.update", "repair", id, map[string]any{
		"totalCents": updated.TotalCents,
	})

	renderJSON(w, http.StatusOK, envelope{"repair": repairResponse(updated)})
}

// updateRepairStatus advances a repair through the generic lifecycle
// (open ↔ in_progress). Completion is a separate endpoint. An unknown status
// string is a 422; a disallowed transition is a 409.
func (s *Server) updateRepairStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.UpdateRepairStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	if !domain.IsValidRepairStatus(req.Status) {
		renderValidation(w, r, map[string]string{"status": validator.CodeInvalid})
		return
	}

	repair, err := s.repairs.SetStatus(r.Context(), tenantID, id, domain.RepairStatus(req.Status))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "repair not found")
		case errors.Is(err, store.ErrInvalidRepairStatusTransition):
			renderProblem(w, r, http.StatusConflict, CodeConflict, "invalid status transition")
		default:
			loggerFromContext(r.Context(), s.logger).Error("set repair status", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "repair.status", "repair", id, map[string]any{
		"status": string(repair.Status),
	})

	renderJSON(w, http.StatusOK, envelope{"repair": repairResponse(repair)})
}

// completeRepair finishes a repair, recording the odometer reading that is also
// written onto the car (odometer only moves forward). An already-completed
// repair is 409; a missing one is 404.
func (s *Server) completeRepair(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.CompleteRepairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}

	v := validator.New()
	v.Range("mileage", req.Mileage, 0, maxMileage)
	if !v.Valid() {
		renderValidation(w, r, v.Errors())
		return
	}

	repair, err := s.repairs.Complete(r.Context(), tenantID, id, req.Mileage)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "repair not found")
		case errors.Is(err, store.ErrRepairNotOpen):
			renderProblem(w, r, http.StatusConflict, CodeConflict, "repair is already completed")
		default:
			loggerFromContext(r.Context(), s.logger).Error("complete repair", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "repair.complete", "repair", id, map[string]any{
		"mileage": req.Mileage,
	})

	renderJSON(w, http.StatusOK, envelope{"repair": repairResponse(repair)})
}

func repairResponse(r *domain.Repair) dto.RepairResponse {
	items := make([]dto.RepairItemResponse, 0, len(r.Items))
	for _, it := range r.Items {
		items = append(items, dto.RepairItemResponse{
			ID:             it.ID,
			Kind:           string(it.Kind),
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			LineTotalCents: it.LineTotalCents,
			SortOrder:      it.SortOrder,
		})
	}
	return dto.RepairResponse{
		ID:            r.ID,
		CarID:         r.CarID,
		OfferID:       r.OfferID,
		Status:        string(r.Status),
		TaxRateBps:    r.TaxRateBps,
		SubtotalCents: r.SubtotalCents,
		TaxCents:      r.TaxCents,
		TotalCents:    r.TotalCents,
		Mileage:       r.Mileage,
		Notes:         r.Notes,
		Items:         items,
		CompletedAt:   r.CompletedAt,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func repairSummaryResponse(sm store.RepairSummary) dto.RepairSummaryResponse {
	r := sm.Repair
	return dto.RepairSummaryResponse{
		ID:           r.ID,
		CarID:        r.CarID,
		CarPlate:     sm.CarPlate,
		CustomerName: sm.CustomerName,
		OfferID:      r.OfferID,
		Status:       string(r.Status),
		TotalCents:   r.TotalCents,
		Mileage:      r.Mileage,
		CompletedAt:  r.CompletedAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

// repairItemsFromRequest maps request lines to domain repair items. IDs,
// tenant/repair linkage, sort order, and line totals are all assigned by the
// store, not here — mirroring offerItemsFromRequest.
func repairItemsFromRequest(items []dto.OfferItemRequest) []domain.RepairItem {
	out := make([]domain.RepairItem, 0, len(items))
	for _, it := range items {
		out = append(out, domain.RepairItem{
			Kind:           domain.OfferItemKind(it.Kind),
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	return out
}
