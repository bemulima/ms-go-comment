package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

type actorKey struct{}

type ErrorWriter func(http.ResponseWriter, error)

func RequireActor(writeError ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := uuid.Parse(r.Header.Get("X-User-ID"))
			role := strings.ToUpper(strings.TrimSpace(r.Header.Get("X-User-Role")))
			actor := domain.Actor{UserID: userID, Role: role}
			if err != nil || actor.Validate() != nil {
				writeError(w, domain.ErrAuthenticationRequired)
				return
			}
			ctx := context.WithValue(r.Context(), actorKey{}, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ActorFromContext(ctx context.Context) (domain.Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(domain.Actor)
	return actor, ok
}
