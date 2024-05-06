package psub

import (
	"context"

	"github.com/camopy/rss_everything/util/errutil"
	ge "github.com/camopy/rss_everything/util/generics"
	"github.com/camopy/rss_everything/util/run"
)

type SubscriptionOption func(*subscriptionConfig)

type Subscription[T any] interface {
	Name() string
	Context() context.Context
	Done() <-chan struct{}
	Err() error
	Data() <-chan T
	Cancel()
}

func WithSubscriptionName(name string) SubscriptionOption {
	return func(cfg *subscriptionConfig) {
		cfg.Name = name
	}
}

func WithSubscriptionNamePrefix(namePrefix string) SubscriptionOption {
	return func(cfg *subscriptionConfig) {
		cfg.NamePrefix = namePrefix
	}
}

func WithSubscriptionBlocking(blocking bool) SubscriptionOption {
	return func(cfg *subscriptionConfig) {
		cfg.Blocking = blocking
	}
}

func WithSubscriptionBufferSize(size int) SubscriptionOption {
	return func(cfg *subscriptionConfig) {
		cfg.BufSize = size
	}
}

func NewSubscription[T any](ctx context.Context, opts ...SubscriptionOption) (Subscription[T], Publisher[T]) {
	sub := newSubscription[T](ctx, nil, opts...)
	return sub, sub
}

func NewSubscriptionWithChannel[T any](ctx context.Context, dataChan chan T, opts ...SubscriptionOption) (Subscription[T], Publisher[T]) {
	sub := newSubscription[T](ctx, dataChan, opts...)
	return sub, sub
}

func NewErrorSubscription[T any](ctx context.Context, err error, opts ...SubscriptionOption) Subscription[T] {
	sub := newSubscription[T](ctx, nil, opts...)
	sub.SendError(err)
	return sub
}

type subscriptionConfig struct {
	Name       string
	NamePrefix string
	BufSize    int
	Blocking   bool
}

var (
	_ Subscription[string] = (*subscription[string])(nil)
	_ Publisher[string]    = (*subscription[string])(nil)
)

func newSubscription[T any](ctx context.Context, dataChan chan T, opts ...SubscriptionOption) *subscription[T] {
	ctx, cancel := context.WithCancel(ctx)
	var cfg subscriptionConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Name == "" {
		cfg.Name = generateName(ge.FirstNonZero(cfg.NamePrefix, "subscription"))
	}
	if dataChan == nil {
		dataChan = make(chan T, cfg.BufSize)
	}
	errDone := errutil.NewErrorDone(cancel)
	s := &subscription[T]{
		ctx:     ctx,
		cfg:     cfg,
		dataCh:  dataChan,
		errDone: errDone,
	}
	go run.DoCtx(ctx, ".psub-wait-ctx", func(ctx context.Context) {
		<-ctx.Done()
		errDone.SendError(ctx.Err())
	})
	return s
}

type subscription[T any] struct {
	ctx     context.Context //nolint:containedctx // Context-aware type
	cfg     subscriptionConfig
	dataCh  chan T
	errDone *errutil.ErrorDone
}

func (s *subscription[T]) Name() string {
	return s.cfg.Name
}

func (s *subscription[T]) Context() context.Context {
	return s.ctx
}

func (s *subscription[T]) Done() <-chan struct{} {
	return s.errDone.Done()
}

func (s *subscription[T]) Err() error {
	return s.errDone.Err()
}

func (s *subscription[T]) Data() <-chan T {
	return s.dataCh
}

func (s *subscription[T]) Cancel() {
	s.errDone.SendError(context.Canceled)
}

func (s *subscription[T]) SendData(ctx context.Context, data T) error {
	if s.cfg.Blocking {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.Done():
			return s.Err()
		case s.dataCh <- data:
		}
	} else {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.Done():
			return s.Err()
		case s.dataCh <- data:
		default:
		}
	}
	return nil
}

func (s *subscription[T]) SendError(err error) {
	s.errDone.SendError(err)
}
