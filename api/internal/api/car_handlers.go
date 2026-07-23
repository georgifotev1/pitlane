package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/validator"
	"github.com/google/uuid"
)

// listCars returns a page of one customer's cars with pagination metadata. The
// customer must exist in this tenant (404 otherwise) so a bad customerId is not
// silently rendered as an empty list. Search params: page, pageSize, search,
// archived. Pagination/search live in query params (not the URL route) because
// the UI nests cars inside the customer detail page rather than a route of its
// own.
func (s *Server) listCars(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	customerID := r.PathValue("customerId")

	if !s.customerExists(w, r, tenantID, customerID) {
		return
	}

	page := clampAtLeast(queryInt(r, "page", 1), 1)
	pageSize := clampRange(queryInt(r, "pageSize", defaultPageSize), 1, maxPageSize)
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	includeArchived := r.URL.Query().Get("archived") == "true"

	cars, total, err := s.cars.List(r.Context(), tenantID, customerID, store.CarListParams{
		Search:          search,
		IncludeArchived: includeArchived,
		Limit:           pageSize,
		Offset:          (page - 1) * pageSize,
	})
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list cars", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.CarResponse, 0, len(cars))
	for _, c := range cars {
		items = append(items, carResponse(c))
	}
	renderJSON(w, http.StatusOK, envelope{
		"cars":     items,
		"metadata": dto.ListMetadata{Page: page, PageSize: pageSize, Total: total},
	})
}

// listAllCars returns a tenant-wide page of cars (the board), plate order,
// with search and archived filters mirroring the nested list. Each row is
// enriched with the owning customer's name so the board renders without extra
// round-trips.
func (s *Server) listAllCars(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())

	page := clampAtLeast(queryInt(r, "page", 1), 1)
	pageSize := clampRange(queryInt(r, "pageSize", defaultPageSize), 1, maxPageSize)
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	includeArchived := r.URL.Query().Get("archived") == "true"

	summaries, total, err := s.cars.ListAll(r.Context(), tenantID, store.CarListParams{
		Search:          search,
		IncludeArchived: includeArchived,
		Limit:           pageSize,
		Offset:          (page - 1) * pageSize,
	})
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list cars board", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.CarSummaryResponse, 0, len(summaries))
	for _, sm := range summaries {
		items = append(items, carSummaryResponse(sm))
	}
	renderJSON(w, http.StatusOK, envelope{
		"cars":     items,
		"metadata": dto.ListMetadata{Page: page, PageSize: pageSize, Total: total},
	})
}

// getCar returns one car or 404.
func (s *Server) getCar(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	id := r.PathValue("id")

	c, err := s.cars.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "car not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("get car", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	renderJSON(w, http.StatusOK, envelope{"car": carResponse(c)})
}

// createCar validates and inserts a car under the customer named in the path,
// then writes the audit log. A duplicate plate is a 422 on the plate field.
func (s *Server) createCar(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	customerID := r.PathValue("customerId")

	if !s.customerExists(w, r, tenantID, customerID) {
		return
	}

	var req dto.CreateCarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	normalizeCar(&req.Plate, &req.VIN, &req.Make, &req.Model)

	if errs := validateCar(req.Plate, req.VIN, req.Make, req.Model, req.Year, req.Mileage); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	c := &domain.Car{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		CustomerID: customerID,
		Plate:      req.Plate,
		VIN:        req.VIN,
		Make:       req.Make,
		Model:      req.Model,
		Year:       req.Year,
		Mileage:    req.Mileage,
	}
	if err := s.cars.Create(r.Context(), c); err != nil {
		if errors.Is(err, store.ErrDuplicatePlate) {
			renderValidation(w, r, map[string]string{"plate": validator.CodeDuplicate})
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("create car", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "car.create", "car", c.ID, map[string]any{
		"plate":      c.Plate,
		"customerId": c.CustomerID,
	})

	renderJSON(w, http.StatusCreated, envelope{"car": carResponse(c)})
}

// updateCar replaces all mutable fields (PUT), or 404 if absent. customer_id is
// immutable. A duplicate plate is a 422 on the plate field.
func (s *Server) updateCar(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.UpdateCarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	normalizeCar(&req.Plate, &req.VIN, &req.Make, &req.Model)

	if errs := validateCar(req.Plate, req.VIN, req.Make, req.Model, req.Year, req.Mileage); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	c := &domain.Car{
		ID:       id,
		TenantID: tenantID,
		Plate:    req.Plate,
		VIN:      req.VIN,
		Make:     req.Make,
		Model:    req.Model,
		Year:     req.Year,
		Mileage:  req.Mileage,
	}
	if err := s.cars.Update(r.Context(), c); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "car not found")
			return
		}
		if errors.Is(err, store.ErrDuplicatePlate) {
			renderValidation(w, r, map[string]string{"plate": validator.CodeDuplicate})
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("update car", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	// Re-read so the response carries the full current row (created_at, customerId).
	updated, err := s.cars.Get(r.Context(), tenantID, id)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("reload car after update", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "car.update", "car", id, map[string]any{
		"plate": updated.Plate,
	})

	renderJSON(w, http.StatusOK, envelope{"car": carResponse(updated)})
}

// archiveCar soft-deletes a car (204), or 404 if not active.
func (s *Server) archiveCar(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	if err := s.cars.Archive(r.Context(), tenantID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "car not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("archive car", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "car.archive", "car", id, nil)

	w.WriteHeader(http.StatusNoContent)
}

// customerExists guards the nested car routes: it confirms the path's customer
// belongs to this tenant, rendering a 404 (and returning false) if not. The
// composite FK enforces the same-tenant relationship at write time; this gives
// a clean 404 up front for read and create.
func (s *Server) customerExists(w http.ResponseWriter, r *http.Request, tenantID, customerID string) bool {
	_, err := s.customers.Get(r.Context(), tenantID, customerID)
	if err == nil {
		return true
	}
	if errors.Is(err, store.ErrNotFound) {
		renderProblem(w, r, http.StatusNotFound, CodeNotFound, "customer not found")
		return false
	}
	loggerFromContext(r.Context(), s.logger).Error("check customer for car", "err", err)
	renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
	return false
}

func carSummaryResponse(sm store.CarSummary) dto.CarSummaryResponse {
	c := sm.Car
	return dto.CarSummaryResponse{
		ID:           c.ID,
		CustomerID:   c.CustomerID,
		CustomerName: sm.CustomerName,
		Plate:        c.Plate,
		Make:         c.Make,
		Model:        c.Model,
		Year:         c.Year,
		Mileage:      c.Mileage,
		ArchivedAt:   c.ArchivedAt,
		CreatedAt:    c.CreatedAt,
	}
}

func carResponse(c *domain.Car) dto.CarResponse {
	return dto.CarResponse{
		ID:         c.ID,
		CustomerID: c.CustomerID,
		Plate:      c.Plate,
		VIN:        c.VIN,
		Make:       c.Make,
		Model:      c.Model,
		Year:       c.Year,
		Mileage:    c.Mileage,
		ArchivedAt: c.ArchivedAt,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

// validateCar enforces the shared create/update rules. plate is required;
// year/mileage are optional (0 = unknown) but range-checked when present.
func validateCar(plate, vin, make, model string, year, mileage int) map[string]string {
	v := validator.New()
	v.NotEmpty("plate", plate)
	v.MaxLength("plate", plate, 20)
	v.MaxLength("vin", vin, 17)
	v.MaxLength("make", make, 100)
	v.MaxLength("model", model, 100)
	if year != 0 {
		v.Range("year", year, 1900, time.Now().Year()+1)
	}
	v.Range("mileage", mileage, 0, 10_000_000)
	return v.Errors()
}

// normalizeCar trims all string fields and upper-cases the plate so uniqueness
// is case-insensitive in practice. plate is the first argument by convention.
func normalizeCar(plate *string, rest ...*string) {
	*plate = strings.ToUpper(strings.TrimSpace(*plate))
	for _, f := range rest {
		*f = strings.TrimSpace(*f)
	}
}
