package requestctx

import "context"

type userKey struct{}

func WithUser(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}
func UserID(ctx context.Context) string { value, _ := ctx.Value(userKey{}).(string); return value }
