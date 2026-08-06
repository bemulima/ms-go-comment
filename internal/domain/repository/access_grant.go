package repository

import (
	"context"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
)

type AccessGrantRepository interface {
	Store(ctx context.Context, grant domain.AccessGrant) error
	Resolve(ctx context.Context, grantHash []byte, now time.Time) (domain.AccessGrant, error)
	DeleteExpired(ctx context.Context, now time.Time, limit int) (int, error)
}
