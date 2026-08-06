package access

import (
	"context"
	"strings"
)

type grantTokenKey struct{}

func WithGrantToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, grantTokenKey{}, strings.TrimSpace(token))
}

func GrantTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(grantTokenKey{}).(string)
	return token, ok && token != ""
}
