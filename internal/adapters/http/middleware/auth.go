package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/bemulima/ms-go-comment/internal/domain"
	accessuc "github.com/bemulima/ms-go-comment/internal/usecase/access"
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

func CaptureAccessGrant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := accessuc.WithGrantToken(r.Context(), r.Header.Get("X-Comment-Access-Grant"))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireInternalToken(token string, writeError ErrorWriter) func(http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actual := sha256.Sum256([]byte(r.Header.Get("X-Internal-Token")))
			if token == "" || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
				writeError(w, domain.ErrInternalAuthentication)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ActorFromContext(ctx context.Context) (domain.Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(domain.Actor)
	return actor, ok
}
