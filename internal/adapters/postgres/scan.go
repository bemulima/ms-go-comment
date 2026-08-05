package postgres

import (
	"encoding/json"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/jackc/pgx/v5"
)

type scanner interface {
	Scan(dest ...any) error
}

const spaceColumns = `id, key, name, status, access_mode, allowed_origins,
allow_images, allow_links, max_depth, max_body_length, max_attachments,
max_image_bytes, edit_window_seconds, created_by, created_at, updated_at`

func scanSpace(row scanner) (domain.Space, error) {
	var item domain.Space
	err := row.Scan(
		&item.ID, &item.Key, &item.Name, &item.Status, &item.AccessMode, &item.AllowedOrigins,
		&item.Policy.AllowImages, &item.Policy.AllowLinks, &item.Policy.MaxDepth,
		&item.Policy.MaxBodyLength, &item.Policy.MaxAttachments, &item.Policy.MaxImageBytes,
		&item.Policy.EditWindowSeconds, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, mapError(err)
}

const threadColumns = `id, space_id, resource_type, resource_id, status,
allow_images, allow_links, max_depth, max_body_length, max_attachments,
max_image_bytes, edit_window_seconds, comment_count, root_comment_count,
last_sequence, last_comment_at, created_at, updated_at`

func scanThread(row scanner) (domain.Thread, error) {
	var item domain.Thread
	err := row.Scan(
		&item.ID, &item.SpaceID, &item.Resource.Type, &item.Resource.ID, &item.Status,
		&item.PolicyOverrides.AllowImages, &item.PolicyOverrides.AllowLinks,
		&item.PolicyOverrides.MaxDepth, &item.PolicyOverrides.MaxBodyLength,
		&item.PolicyOverrides.MaxAttachments, &item.PolicyOverrides.MaxImageBytes,
		&item.PolicyOverrides.EditWindowSeconds, &item.CommentCount, &item.RootCommentCount,
		&item.LastSequence, &item.LastCommentAt, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, mapError(err)
}

const commentColumns = `id, thread_id, author_id, parent_id, root_id, path,
depth, body, links, status, version, sequence, direct_replies_count,
idempotency_key, edited_at, deleted_at, created_at, updated_at`

func scanComment(row scanner) (domain.Comment, error) {
	var item domain.Comment
	var links []byte
	err := row.Scan(
		&item.ID, &item.ThreadID, &item.AuthorID, &item.ParentID, &item.RootID, &item.Path,
		&item.Depth, &item.Body, &links, &item.Status, &item.Version, &item.Sequence,
		&item.DirectRepliesCount, &item.IdempotencyKey, &item.EditedAt, &item.DeletedAt,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return domain.Comment{}, mapError(err)
	}
	if err := json.Unmarshal(links, &item.Links); err != nil {
		return domain.Comment{}, err
	}
	return item, nil
}

func scanComments(rows pgx.Rows) ([]domain.Comment, error) {
	items := make([]domain.Comment, 0)
	for rows.Next() {
		item, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const attachmentColumns = `id, thread_id, comment_id, uploader_id,
filestorage_id, status, mime_type, size_bytes, width, height,
original_filename, expires_at, activated_at, deleted_at, created_at, updated_at`

func scanAttachment(row scanner) (domain.Attachment, error) {
	var item domain.Attachment
	err := row.Scan(
		&item.ID, &item.ThreadID, &item.CommentID, &item.UploaderID, &item.FileStorageID,
		&item.Status, &item.MIMEType, &item.SizeBytes, &item.Width, &item.Height,
		&item.OriginalFilename, &item.ExpiresAt, &item.ActivatedAt, &item.DeletedAt,
		&item.CreatedAt, &item.UpdatedAt,
	)
	return item, mapError(err)
}

const outboxColumns = `id, aggregate_type, aggregate_id, subject, schema_version,
payload, attempts, next_attempt_at, published_at, COALESCE(last_error, ''), created_at`

func scanOutbox(row scanner) (domain.OutboxEvent, error) {
	var item domain.OutboxEvent
	err := row.Scan(
		&item.ID, &item.AggregateType, &item.AggregateID, &item.Subject, &item.SchemaVersion,
		&item.Payload, &item.Attempts, &item.NextAttemptAt, &item.PublishedAt, &item.LastError,
		&item.CreatedAt,
	)
	return item, mapError(err)
}
