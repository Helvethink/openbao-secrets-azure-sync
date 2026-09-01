package sync

import (
	"context"
	"log/slog"

	"openbao-kv-azure-sync/internal/azure"
	"openbao-kv-azure-sync/internal/metrics"
	"openbao-kv-azure-sync/internal/watcher"
)

// Options wires the controller to its event source, OpenBao source, and destination.
type Options struct {
	Watcher        watcher.Watcher
	Source         Source
	SourceInstance string
	Destination    azure.Destination
	Metrics        *metrics.Collector
	Logger         *slog.Logger
}

// Controller reconciles OpenBao audit events into destination operations.
type Controller struct {
	watcher    watcher.Watcher
	reconciler *Reconciler
	logger     *slog.Logger
}

// New creates a Controller from explicit dependencies.
func New(options Options) *Controller {
	logger := defaultLogger(options.Logger)
	reconciler := NewReconciler(ReconcilerOptions{
		Source:         options.Source,
		SourceInstance: options.SourceInstance,
		Destination:    options.Destination,
		Metrics:        options.Metrics,
		Logger:         logger,
	})
	return &Controller{
		watcher:    options.Watcher,
		reconciler: reconciler,
		logger:     logger,
	}
}

// Run consumes watcher events until ctx is cancelled or the watcher fails.
func (c *Controller) Run(ctx context.Context) error {
	events, errs := c.watcher.Watch(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			return err
		case event, ok := <-events:
			if !ok {
				events = nil
				if errs == nil {
					return nil
				}
				continue
			}
			if err := c.Reconcile(ctx, event); err != nil {
				c.logger.Error("reconcile failed",
					"mount", event.Mount,
					"path", event.Path,
					"operation", event.Operation,
					"error_type", errorType(err),
					"error", err,
				)
				acknowledge(event, err)
				continue
			}
			acknowledge(event, nil)
		}
	}
}

// acknowledge unblocks a synchronous audit event after reconciliation.
func acknowledge(event watcher.Event, err error) {
	if event.Ack == nil {
		return
	}
	event.Ack <- err
}

// Reconcile applies the current OpenBao state for a single audit event.
func (c *Controller) Reconcile(ctx context.Context, event watcher.Event) error {
	return c.reconciler.Reconcile(ctx, event)
}
