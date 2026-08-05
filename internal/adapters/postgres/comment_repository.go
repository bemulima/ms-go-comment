package postgres

import (
	"context"
	"encoding/json"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CommentRepository struct {
	Pool *pgxpool.Pool
}

func (r CommentRepository) Create(ctx context.Context, item domain.Comment) error {
	links, err := json.Marshal(item.Links)
	if err != nil {
		return err
	}
	_, err = runner(ctx, r.Pool).Exec(ctx, `INSERT INTO comment (
id, thread_id, author_id, parent_id, root_id, path, depth, body, links, status,
version, sequence, direct_replies_count, idempotency_key, edited_at, deleted_at,
created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		item.ID, item.ThreadID, item.AuthorID, item.ParentID, item.RootID, item.Path,
		item.Depth, item.Body, links, item.Status, item.Version, item.Sequence,
		item.DirectRepliesCount, item.IdempotencyKey, item.EditedAt, item.DeletedAt,
		item.CreatedAt, item.UpdatedAt)
	return mapError(err)
}

func (r CommentRepository) GetByID(ctx context.Context, commentID uuid.UUID) (domain.Comment, error) {
	return scanComment(runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT `+commentColumns+` FROM comment WHERE id=$1`, commentID))
}

func (r CommentRepository) GetByIDForUpdate(ctx context.Context, commentID uuid.UUID) (domain.Comment, error) {
	return scanComment(runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT `+commentColumns+` FROM comment WHERE id=$1 FOR UPDATE`, commentID))
}

func (r CommentRepository) GetByIdempotencyKey(ctx context.Context, authorID, key uuid.UUID) (domain.Comment, error) {
	return scanComment(runner(ctx, r.Pool).QueryRow(ctx, `SELECT `+commentColumns+`
FROM comment WHERE author_id=$1 AND idempotency_key=$2`, authorID, key))
}

func (r CommentRepository) LockIdempotencyKey(ctx context.Context, authorID, key uuid.UUID) error {
	_, err := runner(ctx, r.Pool).Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, authorID.String()+":"+key.String())
	return err
}

func (r CommentRepository) List(ctx context.Context, query repository.CommentListQuery) ([]domain.Comment, error) {
	base := `SELECT ` + commentColumns + ` FROM comment
WHERE thread_id=$1 AND parent_id IS NOT DISTINCT FROM $2 AND status<>3`
	args := []any{query.ThreadID, query.ParentID}
	if query.After != nil {
		base += ` AND (created_at, id) > ($3, $4) ORDER BY created_at, id LIMIT $5`
		args = append(args, query.After.CreatedAt, query.After.ID, query.Limit)
	} else {
		base += ` ORDER BY created_at, id LIMIT $3`
		args = append(args, query.Limit)
	}
	rows, err := runner(ctx, r.Pool).Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanComments(rows)
}

func (r CommentRepository) ListChanges(ctx context.Context, query repository.CommentChangeQuery) ([]domain.Comment, error) {
	rows, err := runner(ctx, r.Pool).Query(ctx, `SELECT `+commentColumns+`
FROM comment WHERE thread_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`,
		query.ThreadID, query.AfterSequence, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanComments(rows)
}

func (r CommentRepository) UpdateContent(ctx context.Context, item domain.Comment, expectedVersion int) error {
	links, err := json.Marshal(item.Links)
	if err != nil {
		return err
	}
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment SET
body=$1, links=$2, version=$3, sequence=$4, edited_at=$5, updated_at=$6
WHERE id=$7 AND version=$8 AND status=1`, item.Body, links, item.Version,
		item.Sequence, item.EditedAt, item.UpdatedAt, item.ID, expectedVersion)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrEditConflict
	}
	return nil
}

func (r CommentRepository) MarkDeleted(ctx context.Context, item domain.Comment, expectedVersion int) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment SET
body='', links='[]'::jsonb, status=$1, version=$2, sequence=$3,
deleted_at=$4, updated_at=$5
WHERE id=$6 AND version=$7 AND status=1`, item.Status, item.Version, item.Sequence,
		item.DeletedAt, item.UpdatedAt, item.ID, expectedVersion)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrEditConflict
	}
	return nil
}

func (r CommentRepository) IncrementReplyCount(ctx context.Context, threadID, commentID uuid.UUID) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment
SET direct_replies_count=direct_replies_count+1, updated_at=NOW()
WHERE thread_id=$1 AND id=$2`, threadID, commentID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

var _ repository.CommentRepository = (*CommentRepository)(nil)
