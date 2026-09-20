package handlers

import (
	"io"
	"net/http"

	"github.com/bemulima/ms-go-comment/internal/domain"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxMultipartBodyBytes = domain.MaxAllowedImageBytes + (1 << 20)

func (h CommentHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBodyBytes)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		WriteError(w, domain.ErrInvalidAttachment)
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	threadID, err := uuid.Parse(r.FormValue("thread_id"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, domain.ErrInvalidAttachment)
		return
	}
	defer file.Close()
	if header.Size < 1 || header.Size > domain.MaxAllowedImageBytes {
		WriteError(w, domain.ErrInvalidAttachment)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, domain.MaxAllowedImageBytes+1))
	if err != nil || int64(len(data)) > domain.MaxAllowedImageBytes {
		WriteError(w, domain.ErrInvalidAttachment)
		return
	}
	attachment, err := h.Service.UploadAttachment(r.Context(), mustActor(r), commentuc.UploadAttachmentInput{
		ThreadID: threadID, Filename: header.Filename, Data: data,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectAttachment(attachment))
}

func (h CommentHandler) GetAttachmentSignedURL(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "attachmentID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	result, err := h.Service.GetAttachmentSignedURL(r.Context(), mustActor(r), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": result.URL, "expires_at": result.ExpiresAt})
}

func (h CommentHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "attachmentID"))
	if err != nil {
		WriteError(w, domain.ErrValidation)
		return
	}
	attachment, err := h.Service.DeleteAttachment(r.Context(), mustActor(r), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectAttachment(attachment))
}
