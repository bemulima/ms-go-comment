package postgres

import (
	"errors"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "23503", "23505", "23514":
			return domain.ErrConflict
		}
	}
	return err
}
