package postgres

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AttachmentRepository struct {
	Pool *pgxpool.Pool
}

func (r AttachmentRepository) Create(ctx context.Context, item domain.Attachment) error {
	_, err := runner(ctx, r.Pool).Exec(ctx, `INSERT INTO comment_attachment (
id, thread_id, comment_id, uploader_id, filestorage_id, status, mime_type,
size_bytes, width, height, original_filename, expires_at, activated_at,
deleted_at, activation_attempts, activation_next_attempt_at, delete_attempts,
delete_next_attempt_at, last_error, storage_deleted_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		item.ID, item.ThreadID, item.CommentID, item.UploaderID, item.FileStorageID,
		item.Status, item.MIMEType, item.SizeBytes, item.Width, item.Height,
		item.OriginalFilename, item.ExpiresAt, item.ActivatedAt, item.DeletedAt,
		item.ActivationAttempts, item.ActivationNextAttemptAt, item.DeleteAttempts,
		item.DeleteNextAttemptAt, nullableString(item.LastError), item.StorageDeletedAt,
		item.CreatedAt, item.UpdatedAt)
	return mapError(err)
}

func (r AttachmentRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Attachment, error) {
	return scanAttachment(runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT `+attachmentColumns+` FROM comment_attachment WHERE id=$1 FOR UPDATE`, id))
}

func (r AttachmentRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Attachment, error) {
	return scanAttachment(runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT `+attachmentColumns+` FROM comment_attachment WHERE id=$1`, id))
}

func (r AttachmentRepository) ListByComment(ctx context.Context, threadID, commentID uuid.UUID) ([]domain.Attachment, error) {
	rows, err := runner(ctx, r.Pool).Query(ctx, `SELECT `+attachmentColumns+`
FROM comment_attachment WHERE thread_id=$1 AND comment_id=$2 AND status<>5
ORDER BY created_at, id`, threadID, commentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Attachment, 0)
	for rows.Next() {
		item, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r AttachmentRepository) CountPendingByUploader(ctx context.Context, threadID, uploaderID uuid.UUID, now time.Time) (int, error) {
	var count int
	err := runner(ctx, r.Pool).QueryRow(ctx, `SELECT COUNT(*) FROM comment_attachment
WHERE thread_id=$1 AND uploader_id=$2 AND status=1 AND comment_id IS NULL AND expires_at>$3`,
		threadID, uploaderID, now).Scan(&count)
	return count, err
}

func (r AttachmentRepository) BindToComment(ctx context.Context, attachmentID, threadID, commentID, uploaderID uuid.UUID) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_attachment SET
comment_id=$1, status=2, updated_at=NOW()
WHERE id=$2 AND thread_id=$3 AND uploader_id=$4 AND comment_id IS NULL
AND status=1 AND expires_at>NOW()`, commentID, attachmentID, threadID, uploaderID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrInvalidCommentContent
	}
	return nil
}

func (r AttachmentRepository) UpdateStatus(ctx context.Context, item domain.Attachment) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_attachment SET
status=$1, activated_at=$2, deleted_at=$3, activation_attempts=$4,
activation_next_attempt_at=$5, delete_attempts=$6, delete_next_attempt_at=$7,
last_error=$8, storage_deleted_at=$9, updated_at=$10 WHERE id=$11`,
		item.Status, item.ActivatedAt, item.DeletedAt, item.ActivationAttempts,
		item.ActivationNextAttemptAt, item.DeleteAttempts, item.DeleteNextAttemptAt,
		nullableString(item.LastError), item.StorageDeletedAt, item.UpdatedAt, item.ID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r AttachmentRepository) ListForActivation(ctx context.Context, now time.Time, limit int) ([]domain.Attachment, error) {
	rows, err := runner(ctx, r.Pool).Query(ctx, `SELECT `+attachmentColumns+`
FROM comment_attachment WHERE status=2
AND (activation_next_attempt_at IS NULL OR activation_next_attempt_at<=$1)
ORDER BY activation_next_attempt_at NULLS FIRST, id LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttachments(rows)
}

func (r AttachmentRepository) MarkReadyIfProcessing(ctx context.Context, id uuid.UUID, now time.Time) (domain.Attachment, bool, error) {
	item, err := scanAttachment(runner(ctx, r.Pool).QueryRow(ctx, `UPDATE comment_attachment SET
status=3, activated_at=$2, activation_next_attempt_at=NULL, last_error=NULL, updated_at=$2
WHERE id=$1 AND status=2 RETURNING `+attachmentColumns, id, now))
	if err == domain.ErrNotFound {
		return domain.Attachment{}, false, nil
	}
	return item, err == nil, err
}

func (r AttachmentRepository) RecordActivationFailure(ctx context.Context, id uuid.UUID, next time.Time, message string, maxAttempts int) (domain.Attachment, bool, error) {
	item, err := scanAttachment(runner(ctx, r.Pool).QueryRow(ctx, `UPDATE comment_attachment SET
activation_attempts=activation_attempts+1,
status=CASE WHEN activation_attempts+1 >= $4 THEN 4 ELSE 2 END,
activation_next_attempt_at=CASE WHEN activation_attempts+1 >= $4 THEN NULL ELSE $2 END,
last_error=$3, updated_at=NOW()
WHERE id=$1 AND status=2 RETURNING `+attachmentColumns, id, next, message, maxAttempts))
	if err == domain.ErrNotFound {
		return domain.Attachment{}, false, nil
	}
	if err != nil {
		return domain.Attachment{}, false, err
	}
	return item, item.Status == domain.AttachmentStatusFailed, nil
}

func (r AttachmentRepository) ListForDeletion(ctx context.Context, now time.Time, limit int) ([]domain.Attachment, error) {
	rows, err := runner(ctx, r.Pool).Query(ctx, `SELECT `+attachmentColumns+`
FROM comment_attachment WHERE status=5 AND storage_deleted_at IS NULL
AND (delete_next_attempt_at IS NULL OR delete_next_attempt_at<=$1)
ORDER BY delete_next_attempt_at NULLS FIRST, id LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttachments(rows)
}

func (r AttachmentRepository) MarkStorageDeleted(ctx context.Context, id uuid.UUID, now time.Time) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_attachment SET
storage_deleted_at=$2, delete_next_attempt_at=NULL, last_error=NULL, updated_at=$2
WHERE id=$1 AND status=5 AND storage_deleted_at IS NULL`, id, now)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r AttachmentRepository) RecordDeleteFailure(ctx context.Context, id uuid.UUID, next time.Time, message string) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_attachment SET
delete_attempts=delete_attempts+1, delete_next_attempt_at=$2, last_error=$3, updated_at=NOW()
WHERE id=$1 AND status=5 AND storage_deleted_at IS NULL`, id, next, message)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanAttachments(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]domain.Attachment, error) {
	items := make([]domain.Attachment, 0)
	for rows.Next() {
		item, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r AttachmentRepository) ListExpired(ctx context.Context, before time.Time, limit int) ([]domain.Attachment, error) {
	rows, err := runner(ctx, r.Pool).Query(ctx, `SELECT `+attachmentColumns+`
FROM comment_attachment WHERE status=1 AND expires_at<$1
ORDER BY expires_at, id LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Attachment, 0)
	for rows.Next() {
		item, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

var _ repository.AttachmentRepository = (*AttachmentRepository)(nil)
