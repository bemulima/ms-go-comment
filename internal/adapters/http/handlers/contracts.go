package handlers

import (
	"context"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/bemulima/ms-go-comment/internal/domain/repository"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/google/uuid"
)

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
