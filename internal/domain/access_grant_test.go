package domain_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

func TestAccessPermission_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		permissions domain.AccessPermission
		valid       bool
	}{
		{name: "read", permissions: domain.AccessPermissionRead, valid: true},
		{name: "read write", permissions: domain.AccessPermissionRead | domain.AccessPermissionWrite, valid: true},
		{name: "full", permissions: domain.FullAccessPermissions(), valid: true},
		{name: "missing read", permissions: domain.AccessPermissionWrite},
		{name: "upload without write", permissions: domain.AccessPermissionRead | domain.AccessPermissionUpload},
		{name: "unknown bit", permissions: domain.FullAccessPermissions() | 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.permissions.Valid(); got != tt.valid {
				t.Fatalf("Valid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestAccessGrant_Validate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	valid := domain.AccessGrant{
		GrantHash: bytes.Repeat([]byte{1}, 32), Issuer: "ms-go-course", UserID: uuid.New(), SpaceID: uuid.New(),
		Resource: domain.ResourceReference{Type: "lesson", ID: "lesson-1"}, Permissions: domain.FullAccessPermissions(),
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*domain.AccessGrant)
	}{
		{name: "short hash", mutate: func(item *domain.AccessGrant) { item.GrantHash = item.GrantHash[:31] }},
		{name: "invalid issuer", mutate: func(item *domain.AccessGrant) { item.Issuer = "Course Service" }},
		{name: "invalid resource", mutate: func(item *domain.AccessGrant) { item.Resource.ID = " " }},
		{name: "invalid permission", mutate: func(item *domain.AccessGrant) { item.Permissions = domain.AccessPermissionWrite }},
		{name: "expired at creation", mutate: func(item *domain.AccessGrant) { item.ExpiresAt = item.CreatedAt }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := valid
			item.GrantHash = append([]byte(nil), valid.GrantHash...)
			tt.mutate(&item)
			if err := item.Validate(); !errors.Is(err, domain.ErrInvalidAccessGrant) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
