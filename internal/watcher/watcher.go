package watcher

import "context"

// Event represents one OpenBao audit event relevant to KV v2 synchronization.
//
// Ack is optional. HTTP audit events use it to keep OpenBao blocked until the
// controller has finished reconciling the event.
type Event struct {
	Mount     string
	Path      string
	Namespace string
	Operation string
	Ack       chan error
}

// Watcher streams OpenBao KV events and asynchronous watcher errors.
type Watcher interface {
	Watch(ctx context.Context) (<-chan Event, <-chan error)
}
