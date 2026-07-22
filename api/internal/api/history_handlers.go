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

// getCarHistory returns the service-history timeline for a car: completed
// repairs union manual notes, sorted newest first.
func (s *Server) getCarHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	carID := r.PathValue("carId")

	if !s.carExists(w, r, tenantID, carID) {
		return
	}

	entries, err := s.history.ListByCar(r.Context(), tenantID, carID)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list car history", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.HistoryEntryResponse, 0, len(entries))
	for _, e := range entries {
		items = append(items, dto.HistoryEntryResponse{
			Type:        e.Type,
			ID:          e.ID,
			Title:       e.Title,
			Description: e.Description,
			RecordedAt:  e.RecordedAt.UTC().Format(time.RFC3339),
			Mileage:     e.Mileage,
			TotalCents:  e.TotalCents,
		})
	}

	renderJSON(w, http.StatusOK, envelope{"history": items})
}

// createHistoryNote adds a manual entry to a car's service history.
func (s *Server) createHistoryNote(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	carID := r.PathValue("carId")

	if !s.carExists(w, r, tenantID, carID) {
		return
	}

	var req dto.CreateHistoryNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	note := &domain.HistoryNote{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		CarID:       carID,
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
	}
	if req.RecordedAt != "" {
		t, err := time.Parse(time.RFC3339, req.RecordedAt)
		if err != nil {
			renderValidation(w, r, map[string]string{"recordedAt": validator.CodeInvalid})
			return
		}
		note.RecordedAt = t
	} else {
		note.RecordedAt = time.Now()
	}

	if errs := validateHistoryNote(note.Title, note.Description); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	if err := s.history.CreateNote(r.Context(), note); err != nil {
		loggerFromContext(r.Context(), s.logger).Error("create history note", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "history_note.create", "history_note", note.ID, map[string]any{
		"carId": carID,
		"title": note.Title,
	})

	renderJSON(w, http.StatusCreated, envelope{"note": historyNoteResponse(note)})
}

// updateHistoryNote rewrites a manual history note.
func (s *Server) updateHistoryNote(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	var req dto.UpdateHistoryNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}

	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	if errs := validateHistoryNote(title, description); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	recordedAt := time.Now()
	if req.RecordedAt != "" {
		t, err := time.Parse(time.RFC3339, req.RecordedAt)
		if err != nil {
			renderValidation(w, r, map[string]string{"recordedAt": validator.CodeInvalid})
			return
		}
		recordedAt = t
	}

	existing, err := s.history.GetNote(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "note not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("get history note for update", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	existing.Title = title
	existing.Description = description
	existing.RecordedAt = recordedAt
	if err := s.history.UpdateNote(r.Context(), existing); err != nil {
		loggerFromContext(r.Context(), s.logger).Error("update history note", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "history_note.update", "history_note", id, map[string]any{
		"carId": existing.CarID,
		"title": title,
	})

	renderJSON(w, http.StatusOK, envelope{"note": historyNoteResponse(existing)})
}

// deleteHistoryNote removes a manual history note.
func (s *Server) deleteHistoryNote(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	if err := s.history.DeleteNote(r.Context(), tenantID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "note not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("delete history note", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "history_note.delete", "history_note", id, nil)

	w.WriteHeader(http.StatusNoContent)
}

// validateHistoryNote enforces the shared create/update rules for notes.
func validateHistoryNote(title, description string) map[string]string {
	v := validator.New()
	v.NotEmpty("title", title)
	v.MaxLength("title", title, 200)
	v.MaxLength("description", description, 5000)
	return v.Errors()
}

func historyNoteResponse(n *domain.HistoryNote) dto.HistoryNoteResponse {
	return dto.HistoryNoteResponse{
		ID:          n.ID,
		CarID:       n.CarID,
		Title:       n.Title,
		Description: n.Description,
		RecordedAt:  n.RecordedAt.UTC().Format(time.RFC3339),
		CreatedAt:   n.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   n.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// repairExists guards nested routes, confirming the repair belongs to the tenant.
func (s *Server) repairExists(w http.ResponseWriter, r *http.Request, tenantID, repairID string) bool {
	_, err := s.repairs.Get(r.Context(), tenantID, repairID)
	if err == nil {
		return true
	}
	if errors.Is(err, store.ErrNotFound) {
		renderProblem(w, r, http.StatusNotFound, CodeNotFound, "repair not found")
		return false
	}
	loggerFromContext(r.Context(), s.logger).Error("check repair", "err", err)
	renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
	return false
}
