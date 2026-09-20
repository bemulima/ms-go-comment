package postgres

import (
	"context"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SpaceRepository struct {
	Pool *pgxpool.Pool
}

func (r SpaceRepository) Create(ctx context.Context, item domain.Space) error {
	_, err := runner(ctx, r.Pool).Exec(ctx, `INSERT INTO comment_space (
id, key, name, status, access_mode, allowed_origins, allow_images, allow_links,
max_depth, max_body_length, max_attachments, max_image_bytes, edit_window_seconds,
created_by, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		item.ID, item.Key, item.Name, item.Status, item.AccessMode, item.AllowedOrigins,
		item.Policy.AllowImages, item.Policy.AllowLinks, item.Policy.MaxDepth,
		item.Policy.MaxBodyLength, item.Policy.MaxAttachments, item.Policy.MaxImageBytes,
		item.Policy.EditWindowSeconds, item.CreatedBy, item.CreatedAt, item.UpdatedAt,
	)
	return mapError(err)
}

func (r SpaceRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Space, error) {
	return scanSpace(runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT `+spaceColumns+` FROM comment_space WHERE id=$1`, id))
}

func (r SpaceRepository) GetByKey(ctx context.Context, key string) (domain.Space, error) {
	return scanSpace(runner(ctx, r.Pool).QueryRow(ctx,
		`SELECT `+spaceColumns+` FROM comment_space WHERE key=$1`, key))
}

func (r SpaceRepository) Update(ctx context.Context, item domain.Space) error {
	command, err := runner(ctx, r.Pool).Exec(ctx, `UPDATE comment_space SET
key=$1, name=$2, status=$3, access_mode=$4, allowed_origins=$5,
allow_images=$6, allow_links=$7, max_depth=$8, max_body_length=$9,
max_attachments=$10, max_image_bytes=$11, edit_window_seconds=$12, updated_at=$13
WHERE id=$14`, item.Key, item.Name, item.Status, item.AccessMode, item.AllowedOrigins,
		item.Policy.AllowImages, item.Policy.AllowLinks, item.Policy.MaxDepth,
		item.Policy.MaxBodyLength, item.Policy.MaxAttachments, item.Policy.MaxImageBytes,
		item.Policy.EditWindowSeconds, item.UpdatedAt, item.ID)
	if err != nil {
		return mapError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r SpaceRepository) List(ctx context.Context, query repository.SpaceListQuery) ([]domain.Space, error) {
	if query.Limit < 1 {
		query.Limit = 20
	}
	var rows interface {
		Next() bool
		Scan(...any) error
		Err() error
		Close()
	}
	var err error
	if query.Status == nil {
		rows, err = runner(ctx, r.Pool).Query(ctx, `SELECT `+spaceColumns+`
FROM comment_space ORDER BY created_at, id LIMIT $1 OFFSET $2`, query.Limit, query.Offset)
	} else {
		rows, err = runner(ctx, r.Pool).Query(ctx, `SELECT `+spaceColumns+`
FROM comment_space WHERE status=$1 ORDER BY created_at, id LIMIT $2 OFFSET $3`, *query.Status, query.Limit, query.Offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Space, 0)
	for rows.Next() {
		item, err := scanSpace(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

var _ repository.SpaceRepository = (*SpaceRepository)(nil)
