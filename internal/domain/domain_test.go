package domain_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

func TestPolicyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		change  func(*domain.Policy)
		wantErr bool
	}{
		{name: "defaults are valid"},
		{name: "zero depth", change: func(p *domain.Policy) { p.MaxDepth = 0 }, wantErr: true},
		{name: "too many attachments", change: func(p *domain.Policy) { p.MaxAttachments = 11 }, wantErr: true},
		{name: "negative edit window", change: func(p *domain.Policy) { p.EditWindowSeconds = -1 }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := domain.DefaultPolicy()
			if tt.change != nil {
				tt.change(&policy)
			}
			if gotErr := policy.Validate() != nil; gotErr != tt.wantErr {
				t.Fatalf("Validate() error = %v, want error %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestApplyPolicyUsesOnlyNonNilOverrides(t *testing.T) {
	t.Parallel()

	base := domain.DefaultPolicy()
	allowImages := true
	maxDepth := int16(4)
	effective, err := domain.ApplyPolicy(base, domain.ThreadPolicyOverrides{
		AllowImages: &allowImages,
		MaxDepth:    &maxDepth,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy() error = %v", err)
	}
	if !effective.AllowImages || effective.MaxDepth != 4 || effective.MaxBodyLength != base.MaxBodyLength {
		t.Fatalf("unexpected effective policy: %#v", effective)
	}
}

func TestResourceReferenceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resource  domain.ResourceReference
		wantError bool
	}{
		{name: "opaque id remains case-sensitive", resource: domain.ResourceReference{Type: "course.lesson", ID: "Course/ABC-42"}},
		{name: "uppercase type", resource: domain.ResourceReference{Type: "Course", ID: "42"}, wantError: true},
		{name: "blank id", resource: domain.ResourceReference{Type: "course", ID: "  "}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resource.Validate() != nil; got != tt.wantError {
				t.Fatalf("Validate() error = %v, want error %v", got, tt.wantError)
			}
		})
	}
}

func TestBuildCommentPlacement(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	rootID := uuid.New()
	rootPlacement, err := domain.BuildCommentPlacement(rootID, threadID, nil, 3)
	if err != nil {
		t.Fatalf("root placement error = %v", err)
	}
	if rootPlacement.RootID != rootID || rootPlacement.Depth != 0 || len(rootPlacement.Path) != 1 {
		t.Fatalf("unexpected root placement: %#v", rootPlacement)
	}

	root := domain.Comment{
		ID: rootID, ThreadID: threadID, RootID: rootID,
		Path: []uuid.UUID{rootID}, Status: domain.CommentStatusActive,
	}
	replyID := uuid.New()
	reply, err := domain.BuildCommentPlacement(replyID, threadID, &root, 3)
	if err != nil {
		t.Fatalf("reply placement error = %v", err)
	}
	if reply.ParentID == nil || *reply.ParentID != rootID || reply.RootID != rootID || reply.Depth != 1 || len(reply.Path) != 2 {
		t.Fatalf("unexpected reply placement: %#v", reply)
	}

	foreign := root
	foreign.ThreadID = uuid.New()
	if _, err := domain.BuildCommentPlacement(uuid.New(), threadID, &foreign, 3); !errors.Is(err, domain.ErrCrossThreadParent) {
		t.Fatalf("cross-thread error = %v", err)
	}

	deep := root
	deep.ID = uuid.New()
	deep.ParentID = ptrUUID(uuid.New())
	deep.Depth = 3
	deep.Path = []uuid.UUID{rootID, uuid.New(), uuid.New(), deep.ID}
	if _, err := domain.BuildCommentPlacement(uuid.New(), threadID, &deep, 3); !errors.Is(err, domain.ErrMaxDepthExceeded) {
		t.Fatalf("max-depth error = %v", err)
	}
}

func TestCommentContentValidation(t *testing.T) {
	t.Parallel()

	base := domain.DefaultPolicy()
	tests := []struct {
		name    string
		policy  domain.Policy
		content domain.CommentContent
		wantErr error
	}{
		{name: "body", policy: base, content: domain.CommentContent{Body: "hello"}},
		{name: "blank", policy: base, content: domain.CommentContent{}, wantErr: domain.ErrInvalidCommentContent},
		{name: "raw html", policy: base, content: domain.CommentContent{Body: "<b>x</b>", ContainsRawHTML: true}, wantErr: domain.ErrInvalidCommentContent},
		{name: "valid link", policy: base, content: domain.CommentContent{Body: "docs", Links: []domain.Link{{URL: "https://example.com/a"}}}},
		{name: "invalid link scheme", policy: base, content: domain.CommentContent{Body: "docs", Links: []domain.Link{{URL: "javascript:alert(1)"}}}, wantErr: domain.ErrInvalidCommentContent},
		{name: "links disabled", policy: withLinks(base, false), content: domain.CommentContent{Body: "docs", Links: []domain.Link{{URL: "https://example.com"}}}, wantErr: domain.ErrLinksDisabled},
		{name: "images disabled", policy: base, content: domain.CommentContent{AttachmentCount: 1}, wantErr: domain.ErrImagesDisabled},
		{name: "image only", policy: withImages(base, true), content: domain.CommentContent{AttachmentCount: 1}},
		{name: "existing image survives disabled policy", policy: base, content: domain.CommentContent{ExistingAttachmentCount: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.content.Validate(tt.policy)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAnalyzeCommentContent(t *testing.T) {
	t.Parallel()

	content := domain.AnalyzeCommentContent(
		"See HTTPS://example.com/docs, duplicate HTTPS://example.com/docs and <script>alert(1)</script>", 2,
	)
	if !content.ContainsRawHTML {
		t.Fatal("raw HTML was not detected")
	}
	if content.AttachmentCount != 2 || len(content.Links) != 1 || content.Links[0].URL != "HTTPS://example.com/docs" {
		t.Fatalf("unexpected analyzed content: %#v", content)
	}
}

func TestAttachmentValidation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	attachment := domain.Attachment{
		ID: uuid.New(), ThreadID: uuid.New(), UploaderID: uuid.New(), FileStorageID: uuid.New(),
		Status: domain.AttachmentStatusPending, MIMEType: "image/webp", SizeBytes: 1024,
		Width: 100, Height: 100, OriginalFilename: "image.webp", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := attachment.Validate(domain.DefaultPolicy()); !errors.Is(err, domain.ErrImagesDisabled) {
		t.Fatalf("disabled-images error = %v", err)
	}
	policy := withImages(domain.DefaultPolicy(), true)
	if err := attachment.Validate(policy); err != nil {
		t.Fatalf("valid attachment error = %v", err)
	}
	attachment.MIMEType = "image/svg+xml"
	if err := attachment.Validate(policy); !errors.Is(err, domain.ErrInvalidAttachment) {
		t.Fatalf("unsafe MIME error = %v", err)
	}
}

func TestOutboxEventAndRealtimeTicketValidation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	event := domain.OutboxEvent{
		ID: uuid.New(), AggregateType: "comment", AggregateID: uuid.New(),
		Subject: domain.EventCommentCreated, SchemaVersion: 1,
		Payload: json.RawMessage(`{"comment_id":"123"}`), NextAttemptAt: now, CreatedAt: now,
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("valid outbox event error = %v", err)
	}
	event.Payload = json.RawMessage(`[]`)
	if err := event.Validate(); !errors.Is(err, domain.ErrInvalidOutboxEvent) {
		t.Fatalf("array payload error = %v", err)
	}

	sequence := int64(7)
	ticket := domain.RealtimeTicket{
		TicketHash: make([]byte, 32), UserID: uuid.New(), ThreadID: uuid.New(),
		Permissions:           domain.RealtimePermissionRead | domain.RealtimePermissionWrite,
		RequestedLastSequence: &sequence, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := ticket.Validate(); err != nil {
		t.Fatalf("valid realtime ticket error = %v", err)
	}
	ticket.Permissions = domain.RealtimePermissionWrite
	if err := ticket.Validate(); !errors.Is(err, domain.ErrInvalidRealtimeTicket) {
		t.Fatalf("missing-read permission error = %v", err)
	}
}

func withImages(policy domain.Policy, allowed bool) domain.Policy {
	policy.AllowImages = allowed
	return policy
}

func withLinks(policy domain.Policy, allowed bool) domain.Policy {
	policy.AllowLinks = allowed
	return policy
}

func ptrUUID(value uuid.UUID) *uuid.UUID {
	return &value
}
