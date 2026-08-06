package comment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	"github.com/google/uuid"
)

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

type Service struct {
	Spaces                repository.SpaceRepository
	Threads               repository.ThreadRepository
	Comments              repository.CommentRepository
	Attachments           repository.AttachmentRepository
	Outbox                repository.OutboxRepository
	Tx                    repository.TransactionManager
	Now                   func() time.Time
	NewID                 func() uuid.UUID
	Files                 FileStorage
	AttachmentTTLMinutes  int
	SignedURLMinutes      int
	ActivationMaxAttempts int
	Access                AccessAuthorizer
}

type AccessAuthorizer interface {
	Permissions(context.Context, domain.Actor, domain.Space, domain.ResourceReference) (domain.AccessPermission, error)
}

type EnsureThreadInput struct {
	SpaceKey string
	Resource domain.ResourceReference
}

type CreateCommentInput struct {
	ThreadID       uuid.UUID
	ParentID       *uuid.UUID
	Body           string
	AttachmentIDs  []uuid.UUID
	IdempotencyKey uuid.UUID
}

type UpdateCommentInput struct {
	CommentID       uuid.UUID
	Body            string
	ExpectedVersion int
}

type DeleteCommentInput struct {
	CommentID       uuid.UUID
	ExpectedVersion int
}

type CommentView struct {
	Comment     domain.Comment
	Attachments []domain.Attachment
}

type ThreadView struct {
	Thread domain.Thread
	Policy domain.Policy
}

type CreateCommentResult struct {
	View    CommentView
	Created bool
}

func (s Service) EnsureThread(ctx context.Context, actor domain.Actor, in EnsureThreadInput) (ThreadView, error) {
	if err := actor.Validate(); err != nil {
		return ThreadView{}, err
	}
	if err := in.Resource.Validate(); err != nil {
		return ThreadView{}, err
	}
	if err := domain.ValidateSpaceKey(in.SpaceKey); err != nil {
		return ThreadView{}, err
	}
	space, err := s.Spaces.GetByKey(ctx, in.SpaceKey)
	if err != nil {
		return ThreadView{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	if err := s.authorizeAccess(ctx, actor, space, in.Resource, domain.AccessPermissionRead); err != nil {
		return ThreadView{}, err
	}
	now := s.now()
	thread := domain.Thread{
		ID: uuidOrNew(s.NewID), SpaceID: space.ID, Resource: in.Resource,
		Status: domain.ThreadStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.Threads.Ensure(ctx, thread)
	if err != nil {
		return ThreadView{}, err
	}
	policy, err := domain.ApplyPolicy(space.Policy, created.PolicyOverrides)
	if err != nil {
		return ThreadView{}, err
	}
	return ThreadView{Thread: created, Policy: policy}, nil
}

func (s Service) GetThread(ctx context.Context, actor domain.Actor, threadID uuid.UUID) (ThreadView, error) {
	thread, _, policy, err := s.loadThreadContext(ctx, actor, threadID, domain.AccessPermissionRead)
	return ThreadView{Thread: thread, Policy: policy}, err
}

func (s Service) ListComments(ctx context.Context, actor domain.Actor, query repository.CommentListQuery) ([]CommentView, error) {
	if query.Limit < 1 || query.Limit > MaxPageLimit+1 {
		return nil, fmt.Errorf("%w: limit must be between 1 and %d", domain.ErrValidation, MaxPageLimit)
	}
	thread, _, _, err := s.loadThreadContext(ctx, actor, query.ThreadID, domain.AccessPermissionRead)
	if err != nil {
		return nil, err
	}
	if query.ParentID != nil {
		parent, err := s.Comments.GetByID(ctx, *query.ParentID)
		if err != nil || parent.ThreadID != thread.ID || parent.Status == domain.CommentStatusHidden {
			return nil, domain.ErrParentNotFound
		}
	}
	comments, err := s.Comments.List(ctx, query)
	if err != nil {
		return nil, err
	}
	return s.views(ctx, comments)
}

func (s Service) GetComment(ctx context.Context, actor domain.Actor, commentID uuid.UUID) (CommentView, error) {
	item, err := s.Comments.GetByID(ctx, commentID)
	if err != nil {
		return CommentView{}, mapNotFound(err, domain.ErrCommentNotFound)
	}
	if item.Status == domain.CommentStatusHidden {
		return CommentView{}, domain.ErrCommentNotFound
	}
	if _, _, _, err := s.loadThreadContext(ctx, actor, item.ThreadID, domain.AccessPermissionRead); err != nil {
		return CommentView{}, err
	}
	attachments, err := s.Attachments.ListByComment(ctx, item.ThreadID, item.ID)
	if err != nil {
		return CommentView{}, err
	}
	return CommentView{Comment: item, Attachments: attachments}, nil
}

func (s Service) ListChanges(ctx context.Context, actor domain.Actor, query repository.CommentChangeQuery) ([]CommentView, error) {
	if query.AfterSequence < 0 || query.Limit < 1 || query.Limit > MaxPageLimit+1 {
		return nil, fmt.Errorf("%w: invalid changes cursor or limit", domain.ErrValidation)
	}
	if _, _, _, err := s.loadThreadContext(ctx, actor, query.ThreadID, domain.AccessPermissionRead); err != nil {
		return nil, err
	}
	comments, err := s.Comments.ListChanges(ctx, query)
	if err != nil {
		return nil, err
	}
	return s.changeViews(ctx, comments)
}

func (s Service) CreateComment(ctx context.Context, actor domain.Actor, in CreateCommentInput) (CreateCommentResult, error) {
	if err := actor.Validate(); err != nil {
		return CreateCommentResult{}, err
	}
	if in.ThreadID == uuid.Nil || in.IdempotencyKey == uuid.Nil {
		return CreateCommentResult{}, fmt.Errorf("%w: thread and idempotency identifiers are required", domain.ErrValidation)
	}

	var result CreateCommentResult
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.Comments.LockIdempotencyKey(txCtx, actor.UserID, in.IdempotencyKey); err != nil {
			return err
		}
		existing, err := s.Comments.GetByIdempotencyKey(txCtx, actor.UserID, in.IdempotencyKey)
		if err == nil {
			return s.resolveReplay(txCtx, actor, existing, in, &result)
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		thread, _, policy, err := s.loadThreadContext(txCtx, actor, in.ThreadID, domain.AccessPermissionWrite)
		if err != nil {
			return err
		}
		attachments, err := s.validateAttachments(txCtx, actor, thread.ID, in.AttachmentIDs, policy)
		if err != nil {
			return err
		}
		content := domain.AnalyzeCommentContent(in.Body, len(attachments))
		if err := content.Validate(policy); err != nil {
			return err
		}

		var parent *domain.Comment
		if in.ParentID != nil {
			loaded, err := s.Comments.GetByIDForUpdate(txCtx, *in.ParentID)
			if err != nil || loaded.ThreadID != thread.ID || loaded.Status != domain.CommentStatusActive {
				return domain.ErrParentNotFound
			}
			parent = &loaded
		}
		commentID := uuidOrNew(s.NewID)
		placement, err := domain.BuildCommentPlacement(commentID, thread.ID, parent, policy.MaxDepth)
		if err != nil {
			return err
		}
		sequence, err := s.Threads.NextSequence(txCtx, thread.ID)
		if err != nil {
			return err
		}
		now := s.now()
		item := domain.Comment{
			ID: commentID, ThreadID: thread.ID, AuthorID: actor.UserID,
			ParentID: placement.ParentID, RootID: placement.RootID, Path: placement.Path, Depth: placement.Depth,
			Body: content.Body, Links: content.Links, Status: domain.CommentStatusActive,
			Version: 1, Sequence: sequence, IdempotencyKey: in.IdempotencyKey,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.Comments.Create(txCtx, item); err != nil {
			return err
		}
		for index := range attachments {
			if err := s.Attachments.BindToComment(txCtx, attachments[index].ID, thread.ID, item.ID, actor.UserID); err != nil {
				return err
			}
			attachments[index].CommentID = &item.ID
			attachments[index].Status = domain.AttachmentStatusProcessing
			attachments[index].UpdatedAt = now
		}
		if err := s.Threads.RecordCommentCreated(txCtx, thread.ID, parent == nil); err != nil {
			return err
		}
		if parent != nil {
			if err := s.Comments.IncrementReplyCount(txCtx, thread.ID, parent.ID); err != nil {
				return err
			}
		}
		if err := s.addOutbox(txCtx, item, attachments, domain.EventCommentCreated, actor.UserID, now); err != nil {
			return err
		}
		result = CreateCommentResult{View: CommentView{Comment: item, Attachments: attachments}, Created: true}
		return nil
	})
	return result, err
}

func (s Service) UpdateComment(ctx context.Context, actor domain.Actor, in UpdateCommentInput) (CommentView, error) {
	var result CommentView
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, thread, policy, err := s.loadMutableComment(txCtx, actor, in.CommentID, in.ExpectedVersion)
		if err != nil {
			return err
		}
		attachments, err := s.Attachments.ListByComment(txCtx, thread.ID, item.ID)
		if err != nil {
			return err
		}
		content := domain.AnalyzeCommentContent(in.Body, 0)
		content.ExistingAttachmentCount = len(attachments)
		if err := content.Validate(policy); err != nil {
			return err
		}
		sequence, err := s.Threads.NextSequence(txCtx, thread.ID)
		if err != nil {
			return err
		}
		now := s.now()
		item.Body, item.Links, item.Sequence = content.Body, content.Links, sequence
		item.Version++
		item.EditedAt, item.UpdatedAt = &now, now
		if err := s.Comments.UpdateContent(txCtx, item, in.ExpectedVersion); err != nil {
			return err
		}
		if err := s.addOutbox(txCtx, item, attachments, domain.EventCommentUpdated, actor.UserID, now); err != nil {
			return err
		}
		result = CommentView{Comment: item, Attachments: attachments}
		return nil
	})
	return result, err
}

func (s Service) DeleteComment(ctx context.Context, actor domain.Actor, in DeleteCommentInput) (CommentView, error) {
	var result CommentView
	err := s.Tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, thread, _, err := s.loadMutableComment(txCtx, actor, in.CommentID, in.ExpectedVersion)
		if err != nil {
			return err
		}
		sequence, err := s.Threads.NextSequence(txCtx, thread.ID)
		if err != nil {
			return err
		}
		now := s.now()
		item.Body, item.Links, item.Sequence = "", nil, sequence
		item.Status, item.Version = domain.CommentStatusDeleted, item.Version+1
		item.DeletedAt, item.UpdatedAt = &now, now
		if err := s.Comments.MarkDeleted(txCtx, item, in.ExpectedVersion); err != nil {
			return err
		}
		attachments, err := s.Attachments.ListByComment(txCtx, thread.ID, item.ID)
		if err != nil {
			return err
		}
		for index := range attachments {
			attachments[index].Status = domain.AttachmentStatusDeleted
			attachments[index].DeletedAt = &now
			attachments[index].DeleteNextAttemptAt = &now
			attachments[index].UpdatedAt = now
			if err := s.Attachments.UpdateStatus(txCtx, attachments[index]); err != nil {
				return err
			}
		}
		if err := s.addOutbox(txCtx, item, nil, domain.EventCommentDeleted, actor.UserID, now); err != nil {
			return err
		}
		result = CommentView{Comment: item, Attachments: attachments}
		return nil
	})
	return result, err
}

func (s Service) loadMutableComment(ctx context.Context, actor domain.Actor, commentID uuid.UUID, expectedVersion int) (domain.Comment, domain.Thread, domain.Policy, error) {
	if err := actor.Validate(); err != nil {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, err
	}
	if commentID == uuid.Nil || expectedVersion < 1 {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, fmt.Errorf("%w: comment id and positive version are required", domain.ErrValidation)
	}
	item, err := s.Comments.GetByIDForUpdate(ctx, commentID)
	if err != nil {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, mapNotFound(err, domain.ErrCommentNotFound)
	}
	if item.Status != domain.CommentStatusActive {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, domain.ErrCommentNotFound
	}
	thread, _, policy, err := s.loadThreadContext(ctx, actor, item.ThreadID, domain.AccessPermissionWrite)
	if err != nil {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, err
	}
	if item.AuthorID != actor.UserID {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, domain.ErrForbidden
	}
	if item.Version != expectedVersion {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, domain.ErrEditConflict
	}
	now := s.now()
	if policy.EditWindowSeconds == 0 || now.After(item.CreatedAt.Add(time.Duration(policy.EditWindowSeconds)*time.Second)) {
		return domain.Comment{}, domain.Thread{}, domain.Policy{}, domain.ErrForbidden
	}
	return item, thread, policy, nil
}

func (s Service) loadThreadContext(
	ctx context.Context,
	actor domain.Actor,
	threadID uuid.UUID,
	required domain.AccessPermission,
) (domain.Thread, domain.Space, domain.Policy, error) {
	if err := actor.Validate(); err != nil {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, err
	}
	thread, err := s.Threads.GetByID(ctx, threadID)
	if err != nil {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, mapNotFound(err, domain.ErrThreadNotFound)
	}
	space, err := s.Spaces.GetByID(ctx, thread.SpaceID)
	if err != nil {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, mapNotFound(err, domain.ErrSpaceNotFound)
	}
	if err := s.authorizeAccess(ctx, actor, space, thread.Resource, required); err != nil {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, err
	}
	if thread.Status == domain.ThreadStatusHidden {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, domain.ErrThreadNotFound
	}
	if required != domain.AccessPermissionRead && thread.Status != domain.ThreadStatusOpen {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, domain.ErrThreadNotWritable
	}
	policy, err := domain.ApplyPolicy(space.Policy, thread.PolicyOverrides)
	if err != nil {
		return domain.Thread{}, domain.Space{}, domain.Policy{}, err
	}
	return thread, space, policy, nil
}

func (s Service) validateAttachments(ctx context.Context, actor domain.Actor, threadID uuid.UUID, ids []uuid.UUID, policy domain.Policy) ([]domain.Attachment, error) {
	if len(ids) > int(policy.MaxAttachments) {
		return nil, fmt.Errorf("%w: attachment count is outside policy", domain.ErrInvalidCommentContent)
	}
	if len(ids) > 0 && !policy.AllowImages {
		return nil, domain.ErrImagesDisabled
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]domain.Attachment, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, fmt.Errorf("%w: attachment id is required", domain.ErrInvalidCommentContent)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("%w: duplicate attachment id", domain.ErrInvalidCommentContent)
		}
		seen[id] = struct{}{}
		attachment, err := s.Attachments.GetByID(ctx, id)
		if err != nil || attachment.ThreadID != threadID || attachment.UploaderID != actor.UserID ||
			attachment.CommentID != nil || attachment.Status != domain.AttachmentStatusPending || !attachment.ExpiresAt.After(s.now()) {
			return nil, fmt.Errorf("%w: attachment is not bindable", domain.ErrInvalidCommentContent)
		}
		if err := attachment.Validate(policy); err != nil {
			return nil, err
		}
		result = append(result, attachment)
	}
	return result, nil
}

func (s Service) resolveReplay(ctx context.Context, actor domain.Actor, existing domain.Comment, in CreateCommentInput, result *CreateCommentResult) error {
	attachments, err := s.Attachments.ListByComment(ctx, existing.ThreadID, existing.ID)
	if err != nil {
		return err
	}
	existingIDs := make([]uuid.UUID, 0, len(attachments))
	for _, item := range attachments {
		existingIDs = append(existingIDs, item.ID)
	}
	if existing.ThreadID != in.ThreadID || !equalUUIDPtr(existing.ParentID, in.ParentID) || existing.Body != in.Body || !equalUUIDSet(existingIDs, in.AttachmentIDs) {
		return domain.ErrIdempotencyConflict
	}
	thread, err := s.Threads.GetByID(ctx, existing.ThreadID)
	if err != nil {
		return mapNotFound(err, domain.ErrThreadNotFound)
	}
	space, err := s.Spaces.GetByID(ctx, thread.SpaceID)
	if err != nil {
		return mapNotFound(err, domain.ErrSpaceNotFound)
	}
	if err := s.authorizeAccess(ctx, actor, space, thread.Resource, domain.AccessPermissionWrite); err != nil {
		return err
	}
	if thread.Status == domain.ThreadStatusHidden {
		return domain.ErrThreadNotFound
	}
	*result = CreateCommentResult{View: CommentView{Comment: existing, Attachments: attachments}}
	return nil
}

func (s Service) views(ctx context.Context, comments []domain.Comment) ([]CommentView, error) {
	result := make([]CommentView, 0, len(comments))
	for _, item := range comments {
		if item.Status == domain.CommentStatusHidden {
			continue
		}
		attachments, err := s.Attachments.ListByComment(ctx, item.ThreadID, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, CommentView{Comment: item, Attachments: attachments})
	}
	return result, nil
}

func (s Service) changeViews(ctx context.Context, comments []domain.Comment) ([]CommentView, error) {
	result := make([]CommentView, 0, len(comments))
	for _, item := range comments {
		if item.Status == domain.CommentStatusHidden {
			item.Body = ""
			item.Links = []domain.Link{}
			result = append(result, CommentView{Comment: item, Attachments: []domain.Attachment{}})
			continue
		}
		attachments, err := s.Attachments.ListByComment(ctx, item.ThreadID, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, CommentView{Comment: item, Attachments: attachments})
	}
	return result, nil
}

func (s Service) addOutbox(ctx context.Context, item domain.Comment, attachments []domain.Attachment, subject domain.EventSubject, actorID uuid.UUID, now time.Time) error {
	eventID := uuidOrNew(s.NewID)
	attachmentPayload := make([]map[string]any, 0, len(attachments))
	for _, attachment := range attachments {
		attachmentPayload = append(attachmentPayload, map[string]any{
			"id": attachment.ID, "status": attachment.Status, "mime_type": attachment.MIMEType,
			"size_bytes": attachment.SizeBytes, "width": attachment.Width, "height": attachment.Height,
		})
	}
	payload, err := json.Marshal(map[string]any{
		"schema_version":       1,
		"event_id":             eventID,
		"occurred_at":          now,
		"thread_id":            item.ThreadID,
		"sequence":             item.Sequence,
		"comment_id":           item.ID,
		"parent_id":            item.ParentID,
		"root_id":              item.RootID,
		"actor_id":             actorID,
		"author_id":            item.AuthorID,
		"status":               item.Status,
		"version":              item.Version,
		"body":                 item.Body,
		"links":                item.Links,
		"attachments":          attachmentPayload,
		"direct_replies_count": item.DirectRepliesCount,
	})
	if err != nil {
		return err
	}
	event := domain.OutboxEvent{
		ID: eventID, AggregateType: "comment", AggregateID: item.ID,
		Subject: subject, SchemaVersion: 1, Payload: payload,
		NextAttemptAt: now, CreatedAt: now,
	}
	if err := event.Validate(); err != nil {
		return err
	}
	return s.Outbox.Add(ctx, event)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func uuidOrNew(generator func() uuid.UUID) uuid.UUID {
	if generator != nil {
		return generator()
	}
	return uuid.New()
}

func (s Service) authorizeAccess(
	ctx context.Context,
	actor domain.Actor,
	space domain.Space,
	resource domain.ResourceReference,
	required domain.AccessPermission,
) error {
	if space.Status != domain.SpaceStatusActive {
		return domain.ErrSpaceNotFound
	}
	if space.AccessMode == domain.AccessModeAuthenticated {
		return nil
	}
	if space.AccessMode != domain.AccessModeContextGrant || s.Access == nil {
		return domain.ErrAccessRequired
	}
	permissions, err := s.Access.Permissions(ctx, actor, space, resource)
	if err != nil {
		return err
	}
	if !permissions.Includes(required) {
		return domain.ErrForbidden
	}
	return nil
}

func mapNotFound(err, target error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return target
	}
	return err
}

func equalUUIDPtr(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalUUIDSet(left, right []uuid.UUID) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[uuid.UUID]int, len(left))
	for _, id := range left {
		counts[id]++
	}
	for _, id := range right {
		counts[id]--
		if counts[id] < 0 {
			return false
		}
	}
	return true
}
