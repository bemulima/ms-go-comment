package comment_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/google/uuid"
)

func TestService_UploadAttachmentValidatesAndStagesTemporaryFile(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	space := store.spaces[thread.SpaceID]
	space.Policy.AllowImages = true
	store.spaces[space.ID] = space
	attachmentID := uuid.New()
	fileID := uuid.New()
	files := &fakeFileStorage{storedID: fileID}
	service.Files = files
	service.NewID = idSequence(attachmentID)

	attachment, err := service.UploadAttachment(context.Background(), actor, commentuc.UploadAttachmentInput{
		ThreadID: thread.ID, Filename: "../avatar.jpg", Data: testPNG(t),
	})
	if err != nil {
		t.Fatalf("UploadAttachment() error = %v", err)
	}
	if attachment.ID != attachmentID || attachment.FileStorageID != fileID || attachment.MIMEType != "image/png" || attachment.Status != domain.AttachmentStatusPending {
		t.Fatalf("unexpected attachment: %#v", attachment)
	}
	if files.upload.OwnerID != attachmentID || files.upload.Filename != "avatar.jpg" || files.upload.TTLMinutes != 60 {
		t.Fatalf("unexpected FileStorage upload: %#v", files.upload)
	}
	if _, ok := store.attachments[attachmentID]; !ok {
		t.Fatal("attachment metadata was not persisted")
	}
}

func TestService_GetAttachmentSignedURLRequiresReadyVisibleComment(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	files := &fakeFileStorage{signedURL: "https://signed.example/image"}
	service.Files = files
	commentID := uuid.New()
	store.comments[commentID] = activeComment(commentID, thread.ID, actor.UserID, service.Now())
	attachmentID := uuid.New()
	fileID := uuid.New()
	store.attachments[attachmentID] = boundAttachment(attachmentID, fileID, thread.ID, commentID, actor.UserID, domain.AttachmentStatusReady, service.Now())

	result, err := service.GetAttachmentSignedURL(context.Background(), actor, attachmentID)
	if err != nil || result.URL != files.signedURL || files.signedFileID != fileID {
		t.Fatalf("signed result=%#v error=%v", result, err)
	}
	attachment := store.attachments[attachmentID]
	attachment.Status = domain.AttachmentStatusProcessing
	store.attachments[attachmentID] = attachment
	if _, err := service.GetAttachmentSignedURL(context.Background(), actor, attachmentID); !errors.Is(err, domain.ErrAttachmentNotReady) {
		t.Fatalf("processing attachment error = %v", err)
	}
}

func TestService_ProcessAttachmentWorkActivatesAndPublishes(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	files := &fakeFileStorage{}
	service.Files = files
	commentID := uuid.New()
	store.comments[commentID] = activeComment(commentID, thread.ID, actor.UserID, service.Now())
	attachmentID := uuid.New()
	fileID := uuid.New()
	store.attachments[attachmentID] = boundAttachment(attachmentID, fileID, thread.ID, commentID, actor.UserID, domain.AttachmentStatusProcessing, service.Now())
	service.NewID = idSequence(uuid.New())

	result, err := service.ProcessAttachmentWork(context.Background(), 10)
	if err != nil {
		t.Fatalf("ProcessAttachmentWork() error = %v", err)
	}
	if result.Activated != 1 || store.attachments[attachmentID].Status != domain.AttachmentStatusReady || files.activatedFileID != fileID {
		t.Fatalf("result=%#v attachment=%#v", result, store.attachments[attachmentID])
	}
	if store.comments[commentID].Sequence != 1 || store.comments[commentID].Version != 2 || len(store.outbox) != 1 || store.outbox[0].Subject != domain.EventCommentAttachmentReady {
		t.Fatalf("comment=%#v outbox=%#v", store.comments[commentID], store.outbox)
	}

	result, err = service.ProcessAttachmentWork(context.Background(), 10)
	if err != nil || result.Activated != 0 || len(store.outbox) != 1 {
		t.Fatalf("idempotent result=%#v error=%v outbox=%d", result, err, len(store.outbox))
	}
}

func TestService_ProcessAttachmentWorkMarksTerminalFailure(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	service.Files = &fakeFileStorage{activateErr: errors.New("unavailable")}
	service.ActivationMaxAttempts = 1
	commentID := uuid.New()
	store.comments[commentID] = activeComment(commentID, thread.ID, actor.UserID, service.Now())
	attachmentID := uuid.New()
	store.attachments[attachmentID] = boundAttachment(attachmentID, uuid.New(), thread.ID, commentID, actor.UserID, domain.AttachmentStatusProcessing, service.Now())
	service.NewID = idSequence(uuid.New())

	result, err := service.ProcessAttachmentWork(context.Background(), 10)
	if err != nil {
		t.Fatalf("ProcessAttachmentWork() error = %v", err)
	}
	if result.Failed != 1 || store.attachments[attachmentID].Status != domain.AttachmentStatusFailed ||
		len(store.outbox) != 1 || store.outbox[0].Subject != domain.EventCommentAttachmentFailed {
		t.Fatalf("result=%#v attachment=%#v outbox=%#v", result, store.attachments[attachmentID], store.outbox)
	}
}

func TestService_DeleteAttachmentQueuesStorageCleanup(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	files := &fakeFileStorage{}
	service.Files = files
	commentID := uuid.New()
	comment := activeComment(commentID, thread.ID, actor.UserID, service.Now())
	comment.Body = "text remains"
	store.comments[commentID] = comment
	attachmentID := uuid.New()
	fileID := uuid.New()
	store.attachments[attachmentID] = boundAttachment(attachmentID, fileID, thread.ID, commentID, actor.UserID, domain.AttachmentStatusReady, service.Now())
	service.NewID = idSequence(uuid.New())

	deleted, err := service.DeleteAttachment(context.Background(), actor, attachmentID)
	if err != nil || deleted.Status != domain.AttachmentStatusDeleted || files.deletedFileID != uuid.Nil {
		t.Fatalf("deleted=%#v error=%v storageDelete=%s", deleted, err, files.deletedFileID)
	}
	result, err := service.ProcessAttachmentWork(context.Background(), 10)
	if err != nil || result.Deleted != 1 || files.deletedFileID != fileID || store.attachments[attachmentID].StorageDeletedAt == nil {
		t.Fatalf("work=%#v error=%v attachment=%#v", result, err, store.attachments[attachmentID])
	}
}

func TestService_ProcessAttachmentWorkCleansExpiredPendingUpload(t *testing.T) {
	t.Parallel()

	service, store, actor, thread := newFixture()
	files := &fakeFileStorage{}
	service.Files = files
	attachmentID := uuid.New()
	fileID := uuid.New()
	now := service.Now()
	store.attachments[attachmentID] = domain.Attachment{
		ID: attachmentID, FileStorageID: fileID, ThreadID: thread.ID, UploaderID: actor.UserID,
		Status: domain.AttachmentStatusPending, MIMEType: "image/png", SizeBytes: 100,
		Width: 2, Height: 3, OriginalFilename: "expired.png", ExpiresAt: now.Add(-time.Minute),
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}

	result, err := service.ProcessAttachmentWork(context.Background(), 10)
	attachment := store.attachments[attachmentID]
	if err != nil || result.Deleted != 1 || attachment.Status != domain.AttachmentStatusDeleted ||
		attachment.StorageDeletedAt == nil || files.deletedFileID != fileID {
		t.Fatalf("work=%#v error=%v attachment=%#v storageDelete=%s", result, err, attachment, files.deletedFileID)
	}
}

type fakeFileStorage struct {
	storedID        uuid.UUID
	upload          commentuc.TemporaryFileInput
	activateErr     error
	activatedFileID uuid.UUID
	signedURL       string
	signedFileID    uuid.UUID
	deletedFileID   uuid.UUID
	deleteErr       error
}

func (f *fakeFileStorage) UploadTemporary(_ context.Context, input commentuc.TemporaryFileInput) (commentuc.StoredFile, error) {
	f.upload = input
	return commentuc.StoredFile{ID: f.storedID}, nil
}
func (f *fakeFileStorage) Activate(_ context.Context, fileID uuid.UUID) error {
	f.activatedFileID = fileID
	return f.activateErr
}
func (f *fakeFileStorage) SignedGETURL(_ context.Context, fileID uuid.UUID, _ int) (string, error) {
	f.signedFileID = fileID
	return f.signedURL, nil
}
func (f *fakeFileStorage) Delete(_ context.Context, fileID uuid.UUID) error {
	f.deletedFileID = fileID
	return f.deleteErr
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return buffer.Bytes()
}

func boundAttachment(id, fileID, threadID, commentID, uploaderID uuid.UUID, status domain.AttachmentStatus, now time.Time) domain.Attachment {
	return domain.Attachment{
		ID: id, FileStorageID: fileID, ThreadID: threadID, CommentID: &commentID, UploaderID: uploaderID,
		Status: status, MIMEType: "image/png", SizeBytes: 100, Width: 2, Height: 3,
		OriginalFilename: "image.png", ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
}
