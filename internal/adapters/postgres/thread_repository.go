package postgres

import (
	"context"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ThreadRepository struct {
	Pool *pgxpool.Pool
}

func (r ThreadRepository) Ensure(ctx context.Context, item domain.Thread) (domain.Thread, error) {
	return scanThread(runner(ctx, r.Pool).QueryRow(ctx, `INSERT INTO comment_thread (
id, space_id, resource_type, resource_id, status, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (space_id, resource_type, resource_id)
DO UPDATE SET resource_id=EXCLUDED.resource_id
RETURNING `+threadColumns,
		item.ID, item.SpaceID, item.Resource.Type, item.Resource.ID, item.Status,
		item.CreatedAt, item.UpdatedAt))
}

func (r ThreadRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Thread, error) {
	return scanThread(runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT `+threadColumns+` FROM comment_thread WHERE id=$1`, id))
}

func (r ThreadRepository) GetByResource(ctx context.Context, spaceID uuid.UUID, resource domain.ResourceReference) (domain.Thread, error) {
	return scanThread(runner(ctx, r.Pool).QueryRow(ctx, `SELECT `+threadColumns+`
FROM comment_thread WHERE space_id=$1 AND resource_type=$2 AND resource_id=$3`,
		spaceID, resource.Type, resource.ID))
}

func (r ThreadRepository) Update(ctx context.Context, item domain.Thread) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_thread SET
status=$1, allow_images=$2, allow_links=$3, max_depth=$4, max_body_length=$5,
max_attachments=$6, max_image_bytes=$7, edit_window_seconds=$8, updated_at=$9
WHERE id=$10`, item.Status, item.PolicyOverrides.AllowImages, item.PolicyOverrides.AllowLinks,
		item.PolicyOverrides.MaxDepth, item.PolicyOverrides.MaxBodyLength,
		item.PolicyOverrides.MaxAttachments, item.PolicyOverrides.MaxImageBytes,
		item.PolicyOverrides.EditWindowSeconds, item.UpdatedAt, item.ID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r ThreadRepository) NextSequence(ctx context.Context, threadID uuid.UUID) (int64, error) {
	var sequence int64
	err := runner(ctx, r.Pool).QueryRow(ctx, `UPDATE comment_thread
SET last_sequence=last_sequence+1, updated_at=NOW()
WHERE id=$1 AND status=1 RETURNING last_sequence`, threadID).Scan(&sequence)
	if err == nil {
		return sequence, nil
	}
	if err != pgx.ErrNoRows {
		return 0, mapError(err)
	}
	var exists bool
	if checkErr := runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM comment_thread WHERE id=$1)`, threadID).Scan(&exists); checkErr != nil {
		return 0, checkErr
	}
	if exists {
		return 0, domain.ErrThreadNotWritable
	}
	return 0, domain.ErrNotFound
}

func (r ThreadRepository) NextSequenceAnyState(ctx context.Context, threadID uuid.UUID) (int64, error) {
	var sequence int64
	err := runner(ctx, r.Pool).QueryRow(ctx, `UPDATE comment_thread
SET last_sequence=last_sequence+1, updated_at=NOW()
WHERE id=$1 RETURNING last_sequence`, threadID).Scan(&sequence)
	return sequence, mapError(err)
}

func (r ThreadRepository) RecordCommentCreated(ctx context.Context, threadID uuid.UUID, root bool) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_thread SET
comment_count=comment_count+1,
root_comment_count=root_comment_count+CASE WHEN $2 THEN 1 ELSE 0 END,
last_comment_at=NOW(), updated_at=NOW()
WHERE id=$1`, threadID, root)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

var _ repository.ThreadRepository = (*ThreadRepository)(nil)
