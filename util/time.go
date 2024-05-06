package util

import (
	"context"
	"time"
)

func SleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}
