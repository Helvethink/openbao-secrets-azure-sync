package watcher

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

type AuditLogWatcher struct {
	path     string
	parser   auditRecordParser
	interval time.Duration
}

// NewAuditLogWatcher creates a watcher that tails an OpenBao audit log file.
func NewAuditLogWatcher(path string, mounts []string, namespaces []string, logger *slog.Logger) *AuditLogWatcher {
	return &AuditLogWatcher{
		path:     path,
		parser:   newAuditRecordParser(mounts, namespaces, logger),
		interval: time.Second,
	}
}

// Watch streams parsed events from the configured audit log until ctx is done.
func (w *AuditLogWatcher) Watch(ctx context.Context) (<-chan Event, <-chan error) {
	events := make(chan Event)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		file, err := os.Open(w.path)
		if err != nil {
			errs <- err
			return
		}
		defer file.Close()

		reader := bufio.NewReader(file)
		for {
			line, err := reader.ReadString('\n')
			if errors.Is(err, io.EOF) {
				if strings.TrimSpace(line) != "" {
					if event, ok := w.parser.parseLine(line); ok {
						select {
						case events <- event:
						case <-ctx.Done():
							return
						}
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(w.interval):
					continue
				}
			}
			if err != nil {
				errs <- err
				return
			}

			event, ok := w.parser.parseLine(line)
			if !ok {
				continue
			}

			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, errs
}
