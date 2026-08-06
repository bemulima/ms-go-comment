package domain

import (
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var accessGrantIssuerPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

type AccessPermission int16

const (
	AccessPermissionRead AccessPermission = 1 << iota
	AccessPermissionWrite
	AccessPermissionUpload
	accessPermissionMask = AccessPermissionRead | AccessPermissionWrite | AccessPermissionUpload
)

func (p AccessPermission) Includes(permission AccessPermission) bool {
	return p&permission == permission
}

func (p AccessPermission) Valid() bool {
	return p&^accessPermissionMask == 0 && p.Includes(AccessPermissionRead) &&
		(!p.Includes(AccessPermissionUpload) || p.Includes(AccessPermissionWrite))
}

func FullAccessPermissions() AccessPermission {
	return AccessPermissionRead | AccessPermissionWrite | AccessPermissionUpload
}

type AccessGrant struct {
	GrantHash   []byte
	Issuer      string
	UserID      uuid.UUID
	SpaceID     uuid.UUID
	Resource    ResourceReference
	Permissions AccessPermission
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

func (g AccessGrant) Validate() error {
	if len(g.GrantHash) != 32 || g.UserID == uuid.Nil || g.SpaceID == uuid.Nil {
		return fmt.Errorf("%w: hash, user, and space are required", ErrInvalidAccessGrant)
	}
	if !accessGrantIssuerPattern.MatchString(g.Issuer) {
		return fmt.Errorf("%w: issuer is invalid", ErrInvalidAccessGrant)
	}
	if err := g.Resource.Validate(); err != nil {
		return fmt.Errorf("%w: resource is invalid", ErrInvalidAccessGrant)
	}
	if !g.Permissions.Valid() {
		return fmt.Errorf("%w: permissions are invalid", ErrInvalidAccessGrant)
	}
	if !g.ExpiresAt.After(g.CreatedAt) {
		return fmt.Errorf("%w: expiry must follow creation", ErrInvalidAccessGrant)
	}
	return nil
}
