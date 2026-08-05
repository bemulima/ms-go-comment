package repository

import (
	"context"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

type SpaceListQuery struct {
	Status *domain.SpaceStatus
	Limit  int
	Offset int
}

type SpaceRepository interface {
	Create(ctx context.Context, space domain.Space) error
	GetByID(ctx context.Context, id uuid.UUID) (domain.Space, error)
	GetByKey(ctx context.Context, key string) (domain.Space, error)
	Update(ctx context.Context, space domain.Space) error
	List(ctx context.Context, query SpaceListQuery) ([]domain.Space, error)
}
