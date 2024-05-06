package psub

import (
	"context"
	"fmt"
	"reflect"

	"github.com/camopy/rss_everything/util/run"
)

func MergeSubscriptions[T any](
	ctx context.Context,
	subscriptions []Subscription[T],
	fetch func(items []*T) (T, bool),
	opts ...SubscriptionOption,
) Subscription[T] {
	if len(subscriptions) == 0 {
		panic("subscriptions are empty")
	}
	if len(subscriptions) == 1 {
		return subscriptions[0]
	}

	opts = append([]SubscriptionOption{WithSubscriptionName(generateName("merged"))}, opts...)
	sub, pub := NewSubscription[T](ctx, opts...)

	cases := make([]reflect.SelectCase, len(subscriptions)*2+1)
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(sub.Done()),
	}
	for i, s := range subscriptions {
		cases[i*2+1] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(s.Data()),
		}
		cases[i*2+2] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(s.Done()),
		}
	}

	process := func() error {
		values := make([]*T, len(subscriptions))
		for {
			chosen, recv, ok := reflect.Select(cases)
			subIndex := (chosen - 1) / 2
			if !ok {
				if chosen == 0 {
					return fmt.Errorf("MergeSubscriptions: merged subscription closed")
				}
				s := subscriptions[subIndex]
				if chosen%2 == 1 {
					return fmt.Errorf("MergeSubscriptions: subscription %q closed: %w", s.Name(), s.Err())
				}
				return fmt.Errorf("MergeSubscriptions: unexpected closed channel: %d", chosen)
			}

			value := recv.Interface().(T) //nolint:errcheck // generics
			values[subIndex] = &value

			data, ok := fetch(values)
			if ok {
				if err := pub.SendData(ctx, data); err != nil {
					return err
				}
			}
		}
	}

	go run.DoCtx(ctx, ".merge", func(context.Context) {
		pub.SendError(process())
	})

	return sub
}

func MergeSubscribers[T any](
	subscribers []Subscriber[T],
	fetch func(items []*T) (T, bool),
) Subscriber[T] {
	if len(subscribers) == 1 {
		return subscribers[0]
	}
	return SubscribeFunc[T](func(ctx context.Context, opts ...SubscriptionOption) Subscription[T] {
		subscriptions := make([]Subscription[T], len(subscribers))
		for i, s := range subscribers {
			subscriptions[i] = s.Subscribe(ctx, opts...)
		}
		return MergeSubscriptions(ctx, subscriptions, fetch, opts...)
	})
}
