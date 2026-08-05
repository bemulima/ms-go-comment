// Package repository defines persistence ports owned by the domain boundary.
// Implementations may attach a transaction to context, but callers never depend
// on a concrete database transaction type.
package repository

import "context"

type TransactionManager interface {
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}
