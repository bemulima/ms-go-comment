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
deleted_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		item.ID, item.ThreadID, item.CommentID, item.UploaderID, item.FileStorageID,
		item.Status, item.MIMEType, item.SizeBytes, item.Width, item.Height,
		item.OriginalFilename, item.ExpiresAt, item.ActivatedAt, item.DeletedAt,
		item.CreatedAt, item.UpdatedAt)
	return mapError(err)
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
status=$1, activated_at=$2, deleted_at=$3, updated_at=$4 WHERE id=$5`,
		item.Status, item.ActivatedAt, item.DeletedAt, item.UpdatedAt, item.ID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r AttachmentRepository) ListExpired(ctx context.Context, before time.Time, limit int) ([]domain.Attachment, error) {
	rows, err := runner(ctx, r.Pool).Query(ctx, `SELECT `+attachmentColumns+`
FROM comment_attachment WHERE status IN (1,4) AND expires_at<$1
ORDER BY expires_at, id LIMIT $2 FOR UPDATE SKIP LOCKED`, before, limit)
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
