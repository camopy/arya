package run

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/camopy/rss_everything/zaplog"
)

const debugLogging = false

type Func func(ctx context.Context) error

type Context interface {
	context.Context
	Named

	New(name string) Context
	Wait() error

	Start(activity Activity)
	Go(name string, fn Func)

	Cancel(err error)
	CancelIfErr(err error)
	OnCancel(fn func())
	OnCancelErr(fn func(err error))

	SetReady()
	Readiness(ctx context.Context) []CheckResult
	Liveness(ctx context.Context) []CheckResult
}

func NewContext(ctx context.Context, logger *zaplog.Logger, name string) Context {
	c := newContext(ctx, logger, name, newHealthChecker())
	c.hc.AddReadyCheck(name, c.ready.Check)
	return c
}

var _ Context = (*runCtx)(nil)

type runGroup struct {
	ctx    context.Context //nolint:containedctx // Context-aware type
	cancel context.CancelCauseFunc
	wg     sync.WaitGroup
}

type runCtx struct {
	context.Context //nolint:containedctx // Context-aware type

	cancel   context.CancelCauseFunc
	name     string
	logger   *zaplog.Logger
	workG    runGroup
	cancelG  runGroup
	doneCh   chan struct{}
	doneOnce sync.Once
	hc       *healthChecker
	ready    ReadyCheck
}

func newContext(ctx context.Context, logger *zaplog.Logger, name string, hc *healthChecker) *runCtx {
	g := &runCtx{
		name:   name,
		logger: logger,
		doneCh: make(chan struct{}),
		hc:     hc,
	}
	ctx, cancel := context.WithCancelCause(ctx)
	g.Context, g.cancel = ctx, cancel
	g.workG.ctx, g.workG.cancel = context.WithCancelCause(ctx)
	g.cancelG.ctx, g.cancelG.cancel = context.WithCancelCause(ctx)
	g.startWaitParentDone(ctx)
	return g
}

func (s *runCtx) new(name string) *runCtx {
	return newContext(s.workG.ctx, s.logger, s.name+"/"+name, s.hc)
}

func (s *runCtx) New(name string) Context {
	return s.new(name)
}

func (s *runCtx) Done() <-chan struct{} {
	return s.doneCh
}

func (s *runCtx) Err() error {
	select {
	case <-s.doneCh:
		return s.Context.Err()
	default:
		return nil
	}
}

func (s *runCtx) Name() string {
	return s.name
}

func (s *runCtx) Wait() error {
	return Wait(s)
}

func (s *runCtx) Start(activity Activity) {
	activityCtx := s.new(activity.Name())
	name := activityCtx.Name()
	s.logger.Info(fmt.Sprintf("starting %q...", name))
	if err := activity.Start(activityCtx); err != nil {
		activityCtx.Cancel(err)
		s.Cancel(err)
		s.addFailedChecks(name)
		s.logger.Info(fmt.Sprintf("failed to start %q", name), zap.Error(err))
	} else {
		s.startWaitChildDone(activityCtx)
		s.addChecks(name, activity)
		s.logger.Info(fmt.Sprintf("started %q", name))
	}
}

func (s *runCtx) addFailedChecks(name string) {
	s.hc.AddReadyCheck(name, checkAlwaysNotReady)
	s.hc.AddHealthCheck(name, checkAlwaysUnhealthy)
}

func (s *runCtx) addChecks(name string, activity Activity) {
	if h, ok := activity.(ReadinessHook); ok {
		s.hc.AddReadyCheck(name, h.Ready)
	} else {
		s.hc.AddReadyCheck(name, checkAlwaysOk)
	}
	if h, ok := activity.(LivenessHook); ok {
		s.hc.AddHealthCheck(name, h.Healthy)
	} else {
		s.hc.AddHealthCheck(name, checkAlwaysOk)
	}
}

func (s *runCtx) Go(name string, fn Func) {
	s.startWorkGoroutine(name, fn)
}

func (s *runCtx) Cancel(err error) {
	s.doneOnce.Do(func() {
		go DoCtx(s, ".cancel", func(context.Context) {
			err = ignoreExactCancelError(err)
			s.logger.Info(fmt.Sprintf("stopping %q...", s.name), zap.Error(err))
			s.workG.cancelAndWait(err)
			s.cancelG.cancelAndWait(err)
			s.cancel(err)
			close(s.doneCh)
			s.logger.Info(fmt.Sprintf("stopped %q", s.name), zap.Error(err))
		})
	})
}

func (s *runCtx) CancelIfErr(err error) {
	if err != nil {
		s.Cancel(err)
	}
}

func (s *runCtx) OnCancel(fn func()) {
	s.startCancelGoroutine("on-cancel", func(ctx context.Context) {
		fn()
	})
}

func (s *runCtx) OnCancelErr(fn func(err error)) {
	s.startCancelGoroutine("on-cancel", func(ctx context.Context) {
		fn(context.Cause(ctx))
	})
}

func (s *runCtx) SetReady() {
	s.ready.Set()
}

func (s *runCtx) Readiness(ctx context.Context) []CheckResult {
	return s.hc.Readiness(ctx)
}

func (s *runCtx) Liveness(ctx context.Context) []CheckResult {
	return s.hc.Liveness(ctx)
}

func (s *runCtx) startWaitParentDone(parent context.Context) {
	s.startWorkGoroutine("wait-parent", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			// already cancelled
			return ctx.Err()
		case <-parent.Done():
			return parent.Err()
		}
	})
}

func (s *runCtx) startWaitChildDone(child Context) {
	name := "wait-child:" + child.Name()
	s.startWorkGoroutine(name, func(ctx context.Context) error {
		<-child.Done()
		return child.Err()
	})
}

func (s *runCtx) startWorkGoroutine(name string, fn Func) {
	select {
	case <-s.workG.ctx.Done():
		return
	default:
		s.startGoroutine(s.workG.ctx, &s.workG.wg, name, fn)
	}
}

func (s *runCtx) startCancelGoroutine(name string, fn func(ctx context.Context)) {
	s.startGoroutine(s.cancelG.ctx, &s.cancelG.wg, name, func(ctx context.Context) error {
		<-ctx.Done()
		fn(ctx)
		return nil
	})
}

func (s *runCtx) startGoroutine(ctx context.Context, wg *sync.WaitGroup, name string, fn Func) {
	wg.Add(1)
	name = "run." + s.Name() + "@" + name
	go DoCtx(ctx, name, func(ctx context.Context) {
		defer wg.Done()
		s.debugLog(fmt.Sprintf("goroutine started %q", name), nil)
		err := fn(ctx)
		s.debugLog(fmt.Sprintf("goroutine stopped %q", name), ignoreExactCancelError(err))
		s.CancelIfErr(err) //nolint:contextcheck // internal Context passed
	})
}

func (s *runCtx) debugLog(message string, err error) {
	if debugLogging {
		s.logger.Debug(message, zap.Error(err))
	}
}

func (g *runGroup) cancelAndWait(err error) {
	g.cancel(err)
	g.wg.Wait()
}

func ignoreExactCancelError(err error) error {
	//goland:noinspection GoDirectComparisonOfErrors
	if err == context.Canceled { //nolint:errorlint // need exact comparison
		return nil
	}
	return err
}
