package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/validator"
	"github.com/google/uuid"
)

// MaxAttachmentSize is the largest upload the API will accept (10 MiB).
const maxAttachmentSize = 10 * 1024 * 1024

// listCarAttachments returns the photos/documents attached to a car.
func (s *Server) listCarAttachments(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	carID := r.PathValue("carId")

	if !s.carExists(w, r, tenantID, carID) {
		return
	}

	atts, err := s.attachments.ListByCar(r.Context(), tenantID, carID)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list car attachments", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.AttachmentResponse, 0, len(atts))
	for _, a := range atts {
		items = append(items, attachmentResponse(&a))
	}
	renderJSON(w, http.StatusOK, envelope{"attachments": items})
}

// listRepairAttachments returns the photos/documents attached to a repair.
func (s *Server) listRepairAttachments(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	repairID := r.PathValue("repairId")

	if !s.repairExists(w, r, tenantID, repairID) {
		return
	}

	atts, err := s.attachments.ListByRepair(r.Context(), tenantID, repairID)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list repair attachments", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	items := make([]dto.AttachmentResponse, 0, len(atts))
	for _, a := range atts {
		items = append(items, attachmentResponse(&a))
	}
	renderJSON(w, http.StatusOK, envelope{"attachments": items})
}

// uploadCarAttachment handles a multipart file upload for a car.
func (s *Server) uploadCarAttachment(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	carID := r.PathValue("carId")

	if !s.carExists(w, r, tenantID, carID) {
		return
	}

	s.uploadAttachment(w, r, tenantID, userID, &carID, nil)
}

// uploadRepairAttachment handles a multipart file upload for a repair.
func (s *Server) uploadRepairAttachment(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	repairID := r.PathValue("repairId")

	if !s.repairExists(w, r, tenantID, repairID) {
		return
	}

	s.uploadAttachment(w, r, tenantID, userID, nil, &repairID)
}

// uploadAttachment parses the multipart body, validates the file, stores it in
// object storage, and writes the metadata row. It renders the response and
// audit log.
func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request, tenantID, userID string, carID, repairID *string) {
	// Cap the whole request body before parsing so an oversized upload is
	// rejected up front — the size limit is enforced here, not after buffering
	// the file to a multipart temp file on disk. The extra 1 MiB covers the
	// multipart framing overhead (boundaries, the filename field, headers).
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentSize+(1<<20))

	if err := r.ParseMultipartForm(maxAttachmentSize + 1024); err != nil {
		// A body past the MaxBytesReader limit surfaces here as *http.MaxBytesError.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			renderProblem(w, r, http.StatusBadRequest, CodeValidationFailed, "file exceeds maximum size")
			return
		}
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid multipart form")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "missing file field")
		return
	}
	defer file.Close()

	name := strings.TrimSpace(path.Base(header.Filename))
	if name == "" || name == "." || name == "/" {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid file name")
		return
	}
	if errs := validateAttachmentName(name); errs != nil {
		renderValidation(w, r, errs)
		return
	}

	// Buffer the file, reading one byte past the cap so an oversized file is
	// still caught if it slipped under the multipart-framing headroom above.
	var full bytes.Buffer
	written, err := io.Copy(&full, io.LimitReader(file, maxAttachmentSize+1))
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("buffer attachment", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	if written > maxAttachmentSize {
		renderProblem(w, r, http.StatusBadRequest, CodeValidationFailed, "file exceeds maximum size")
		return
	}
	if written == 0 {
		// An empty file would violate the size_bytes > 0 DB CHECK; reject cleanly.
		renderProblem(w, r, http.StatusBadRequest, CodeValidationFailed, "file is empty")
		return
	}

	// Trust the sniffed content type, never the client-supplied one. A spoofed
	// type therefore has no effect: the real bytes decide, and downloads are
	// served with this type plus nosniff + attachment disposition.
	// http.DetectContentType inspects only the first 512 bytes.
	contentType := http.DetectContentType(full.Bytes())

	att := &domain.Attachment{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		CarID:       carID,
		RepairID:    repairID,
		Name:        name,
		StorageKey:  store.AttachmentStorageKey(tenantID, uuid.NewString(), name),
		SizeBytes:   written,
		ContentType: contentType,
	}
	// The key must be deterministic per attachment, so regenerate with the real ID.
	att.StorageKey = store.AttachmentStorageKey(tenantID, att.ID, name)

	if err := s.files.Put(r.Context(), att.StorageKey, contentType, bytes.NewReader(full.Bytes())); err != nil {
		loggerFromContext(r.Context(), s.logger).Error("store attachment", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	if err := s.attachments.Create(r.Context(), att); err != nil {
		// Best-effort cleanup: delete the orphaned object. If this fails, the
		// object is harmless (tenant-prefixed, unreferenced).
		_ = s.files.Delete(r.Context(), att.StorageKey)
		loggerFromContext(r.Context(), s.logger).Error("create attachment metadata", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "attachment.create", "attachment", att.ID, map[string]any{
		"name":      att.Name,
		"carId":     carID,
		"repairId":  repairID,
		"sizeBytes": att.SizeBytes,
	})

	renderJSON(w, http.StatusCreated, envelope{"attachment": attachmentResponse(att)})
}

// downloadAttachment streams an attachment from object storage with a strong
// Content-Disposition: attachment header so browsers download rather than display
// user-uploaded content.
func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	id := r.PathValue("id")

	att, err := s.attachments.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "attachment not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("get attachment metadata", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	obj, err := s.files.Get(r.Context(), att.StorageKey)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("get attachment object", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	defer obj.Body.Close()

	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Name))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj.Body)
}

// deleteAttachment removes the metadata row and the object from storage.
func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	att, err := s.attachments.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "attachment not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("get attachment for delete", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	if err := s.attachments.Delete(r.Context(), tenantID, id); err != nil {
		loggerFromContext(r.Context(), s.logger).Error("delete attachment metadata", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	// Best-effort object cleanup; if it fails, the metadata is gone so the row
	// won't point at it again.
	_ = s.files.Delete(r.Context(), att.StorageKey)

	_ = s.audit.Insert(r.Context(), tenantID, userID, "attachment.delete", "attachment", id, map[string]any{
		"name": att.Name,
	})

	w.WriteHeader(http.StatusNoContent)
}

func validateAttachmentName(name string) map[string]string {
	v := validator.New()
	v.NotEmpty("name", name)
	v.MaxLength("name", name, 255)
	return v.Errors()
}

func attachmentResponse(a *domain.Attachment) dto.AttachmentResponse {
	return dto.AttachmentResponse{
		ID:          a.ID,
		CarID:       a.CarID,
		RepairID:    a.RepairID,
		Name:        a.Name,
		SizeBytes:   a.SizeBytes,
		ContentType: a.ContentType,
		CreatedAt:   a.CreatedAt.UTC().Format(time.RFC3339),
	}
}
