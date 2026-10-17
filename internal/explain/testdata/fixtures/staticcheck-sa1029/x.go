package fixture

import "context"

func WithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, "user", user)
}
