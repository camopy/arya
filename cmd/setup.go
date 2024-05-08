package main

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/camopy/rss_everything/zaplog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const monitoringServerPort = "9091"

func startMonitoringServer(logger *zaplog.Logger) {
	var mux http.ServeMux
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	}))
	logger.Info("starting metrics server at /metrics")
	logger.Info("starting health check at /health")
	if err := http.ListenAndServe(fmt.Sprintf(":%s", monitoringServerPort), &mux); err != nil {
		logger.Fatal("failed to start metrics server", zap.Error(err))
	}
}
