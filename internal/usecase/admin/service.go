package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

type Service struct {
	Spaces      repository.SpaceRepository
	Threads     repository.ThreadRepository
	Comments    moderationCommentRepository
	Attachments moderationAttachmentRepository
	Outbox      repository.OutboxRepository
	Tx          repository.TransactionManager
	Now         func() time.Time
	NewID       func() uuid.UUID
}

type moderationCommentRepository interface {
	GetByIDForUpdate(context.Context, uuid.UUID) (domain.Comment, error)
	UpdateModerationStatus(context.Context, domain.Comment, domain.CommentStatus, int) error
}

type moderationAttachmentRepository interface {
	ListByComment(context.Context, uuid.UUID, uuid.UUID) ([]domain.Attachment, error)
}

type CreateSpaceInput struct {
	Key            string
	Name           string
	AccessMode     domain.AccessMode
	AllowedOrigins []string
	Policy         domain.Policy
}

type UpdateSpaceInput struct {
	ID             uuid.UUID
	Name           string
	Status         domain.SpaceStatus
	AccessMode     domain.AccessMode
	AllowedOrigins []string
	Policy         domain.Policy
}

type UpdateThreadInput struct {
	ID        uuid.UUID
	Status    domain.ThreadStatus
	Overrides domain.ThreadPolicyOverrides
}

type ThreadView struct {
	Thread domain.Thread
	Policy domain.Policy
}

type ModerationView struct {
	Comment     domain.Comment
	Attachments []domain.Attachment
}

func (s Service) CreateSpace(ctx context.Context, actor domain.Actor, in CreateSpaceInput) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	now := s.now()
	item := domain.Space{ID: s.newID(), Key: in.Key, Name: strings.TrimSpace(in.Name), Status: domain.SpaceStatusActive,
		AccessMode: in.AccessMode, AllowedOrigins: in.AllowedOrigins, Policy: in.Policy,
		CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := item.Validate(); err != nil {
		return domain.Space{}, err
	}
	if err := s.Spaces.Create(ctx, item); err != nil {
		return domain.Space{}, err
	}
	return item, nil
}

func (s Service) GetSpace(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Space, error) {
	if err := requireAdminRead(actor); err != nil {
		return domain.Space{}, err
	}
	item, err := s.Spaces.GetByID(ctx, id)
	return item, mapNotFound(err, domain.ErrSpaceNotFound)
}

func (s Service) ListSpaces(ctx context.Context, actor domain.Actor, query repository.SpaceListQuery) ([]domain.Space, error) {
	if err := requireAdminRead(actor); err != nil {
		return nil, err
	}
	if err := validatePage(query.Limit, query.Offset); err != nil {
		return nil, err
	}
	return s.Spaces.List(ctx, query)
}

func (s Service) UpdateSpace(ctx context.Context, actor domain.Actor, in UpdateSpaceInput) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	item, err := s.Spaces.GetByID(ctx, in.ID)
	if err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	item.Name, item.Status, item.AccessMode = strings.TrimSpace(in.Name), in.Status, in.AccessMode
	item.AllowedOrigins, item.Policy, item.UpdatedAt = in.AllowedOrigins, in.Policy, s.now()
	if err := item.Validate(); err != nil {
		return domain.Space{}, err
	}
	if err := s.Spaces.Update(ctx, item); err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	return item, nil
}

func (s Service) DisableSpace(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Space, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.Space{}, err
	}
	item, err := s.Spaces.GetByID(ctx, id)
	if err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	if item.Status == domain.SpaceStatusDisabled {
		return item, nil
	}
	item.Status, item.UpdatedAt = domain.SpaceStatusDisabled, s.now()
	if err := s.Spaces.Update(ctx, item); err != nil {
		return domain.Space{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	return item, nil
}

func (s Service) ListThreads(ctx context.Context, actor domain.Actor, query repository.ThreadListQuery) ([]ThreadView, error) {
	if err := requireAdminRead(actor); err != nil {
		return nil, err
	}
	if err := validatePage(query.Limit, query.Offset); err != nil {
		return nil, err
	}
	items, err := s.Threads.List(ctx, query)
	if err != nil {
		return nil, err
	}
	views := make([]ThreadView, 0, len(items))
	spaces := make(map[uuid.UUID]domain.Space)
	for _, item := range items {
		space, ok := spaces[item.SpaceID]
		if !ok {
			space, err = s.Spaces.GetByID(ctx, item.SpaceID)
			if err != nil {
				return nil, mapNotFound(err, domain.ErrSpaceNotFound)
			}
			spaces[item.SpaceID] = space
		}
		policy, err := domain.ApplyPolicy(space.Policy, item.PolicyOverrides)
		if err != nil {
			return nil, err
		}
		views = append(views, ThreadView{Thread: item, Policy: policy})
	}
	return views, nil
}

func (s Service) UpdateThread(ctx context.Context, actor domain.Actor, in UpdateThreadInput) (ThreadView, error) {
	if err := requireAdmin(actor); err != nil {
		return ThreadView{}, err
	}
	if !in.Status.Valid() {
		return ThreadView{}, fmt.Errorf("%w: unsupported thread status", domain.ErrValidation)
	}
	var result ThreadView
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.Threads.GetByID(txCtx, in.ID)
		if err != nil {
			return mapNotFound(err, domain.ErrThreadNotFound)
		}
		space, err := s.Spaces.GetByID(txCtx, item.SpaceID)
		if err != nil {
			return mapNotFound(err, domain.ErrSpaceNotFound)
		}
		policy, err := domain.ApplyPolicy(space.Policy, in.Overrides)
		if err != nil {
			return err
		}
		now := s.now()
		sequence, err := s.Threads.NextSequenceAnyState(txCtx, item.ID)
		if err != nil {
			return mapNotFound(err, domain.ErrThreadNotFound)
		}
		item.Status, item.PolicyOverrides, item.UpdatedAt, item.LastSequence = in.Status, in.Overrides, now, sequence
		if err := s.Threads.Update(txCtx, item); err != nil {
			return mapNotFound(err, domain.ErrThreadNotFound)
		}
		eventID := s.newID()
		payload, err := json.Marshal(map[string]any{
			"schema_version": 1, "event_id": eventID, "occurred_at": now,
			"thread_id": item.ID, "sequence": sequence, "actor_id": actor.UserID,
			"status": item.Status, "policy": policyPayload(policy),
		})
		if err != nil {
			return err
		}
		event := domain.OutboxEvent{ID: eventID, AggregateType: "thread", AggregateID: item.ID,
			Subject: domain.EventCommentThreadUpdated, SchemaVersion: 1, Payload: payload, NextAttemptAt: now, CreatedAt: now}
		if err := event.Validate(); err != nil {
			return err
		}
		if err := s.Outbox.Add(txCtx, event); err != nil {
			return err
		}
		result = ThreadView{Thread: item, Policy: policy}
		return nil
	})
	return result, err
}

func (s Service) HideComment(ctx context.Context, actor domain.Actor, id uuid.UUID) (ModerationView, error) {
	return s.moderateComment(ctx, actor, id, domain.CommentStatusHidden, domain.EventCommentHidden)
}

func (s Service) RestoreComment(ctx context.Context, actor domain.Actor, id uuid.UUID) (ModerationView, error) {
	return s.moderateComment(ctx, actor, id, domain.CommentStatusActive, domain.EventCommentRestored)
}

func (s Service) moderateComment(
	ctx context.Context,
	actor domain.Actor,
	id uuid.UUID,
	target domain.CommentStatus,
	subject domain.EventSubject,
) (ModerationView, error) {
	if err := requireAdminRead(actor); err != nil {
		return ModerationView{}, err
	}
	if id == uuid.Nil || (target != domain.CommentStatusHidden && target != domain.CommentStatusActive) {
		return ModerationView{}, fmt.Errorf("%w: invalid moderation target", domain.ErrValidation)
	}

	var result ModerationView
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.Comments.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return mapNotFound(err, domain.ErrCommentNotFound)
		}
		thread, err := s.Threads.GetByID(txCtx, item.ThreadID)
		if err != nil {
			return mapNotFound(err, domain.ErrThreadNotFound)
		}
		if _, err := s.Spaces.GetByID(txCtx, thread.SpaceID); err != nil {
			return mapNotFound(err, domain.ErrSpaceNotFound)
		}
		if item.Status == domain.CommentStatusDeleted {
			return domain.ErrModerationConflict
		}
		attachments, err := s.Attachments.ListByComment(txCtx, item.ThreadID, item.ID)
		if err != nil {
			return err
		}
		if item.Status == target {
			result = ModerationView{Comment: item, Attachments: attachments}
			return nil
		}
		expectedStatus, expectedVersion := item.Status, item.Version
		sequence, err := s.Threads.NextSequenceAnyState(txCtx, item.ThreadID)
		if err != nil {
			return mapNotFound(err, domain.ErrThreadNotFound)
		}
		now := s.now()
		item.Status, item.Version, item.Sequence, item.UpdatedAt = target, item.Version+1, sequence, now
		if err := s.Comments.UpdateModerationStatus(txCtx, item, expectedStatus, expectedVersion); err != nil {
			return err
		}
		if err := s.addModerationOutbox(txCtx, item, attachments, subject, actor.UserID, now); err != nil {
			return err
		}
		result = ModerationView{Comment: item, Attachments: attachments}
		return nil
	})
	return result, err
}

func (s Service) addModerationOutbox(
	ctx context.Context,
	item domain.Comment,
	attachments []domain.Attachment,
	subject domain.EventSubject,
	actorID uuid.UUID,
	now time.Time,
) error {
	eventID := s.newID()
	payload := map[string]any{
		"schema_version": 1, "event_id": eventID, "occurred_at": now,
		"thread_id": item.ThreadID, "sequence": item.Sequence, "comment_id": item.ID,
		"parent_id": item.ParentID, "root_id": item.RootID, "actor_id": actorID,
		"author_id": item.AuthorID, "status": item.Status, "version": item.Version,
		"direct_replies_count": item.DirectRepliesCount,
	}
	if subject == domain.EventCommentRestored {
		payload["body"], payload["links"] = item.Body, item.Links
		payload["attachments"] = attachmentPayload(attachments)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	event := domain.OutboxEvent{
		ID: eventID, AggregateType: "comment", AggregateID: item.ID,
		Subject: subject, SchemaVersion: 1, Payload: encoded,
		NextAttemptAt: now, CreatedAt: now,
	}
	if err := event.Validate(); err != nil {
		return err
	}
	return s.Outbox.Add(ctx, event)
}

func attachmentPayload(items []domain.Attachment) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"id": item.ID, "status": item.Status, "mime_type": item.MIMEType,
			"size_bytes": item.SizeBytes, "width": item.Width, "height": item.Height,
		})
	}
	return result
}

func policyPayload(policy domain.Policy) map[string]any {
	return map[string]any{
		"allow_images": policy.AllowImages, "allow_links": policy.AllowLinks,
		"max_depth": policy.MaxDepth, "max_body_length": policy.MaxBodyLength,
		"max_attachments": policy.MaxAttachments, "max_image_bytes": policy.MaxImageBytes,
		"edit_window_seconds": policy.EditWindowSeconds,
	}
}

func requireAdminRead(actor domain.Actor) error {
	if err := actor.Validate(); err != nil {
		return err
	}
	role := strings.ToUpper(strings.TrimSpace(actor.Role))
	if role != "ADMIN" && role != "MODERATOR" {
		return domain.ErrForbidden
	}
	return nil
}

func requireAdmin(actor domain.Actor) error {
	if err := requireAdminRead(actor); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "ADMIN") {
		return domain.ErrForbidden
	}
	return nil
}

func validatePage(limit, offset int) error {
	if limit < 1 || limit > MaxListLimit || offset < 0 {
		return fmt.Errorf("%w: limit must be between 1 and %d and offset non-negative", domain.ErrValidation, MaxListLimit)
	}
	return nil
}

func mapNotFound(err, target error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return target
	}
	return err
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID() uuid.UUID {
	if s.NewID != nil {
		return s.NewID()
	}
	return uuid.New()
}
