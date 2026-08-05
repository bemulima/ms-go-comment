package handlers

import (
	"context"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	adminuc "github.com/bemulima/ms-go-comment/internal/usecase/admin"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/google/uuid"
)

type AdminService interface {
	CreateSpace(context.Context, domain.Actor, adminuc.CreateSpaceInput) (domain.Space, error)
	GetSpace(context.Context, domain.Actor, uuid.UUID) (domain.Space, error)
	ListSpaces(context.Context, domain.Actor, repository.SpaceListQuery) ([]domain.Space, error)
	UpdateSpace(context.Context, domain.Actor, adminuc.UpdateSpaceInput) (domain.Space, error)
	DisableSpace(context.Context, domain.Actor, uuid.UUID) (domain.Space, error)
	ListThreads(context.Context, domain.Actor, repository.ThreadListQuery) ([]adminuc.ThreadView, error)
	UpdateThread(context.Context, domain.Actor, adminuc.UpdateThreadInput) (adminuc.ThreadView, error)
}

type CommentService interface {
	EnsureThread(context.Context, domain.Actor, commentuc.EnsureThreadInput) (commentuc.ThreadView, error)
	GetThread(context.Context, domain.Actor, uuid.UUID) (commentuc.ThreadView, error)
	ListComments(context.Context, domain.Actor, repository.CommentListQuery) ([]commentuc.CommentView, error)
	GetComment(context.Context, domain.Actor, uuid.UUID) (commentuc.CommentView, error)
	ListChanges(context.Context, domain.Actor, repository.CommentChangeQuery) ([]commentuc.CommentView, error)
	CreateComment(context.Context, domain.Actor, commentuc.CreateCommentInput) (commentuc.CreateCommentResult, error)
	UpdateComment(context.Context, domain.Actor, commentuc.UpdateCommentInput) (commentuc.CommentView, error)
	DeleteComment(context.Context, domain.Actor, commentuc.DeleteCommentInput) (commentuc.CommentView, error)
	UploadAttachment(context.Context, domain.Actor, commentuc.UploadAttachmentInput) (domain.Attachment, error)
	GetAttachmentSignedURL(context.Context, domain.Actor, uuid.UUID) (commentuc.SignedFileURL, error)
	DeleteAttachment(context.Context, domain.Actor, uuid.UUID) (domain.Attachment, error)
}
