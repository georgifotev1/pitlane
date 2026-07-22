package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/validator"
	"github.com/google/uuid"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// listCustomers returns a page of customers with pagination metadata. Search
// params come off the URL query string (page, pageSize, search, archived).
func (s *Server) listCustomers(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())

	page := clampAtLeast(queryInt(r, "page", 1), 1)
	pageSize := clampRange(queryInt(r, "pageSize", defaultPageSize), 1, maxPageSize)
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	includeArchived := r.URL.Query().Get("archived") == "true"

	customers, total, err := s.customers.List(r.Context(), tenantID, store.CustomerListParams{
		Search:          search,
		IncludeArchived: includeArchived,
		Limit:           pageSize,
		Offset:          (page - 1) * pageSize,
	})
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list customers", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.CustomerResponse, 0, len(customers))
	for _, c := range customers {
		items = append(items, customerResponse(c))
	}
	renderJSON(w, http.StatusOK, envelope{
		"customers": items,
		"metadata":  dto.ListMetadata{Page: page, PageSize: pageSize, Total: total},
	})
}

// getCustomer returns one customer or 404.
func (s *Server) getCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	id := r.PathValue("id")

	c, err := s.customers.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "customer not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("get customer", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	renderJSON(w, http.StatusOK, envelope{"customer": customerResponse(c)})
}

// createCustomer validates and inserts a customer, then writes the audit log.
func (s *Server) createCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())

	var req dto.CreateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	trimCustomer(&req.Name, &req.Company, &req.Email, &req.Phone, &req.Address, &req.Notes)

	if errs := validateCustomer(req.Name, req.Company, req.Email, req.Phone, req.Address, req.Notes); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	c := &domain.Customer{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     req.Name,
		Company:  req.Company,
		Email:    req.Email,
		Phone:    req.Phone,
		Address:  req.Address,
		Notes:    req.Notes,
	}
	if err := s.customers.Create(r.Context(), c); err != nil {
		loggerFromContext(r.Context(), s.logger).Error("create customer", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "customer.create", "customer", c.ID, map[string]any{
		"name": c.Name,
	})

	renderJSON(w, http.StatusCreated, envelope{"customer": customerResponse(c)})
}

// updateCustomer replaces all mutable fields (PUT), or 404 if absent.
func (s *Server) updateCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.UpdateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	trimCustomer(&req.Name, &req.Company, &req.Email, &req.Phone, &req.Address, &req.Notes)

	if errs := validateCustomer(req.Name, req.Company, req.Email, req.Phone, req.Address, req.Notes); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	c := &domain.Customer{
		ID:       id,
		TenantID: tenantID,
		Name:     req.Name,
		Company:  req.Company,
		Email:    req.Email,
		Phone:    req.Phone,
		Address:  req.Address,
		Notes:    req.Notes,
	}
	if err := s.customers.Update(r.Context(), c); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "customer not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("update customer", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	// Re-read so the response carries the full current row (created_at etc.).
	updated, err := s.customers.Get(r.Context(), tenantID, id)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("reload customer after update", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "customer.update", "customer", id, map[string]any{
		"name": updated.Name,
	})

	renderJSON(w, http.StatusOK, envelope{"customer": customerResponse(updated)})
}

// archiveCustomer soft-deletes a customer (204), or 404 if not active.
func (s *Server) archiveCustomer(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	if err := s.customers.Archive(r.Context(), tenantID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "customer not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("archive customer", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "customer.archive", "customer", id, nil)

	w.WriteHeader(http.StatusNoContent)
}

func customerResponse(c *domain.Customer) dto.CustomerResponse {
	return dto.CustomerResponse{
		ID:         c.ID,
		Name:       c.Name,
		Company:    c.Company,
		Email:      c.Email,
		Phone:      c.Phone,
		Address:    c.Address,
		Notes:      c.Notes,
		ArchivedAt: c.ArchivedAt,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

// validateCustomer enforces the shared create/update rules. Email is optional,
// but validated when present.
func validateCustomer(name, company, email, phone, address, notes string) map[string]string {
	v := validator.New()
	v.NotEmpty("name", name)
	v.MaxLength("name", name, 200)
	v.MaxLength("company", company, 200)
	if email != "" {
		v.Email("email", email)
		v.MaxLength("email", email, 254)
	}
	v.MaxLength("phone", phone, 50)
	v.MaxLength("address", address, 500)
	v.MaxLength("notes", notes, 2000)
	return v.Errors()
}

func trimCustomer(fields ...*string) {
	for _, f := range fields {
		*f = strings.TrimSpace(*f)
	}
}

// queryInt reads an integer query param, returning def on absent/invalid.
func queryInt(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func clampAtLeast(n, min int) int {
	if n < min {
		return min
	}
	return n
}

func clampRange(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
