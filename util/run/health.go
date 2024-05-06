package run

import (
	"context"
	"errors"
	ge "github.com/camopy/rss_everything/util/generics"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	jsoniter "github.com/json-iterator/go"
)

type CheckFunc func(ctx context.Context) error

var (
	ErrNotReady  = errors.New("not ready")
	ErrUnhealthy = errors.New("unhealthy")

	checkAlwaysOk        CheckFunc = func(context.Context) error { return nil }
	checkAlwaysNotReady  CheckFunc = func(context.Context) error { return ErrNotReady }
	checkAlwaysUnhealthy CheckFunc = func(context.Context) error { return ErrUnhealthy }
)

type ReadinessHook interface {
	Ready(ctx context.Context) error
}

type LivenessHook interface {
	Healthy(ctx context.Context) error
}

type CheckResult struct {
	Name  string `json:"name"`
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type healthChecker struct {
	mu           sync.Mutex
	readyChecks  []check
	healthChecks []check
}

type check struct {
	CheckResult
	fn CheckFunc
}

func newHealthChecker() *healthChecker {
	return &healthChecker{}
}

func (s *healthChecker) AddReadyCheck(name string, check CheckFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addCheck(name, check, &s.readyChecks)
}

func (s *healthChecker) AddHealthCheck(name string, check CheckFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addCheck(name, check, &s.healthChecks)
}

func (s *healthChecker) addCheck(name string, fn CheckFunc, list *[]check) {
	*list = append(*list, check{
		fn:          fn,
		CheckResult: CheckResult{Name: name},
	})
	slices.SortFunc(*list, func(a, b check) int {
		return strings.Compare(a.Name, b.Name)
	})
}

func (s *healthChecker) Readiness(ctx context.Context) []CheckResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ge.MapPtr[check, CheckResult](s.readyChecks, func(c *check) CheckResult {
		if !c.Ok {
			c.update(ctx)
		}
		return c.CheckResult
	})
}

func (s *healthChecker) Liveness(ctx context.Context) []CheckResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ge.MapPtr[check, CheckResult](s.healthChecks, func(c *check) CheckResult {
		c.update(ctx)
		return c.CheckResult
	})
}

type ReadyCheck struct {
	v atomic.Bool
}

func (c *ReadyCheck) Check(context.Context) error {
	if c.v.Load() {
		return nil
	}
	return ErrNotReady
}

func (c *ReadyCheck) Set() {
	c.v.Store(true)
}

func (c *check) update(ctx context.Context) {
	if err := c.fn(ctx); err != nil {
		c.Error = err.Error()
	} else {
		c.Ok = true
		c.Error = ""
	}
}

type healthHandler struct {
	mux http.ServeMux
	ctx Context
}

func HealthHandler(ctx Context) http.Handler {
	h := healthHandler{ctx: ctx}
	h.mux.HandleFunc("/ready", h.handleReady)
	h.mux.HandleFunc("/live", h.handleLive)
	return &h.mux
}

func (h *healthHandler) handleReady(w http.ResponseWriter, req *http.Request) {
	h.writeResponse(w, req, h.ctx.Readiness(req.Context()))
}

func (h *healthHandler) handleLive(w http.ResponseWriter, req *http.Request) {
	h.writeResponse(w, req, h.ctx.Liveness(req.Context()))
}

func (h *healthHandler) writeResponse(w http.ResponseWriter, req *http.Request, checks []CheckResult) {
	ok := true
	if len(checks) == 0 {
		ok = false
	}
	for _, c := range checks {
		ok = ok && c.Ok
	}

	var contentType string
	var body []byte

	if req.URL.Query().Has("verbose") {
		contentType = "application/json"
		body, _ = jsoniter.ConfigFastest.Marshal(checks)
	} else {
		contentType = "text/plain"
		body = []byte(ge.Cond(ok, "ok", "failed"))
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(ge.Cond(ok, http.StatusOK, http.StatusServiceUnavailable))
	_, _ = w.Write(body)
}
