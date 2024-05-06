package run

import (
	"context"
	"runtime/pprof"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/camopy/rss_everything/util"
	"github.com/camopy/rss_everything/util/errutil"
	"github.com/camopy/rss_everything/zaplog"
)

const contextLabelName = "aurox"

func Do(name string, fn func()) {
	DoCtx(context.Background(), name, func(_ context.Context) { fn() })
}

func DoCtx(ctx context.Context, name string, fn func(ctx context.Context)) {
	if strings.HasPrefix(name, ".") {
		name = name[1:]
		if value, _ := pprof.Label(ctx, contextLabelName); value != "" {
			name = value + "__" + name
		}
	}
	pprof.Do(ctx, pprof.Labels(contextLabelName, name), fn)
}

func Wait(c context.Context) error {
	<-c.Done()
	return c.Err()
}

func Periodically(logger *zaplog.Logger, runAfter, interval time.Duration, fn Func) Func {
	return func(ctx context.Context) error {
		waitDuration := runAfter
		for {
			if err := util.SleepContext(ctx, waitDuration); err != nil {
				return err
			}
			waitDuration = interval
			if err := fn(ctx); err != nil {
				waitDuration /= 2
				logger.Error("run: periodic handler failed", zap.Error(err))
			}
		}
	}
}

func OnInterval(logger *zaplog.Logger, interval time.Duration, fn Func) Func {
	return func(ctx context.Context) error {
		for {
			now := time.Now()
			duration := now.Add(interval).Truncate(interval).Sub(now)
			if err := util.SleepContext(ctx, duration); err != nil {
				return err
			}
			if err := fn(ctx); err != nil {
				logger.Error("run: interval handler failed", zap.Error(err))
			}
		}
	}
}

type RetryFunc func(ctx context.Context) (retryAfter time.Duration, err error)

func Retry(logger *zaplog.Logger, fn func(ctx context.Context) (retryAfter time.Duration, err error)) Func {
	return func(ctx context.Context) error {
		for {
			retryAfter, err := fn(ctx)
			if err == nil || errutil.IsFatalError(err) {
				return err
			}
			logger.Error("run: retry handler failed", zap.Error(err))
			if err = util.SleepContext(ctx, retryAfter); err != nil {
				return err
			}
		}
	}
}
