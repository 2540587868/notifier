package retrier

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

type Retrier struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func New(maxAttempts int, baseDelay, maxDelay time.Duration) *Retrier {
	return &Retrier{
		MaxAttempts: maxAttempts,
		BaseDelay:   baseDelay,
		MaxDelay:    maxDelay,
	}
}

func (r *Retrier) Do(ctx context.Context, fn func() error) error {
	var lastErr error
	for i := 0; i <= r.MaxAttempts; i++ {
		if i > 0 {
			delay := time.Duration(float64(r.BaseDelay) * math.Pow(2, float64(i-1)) * (0.8 + rand.Float64()*0.4))
			if delay > r.MaxDelay {
				delay = r.MaxDelay
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := fn(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("retry exhausted after %d attempts: %w", r.MaxAttempts+1, lastErr)
}
