package main

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "rss_feed"
const subsystem = "http"

var metrics = struct {
	externalRequestDuration *prometheus.HistogramVec
	externalRequestTotal    *prometheus.CounterVec
}{
	externalRequestDuration: NewHistogramVec(
		subsystem,
		"external_request_duration_seconds",
		"Duration of external requests in seconds",
		[]string{"method", "endpoint", "code"},
		prometheus.DefBuckets,
	),
	externalRequestTotal: NewCounterVec(
		subsystem,
		"external_request_total",
		"Total number of external requests",
		[]string{"method", "endpoint", "statusCode"},
	),
}

func trackExternalRequest(method, endpoint string, statusCode int, duration time.Duration) {
	code := strconv.Itoa(statusCode)
	metrics.externalRequestDuration.WithLabelValues(method, endpoint, code).Observe(duration.Seconds())
	metrics.externalRequestTotal.WithLabelValues(method, endpoint, code).Inc()
}

func trackDuration(observe func(float64)) (stop func()) {
	start := time.Now()
	return func() {
		observe(time.Since(start).Seconds())
	}
}

func NewGaugeVec(subsystem, name, help string, labels []string) *prometheus.GaugeVec {
	return promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      name,
			Help:      help,
		},
		labels,
	)
}

func NewCounterVec(subsystem, name, help string, labels []string) *prometheus.CounterVec {
	return promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      name,
			Help:      help,
		},
		labels,
	)
}

func NewHistogramVec(subsystem, name, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	return promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      name,
			Help:      help,
			Buckets:   buckets,
		},
		labels,
	)
}
