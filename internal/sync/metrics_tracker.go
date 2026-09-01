package sync

import (
	"time"

	"openbao-kv-azure-sync/internal/metrics"
	"openbao-kv-azure-sync/internal/policy"
)

type reconcileMetricsTracker struct {
	metrics     *metrics.Collector
	operation   string
	targetType  string
	result      string
	failureType string
	start       time.Time
}

func newReconcileMetricsTracker(metrics *metrics.Collector, operation string) *reconcileMetricsTracker {
	return &reconcileMetricsTracker{
		metrics:     metrics,
		operation:   operation,
		targetType:  "unknown",
		result:      "success",
		failureType: errorTypeNone,
		start:       time.Now(),
	}
}

func (t *reconcileMetricsTracker) setTarget(syncMetadata policy.SyncMetadata) {
	t.targetType = syncMetadata.TargetProvider
	if syncMetadata.TargetType != "" {
		t.targetType += "/" + syncMetadata.TargetType
	}
}

func (t *reconcileMetricsTracker) fail(err error) {
	t.result = "error"
	t.failureType = errorType(err)
}

func (t *reconcileMetricsTracker) finish() {
	if t.metrics == nil {
		return
	}
	t.metrics.IncReconcile(t.operation, t.targetType, t.result, t.failureType)
	t.metrics.ObserveReconcileDuration(t.operation, t.targetType, time.Since(t.start))
}
