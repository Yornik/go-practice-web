package fixture

import (
	"context"
	"time"
)

func Wait(ctx context.Context, ready <-chan struct{}) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}
