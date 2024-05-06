package psub

import (
	"context"
	"fmt"
	"sync"

	"github.com/camopy/rss_everything/util/run"
)

type Subscriber[T any] interface {
	Subscribe(ctx context.Context, opts ...SubscriptionOption) Subscription[T]
}

var _ Subscriber[string] = SubscribeFunc[string](nil)

type SubscribeFunc[T any] func(ctx context.Context, opts ...SubscriptionOption) Subscription[T]

func (f SubscribeFunc[T]) Subscribe(ctx context.Context, opts ...SubscriptionOption) Subscription[T] {
	return f(ctx, opts...)
}

type SubscriberOption func(*subscriberConfig)

func WithSubscriberName(name string) SubscriberOption {
	return func(cfg *subscriberConfig) {
		cfg.Name = name
	}
}

func WithSubscriberSubscriptionOptions(opts ...SubscriptionOption) SubscriberOption {
	return func(cfg *subscriberConfig) {
		cfg.SubscriptionOptions = opts
	}
}

var (
	_ Subscriber[string] = (*subscriber[string])(nil)
	_ Publisher[string]  = (*subscriber[string])(nil)
)

type subscriber[T any] struct {
	mu            sync.RWMutex
	cfg           subscriberConfig
	nextSubId     int
	subscriptions map[int]*subscription[T]
}

type subscriberConfig struct {
	Name                string
	SubscriptionOptions []SubscriptionOption
}

func NewSubscriber[T any](opts ...SubscriberOption) (Subscriber[T], Publisher[T]) {
	cfg := subscriberConfig{
		Name: generateName("subscriber"),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	sub := &subscriber[T]{
		cfg:           cfg,
		nextSubId:     1,
		subscriptions: map[int]*subscription[T]{},
	}
	return sub, sub
}

func (s *subscriber[T]) Subscribe(ctx context.Context, opts ...SubscriptionOption) Subscription[T] {
	s.mu.Lock()
	defer s.mu.Unlock()

	subId := s.nextSubId
	s.nextSubId++

	opts = append(append(s.cfg.SubscriptionOptions, WithSubscriptionName(fmt.Sprintf("%s:sub-%d", s.cfg.Name, subId))), opts...)
	sub := newSubscription[T](ctx, nil, opts...)
	s.subscriptions[subId] = sub
	go run.DoCtx(ctx, ".subscribe", func(context.Context) {
		// TODO: can be refactored to use only one goroutine and process all subscriptions with dynamic channel select using reflection
		<-sub.Done()
		s.mu.Lock()
		delete(s.subscriptions, subId)
		s.mu.Unlock()
	})
	return sub
}

func (s *subscriber[T]) SendData(ctx context.Context, data T) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subscriptions {
		if err := sub.SendData(ctx, data); err != nil {
			return err
		}
	}
	return nil
}

func (s *subscriber[T]) SendError(err error) {
	if err == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subscriptions {
		sub.SendError(err)
	}
}
