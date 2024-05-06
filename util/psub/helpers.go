package psub

import (
	"context"
	"fmt"

	"github.com/camopy/rss_everything/util/run"
)

func Process[T any](sub Subscription[T], fn func(ctx context.Context, data T) error) error {
	return ProcessWithContext(sub.Context(), sub, fn)
}

func ProcessWithContext[T any](ctx context.Context, sub Subscription[T], fn func(ctx context.Context, data T) error) error {
	defer sub.Cancel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data := <-sub.Data():
			if err := fn(ctx, data); err != nil {
				return err
			}
		}
	}
}

func SubscriptionMux[T any](sub Subscription[T], opts ...SubscriberOption) Subscriber[T] {
	opts = append([]SubscriberOption{WithSubscriberName(sub.Name())}, opts...)
	s, pub := NewSubscriber[T](opts...)
	go run.DoCtx(sub.Context(), ".mux", func(ctx context.Context) {
		pub.SendError(Process[T](sub, pub.SendData))
	})
	return s
}

func MapSubscription[T, R any](sub Subscription[T], mapper func(data T) R, opts ...SubscriptionOption) Subscription[R] {
	return WrapSubscription[T, R](
		sub,
		nil,
		func(ctx context.Context, data T) (res R, send bool, err error) {
			return mapper(data), true, nil
		},
		opts...,
	)
}

func WrapSubscription[T, R any](
	subscription Subscription[T],
	init func(ctx context.Context, publisher Publisher[R]) error,
	mapper func(ctx context.Context, data T) (res R, send bool, err error),
	opts ...SubscriptionOption,
) Subscription[R] {
	wrapped, pub := NewSubscription[R](subscription.Context(), opts...)
	go run.DoCtx(wrapped.Context(), ".wrap", func(ctx context.Context) {
		if init != nil {
			if err := init(ctx, pub); err != nil {
				pub.SendError(fmt.Errorf("failed to initialize %q subscription: %w", wrapped.Name(), err))
			}
		}
		pub.SendError(ProcessWithContext[T](ctx, subscription, func(ctx context.Context, data T) error {
			res, send, err := mapper(ctx, data)
			if !send || err != nil {
				return err
			}
			return pub.SendData(ctx, res)
		}))
	})
	return wrapped
}
