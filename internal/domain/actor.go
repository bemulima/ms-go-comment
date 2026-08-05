package domain

import (
	"strings"

	"github.com/google/uuid"
)

type Actor struct {
	UserID uuid.UUID
	Role   string
}

func (a Actor) Validate() error {
	if a.UserID == uuid.Nil || strings.TrimSpace(a.Role) == "" || strings.EqualFold(a.Role, "GUEST") {
		return ErrAuthenticationRequired
	}
	return nil
}
