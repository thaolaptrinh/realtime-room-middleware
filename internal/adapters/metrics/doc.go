// Package metrics defines the adapter boundary for runtime metrics collection.
//
// Future implementations:
//
//   - LogMetrics: writes metrics to structured logs (single-vps default).
//   - PrometheusMetrics: exposes a /metrics endpoint for Prometheus scrape
//     (distributed-k3s default).
//
// The core runtime calls metrics interfaces defined in internal/observability.
// Adapter implementations in this package bridge those interfaces to concrete
// backends. Single-vps mode should not import any Prometheus dependency from
// this package at runtime.
package metrics
