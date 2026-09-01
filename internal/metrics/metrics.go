package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Collector stores the small set of Prometheus-style metrics exposed by the controller.
type Collector struct {
	mu       sync.Mutex
	counters map[string]float64
}

// NewCollector creates an empty in-memory metrics collector.
func NewCollector() *Collector {
	return &Collector{counters: make(map[string]float64)}
}

// IncReconcile records one reconciliation attempt by operation, target, result,
// and normalized error type.
func (c *Collector) IncReconcile(operation, target, result, errorType string) {
	c.add("openbao_sync_reconcile_total", 1, "operation", operation, "target", target, "result", result, "error_type", errorType)
}

// ObserveReconcileDuration records reconciliation duration as count and sum.
func (c *Collector) ObserveReconcileDuration(operation, target string, duration time.Duration) {
	c.add("openbao_sync_reconcile_duration_seconds_count", 1, "operation", operation, "target", target)
	c.add("openbao_sync_reconcile_duration_seconds_sum", duration.Seconds(), "operation", operation, "target", target)
}

// IncDestination records one destination operation by provider, operation,
// result, and normalized error type.
func (c *Collector) IncDestination(provider, operation, result, errorType string) {
	c.add("openbao_sync_destination_operations_total", 1, "provider", provider, "operation", operation, "result", result, "error_type", errorType)
}

// ServeHTTP exposes metrics in the Prometheus text exposition format.
func (c *Collector) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = response.Write([]byte(c.Render()))
}

// Render returns all metrics in deterministic order.
func (c *Collector) Render() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.counters))
	for key := range c.counters {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprintf("%g", c.counters[key]))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func (c *Collector) add(name string, value float64, labels ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counters[metricKey(name, labels...)] += value
}

func metricKey(name string, labels ...string) string {
	if len(labels) == 0 {
		return name
	}
	pairs := make([]string, 0, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		pairs = append(pairs, labels[i]+`="`+escapeLabel(labels[i+1])+`"`)
	}
	sort.Strings(pairs)
	return name + "{" + strings.Join(pairs, ",") + "}"
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}
