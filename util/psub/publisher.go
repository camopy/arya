package psub

import (
	"context"
	"time"

	"github.com/camopy/rss_everything/util/run"
)

type Publisher[T any] interface {
	SendData(ctx context.Context, data T) error
	SendError(err error)
}

func NewThrottlingPublisher[T any](
	ctx context.Context,
	publisher Publisher[T],
	duration time.Duration,
	merger func(a, b T) T,
) Publisher[T] {
	p := &throttlingPublisher[T]{
		pub:      publisher,
		duration: duration,
		merger:   merger,
		dataCh:   make(chan T),
	}
	go run.DoCtx(ctx, ".throttling", p.startDataListener)
	return p
}

var _ Publisher[string] = (*throttlingPublisher[string])(nil)

type throttlingPublisher[T any] struct {
	pub      Publisher[T]
	duration time.Duration
	merger   func(a, b T) T
	dataCh   chan T
}

func (p *throttlingPublisher[T]) startDataListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-p.dataCh:
			_ = p.throttleData(ctx, data)
		}
	}
}

func (p *throttlingPublisher[T]) throttleData(ctx context.Context, data T) error {
	timer := time.NewTimer(p.duration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return p.pub.SendData(ctx, data)
		case update := <-p.dataCh:
			if p.merger == nil {
				data = update
			} else {
				data = p.merger(data, update)
			}
		}
	}
}

func (p *throttlingPublisher[T]) SendData(ctx context.Context, data T) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.dataCh <- data:
		return nil
	}
}

func (p throttlingPublisher[T]) SendError(err error) {
	p.pub.SendError(err)
}
