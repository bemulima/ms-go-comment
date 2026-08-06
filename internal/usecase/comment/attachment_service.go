package comment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

const (
	defaultAttachmentTTLMinutes = 60
	defaultSignedURLMinutes     = 5
	defaultActivationAttempts   = 5
)

type UploadAttachmentInput struct {
	ThreadID uuid.UUID
	Filename string
	Data     []byte
}

type AttachmentWorkResult struct {
	Activated int
	Failed    int
	Deleted   int
}

func (s Service) UploadAttachment(ctx context.Context, actor domain.Actor, input UploadAttachmentInput) (domain.Attachment, error) {
	if err := actor.Validate(); err != nil {
		return domain.Attachment{}, err
	}
	if s.Files == nil {
		return domain.Attachment{}, errors.New("filestorage adapter is not configured")
	}
	thread, _, policy, err := s.loadThreadContext(ctx, actor, input.ThreadID, domain.AccessPermissionUpload)
	if err != nil {
		return domain.Attachment{}, err
	}
	metadata, err := domain.InspectImage(input.Data, input.Filename, policy)
	if err != nil {
		return domain.Attachment{}, err
	}
	now := s.now()
	pending, err := s.Attachments.CountPendingByUploader(ctx, thread.ID, actor.UserID, now)
	if err != nil {
		return domain.Attachment{}, err
	}
	if pending >= int(policy.MaxAttachments) {
		return domain.Attachment{}, fmt.Errorf("%w: pending attachment limit reached", domain.ErrInvalidCommentContent)
	}
	ttlMinutes := s.attachmentTTLMinutes()
	attachmentID := uuidOrNew(s.NewID)
	stored, err := s.Files.UploadTemporary(ctx, TemporaryFileInput{
		OwnerID: attachmentID, Filename: metadata.OriginalFilename, MIMEType: metadata.MIMEType,
		Data: input.Data, TTLMinutes: ttlMinutes,
	})
	if err != nil {
		return domain.Attachment{}, fmt.Errorf("upload temporary attachment: %w", err)
	}
	attachment := domain.Attachment{
		ID: attachmentID, ThreadID: thread.ID, UploaderID: actor.UserID, FileStorageID: stored.ID,
		Status: domain.AttachmentStatusPending, MIMEType: metadata.MIMEType,
		SizeBytes: metadata.SizeBytes, Width: metadata.Width, Height: metadata.Height,
		OriginalFilename: metadata.OriginalFilename, ExpiresAt: now.Add(time.Duration(ttlMinutes) * time.Minute),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := attachment.Validate(policy); err != nil {
		return domain.Attachment{}, err
	}
	if err := s.Attachments.Create(ctx, attachment); err != nil {
		// The FileStorage object intentionally remains temporary and will expire.
		return domain.Attachment{}, err
	}
	return attachment, nil
}

func (s Service) GetAttachmentSignedURL(ctx context.Context, actor domain.Actor, attachmentID uuid.UUID) (SignedFileURL, error) {
	if err := actor.Validate(); err != nil {
		return SignedFileURL{}, err
	}
	if s.Files == nil {
		return SignedFileURL{}, errors.New("filestorage adapter is not configured")
	}
	attachment, err := s.Attachments.GetByID(ctx, attachmentID)
	if err != nil {
		return SignedFileURL{}, mapNotFound(err, domain.ErrAttachmentNotFound)
	}
	if attachment.Status != domain.AttachmentStatusReady || attachment.CommentID == nil {
		return SignedFileURL{}, domain.ErrAttachmentNotReady
	}
	comment, err := s.Comments.GetByID(ctx, *attachment.CommentID)
	if err != nil || comment.Status != domain.CommentStatusActive {
		return SignedFileURL{}, domain.ErrAttachmentNotFound
	}
	if _, _, _, err := s.loadThreadContext(ctx, actor, comment.ThreadID, domain.AccessPermissionRead); err != nil {
		return SignedFileURL{}, err
	}
	minutes := s.signedURLMinutes()
	url, err := s.Files.SignedGETURL(ctx, attachment.FileStorageID, minutes)
	if err != nil {
		return SignedFileURL{}, fmt.Errorf("request attachment signed URL: %w", err)
	}
	return SignedFileURL{URL: url, ExpiresAt: s.now().Add(time.Duration(minutes) * time.Minute)}, nil
}

func (s Service) DeleteAttachment(ctx context.Context, actor domain.Actor, attachmentID uuid.UUID) (domain.Attachment, error) {
	if err := actor.Validate(); err != nil {
		return domain.Attachment{}, err
	}
	var result domain.Attachment
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		attachment, err := s.Attachments.GetByIDForUpdate(txCtx, attachmentID)
		if err != nil {
			return mapNotFound(err, domain.ErrAttachmentNotFound)
		}
		if attachment.UploaderID != actor.UserID {
			return domain.ErrForbidden
		}
		if attachment.Status == domain.AttachmentStatusDeleted {
			result = attachment
			return nil
		}
		thread, _, policy, err := s.loadThreadContext(txCtx, actor, attachment.ThreadID, domain.AccessPermissionWrite)
		if err != nil {
			return err
		}
		now := s.now()
		var ownerComment *domain.Comment
		if attachment.CommentID != nil {
			item, err := s.Comments.GetByIDForUpdate(txCtx, *attachment.CommentID)
			if err != nil || item.Status != domain.CommentStatusActive {
				return domain.ErrAttachmentNotFound
			}
			if item.AuthorID != actor.UserID || policy.EditWindowSeconds == 0 ||
				now.After(item.CreatedAt.Add(time.Duration(policy.EditWindowSeconds)*time.Second)) {
				return domain.ErrForbidden
			}
			attachments, err := s.Attachments.ListByComment(txCtx, thread.ID, item.ID)
			if err != nil {
				return err
			}
			if strings.TrimSpace(item.Body) == "" && len(attachments) <= 1 {
				return fmt.Errorf("%w: comment body or attachment is required", domain.ErrInvalidCommentContent)
			}
			ownerComment = &item
		}
		attachment.Status = domain.AttachmentStatusDeleted
		attachment.DeletedAt = &now
		attachment.DeleteNextAttemptAt = &now
		attachment.LastError = ""
		attachment.UpdatedAt = now
		if err := s.Attachments.UpdateStatus(txCtx, attachment); err != nil {
			return err
		}
		if ownerComment != nil {
			sequence, err := s.Threads.NextSequence(txCtx, thread.ID)
			if err != nil {
				return err
			}
			updated, err := s.Comments.AdvanceSequence(txCtx, ownerComment.ID, sequence, now)
			if err != nil {
				return err
			}
			remaining, err := s.Attachments.ListByComment(txCtx, thread.ID, ownerComment.ID)
			if err != nil {
				return err
			}
			if err := s.addOutbox(txCtx, updated, remaining, domain.EventCommentUpdated, actor.UserID, now); err != nil {
				return err
			}
		}
		result = attachment
		return nil
	})
	return result, err
}

func (s Service) ProcessAttachmentWork(ctx context.Context, limit int) (AttachmentWorkResult, error) {
	if limit < 1 {
		return AttachmentWorkResult{}, fmt.Errorf("%w: worker limit must be positive", domain.ErrValidation)
	}
	if s.Files == nil {
		return AttachmentWorkResult{}, errors.New("filestorage adapter is not configured")
	}
	result := AttachmentWorkResult{}
	var workErrors []error
	if err := s.queueExpiredPendingAttachments(ctx, limit); err != nil {
		workErrors = append(workErrors, err)
	}
	activationItems, err := s.Attachments.ListForActivation(ctx, s.now(), limit)
	if err != nil {
		return result, err
	}
	for _, attachment := range activationItems {
		if err := s.Files.Activate(ctx, attachment.FileStorageID); err != nil {
			terminal, recordErr := s.recordActivationFailure(ctx, attachment, err)
			if terminal {
				result.Failed++
			}
			if recordErr != nil {
				workErrors = append(workErrors, recordErr)
			}
			continue
		}
		changed, err := s.markAttachmentReady(ctx, attachment.ID)
		if err != nil {
			workErrors = append(workErrors, err)
			continue
		}
		if changed {
			result.Activated++
		}
	}
	deleteItems, err := s.Attachments.ListForDeletion(ctx, s.now(), limit)
	if err != nil {
		return result, errors.Join(append(workErrors, err)...)
	}
	for _, attachment := range deleteItems {
		if err := s.Files.Delete(ctx, attachment.FileStorageID); err != nil {
			next := s.now().Add(attachmentRetryDelay(attachment.DeleteAttempts + 1))
			if recordErr := s.Attachments.RecordDeleteFailure(ctx, attachment.ID, next, truncateError(err)); recordErr != nil {
				workErrors = append(workErrors, recordErr)
			}
			continue
		}
		if err := s.Attachments.MarkStorageDeleted(ctx, attachment.ID, s.now()); err != nil && !errors.Is(err, domain.ErrNotFound) {
			workErrors = append(workErrors, err)
			continue
		}
		result.Deleted++
	}
	return result, errors.Join(workErrors...)
}

func (s Service) queueExpiredPendingAttachments(ctx context.Context, limit int) error {
	items, err := s.Attachments.ListExpired(ctx, s.now(), limit)
	if err != nil {
		return err
	}
	var queueErrors []error
	for _, source := range items {
		if source.Status != domain.AttachmentStatusPending {
			continue
		}
		err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
			attachment, err := s.Attachments.GetByIDForUpdate(txCtx, source.ID)
			if err != nil || attachment.Status != domain.AttachmentStatusPending || attachment.ExpiresAt.After(s.now()) {
				return err
			}
			now := s.now()
			attachment.Status = domain.AttachmentStatusDeleted
			attachment.DeletedAt = &now
			attachment.DeleteNextAttemptAt = &now
			attachment.UpdatedAt = now
			return s.Attachments.UpdateStatus(txCtx, attachment)
		})
		if err != nil {
			queueErrors = append(queueErrors, err)
		}
	}
	return errors.Join(queueErrors...)
}

func (s Service) markAttachmentReady(ctx context.Context, attachmentID uuid.UUID) (bool, error) {
	changed := false
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		attachment, updated, err := s.Attachments.MarkReadyIfProcessing(txCtx, attachmentID, s.now())
		if err != nil || !updated {
			return err
		}
		if attachment.CommentID == nil {
			return domain.ErrInvalidAttachment
		}
		if err := s.emitAttachmentLifecycle(txCtx, attachment, domain.EventCommentAttachmentReady); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}

func (s Service) recordActivationFailure(ctx context.Context, source domain.Attachment, cause error) (bool, error) {
	terminal := false
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		next := s.now().Add(attachmentRetryDelay(source.ActivationAttempts + 1))
		attachment, becameTerminal, err := s.Attachments.RecordActivationFailure(
			txCtx, source.ID, next, truncateError(cause), s.activationMaxAttempts(),
		)
		if err != nil {
			return err
		}
		terminal = becameTerminal
		if !becameTerminal {
			return nil
		}
		return s.emitAttachmentLifecycle(txCtx, attachment, domain.EventCommentAttachmentFailed)
	})
	return terminal, err
}

func (s Service) emitAttachmentLifecycle(ctx context.Context, attachment domain.Attachment, subject domain.EventSubject) error {
	if attachment.CommentID == nil {
		return domain.ErrInvalidAttachment
	}
	comment, err := s.Comments.GetByIDForUpdate(ctx, *attachment.CommentID)
	if err != nil {
		return err
	}
	sequence, err := s.Threads.NextSequenceAnyState(ctx, comment.ThreadID)
	if err != nil {
		return err
	}
	comment, err = s.Comments.AdvanceSequence(ctx, comment.ID, sequence, s.now())
	if err != nil {
		return err
	}
	attachments, err := s.Attachments.ListByComment(ctx, comment.ThreadID, comment.ID)
	if err != nil {
		return err
	}
	return s.addOutbox(ctx, comment, attachments, subject, attachment.UploaderID, s.now())
}

func (s Service) attachmentTTLMinutes() int {
	if s.AttachmentTTLMinutes > 0 {
		return s.AttachmentTTLMinutes
	}
	return defaultAttachmentTTLMinutes
}

func (s Service) signedURLMinutes() int {
	if s.SignedURLMinutes > 0 {
		return s.SignedURLMinutes
	}
	return defaultSignedURLMinutes
}

func (s Service) activationMaxAttempts() int {
	if s.ActivationMaxAttempts > 0 {
		return s.ActivationMaxAttempts
	}
	return defaultActivationAttempts
}

func attachmentRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second * time.Duration(1<<min(attempt-1, 6))
	return min(delay, 5*time.Minute)
}

func truncateError(err error) string {
	message := err.Error()
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}
