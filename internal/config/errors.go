package config

import "fmt"

// UnsupportedWatcherTypeError reports an unknown watcher.type value.
type UnsupportedWatcherTypeError string

// UnsupportedDestinationTypeError reports an unknown destination.type value.
type UnsupportedDestinationTypeError string

// ErrUnsupportedWatcherType wraps an unsupported watcher type string.
func ErrUnsupportedWatcherType(watcherType string) error {
	return UnsupportedWatcherTypeError(watcherType)
}

// ErrUnsupportedDestinationType wraps an unsupported destination type string.
func ErrUnsupportedDestinationType(destinationType string) error {
	return UnsupportedDestinationTypeError(destinationType)
}

// Error implements error for UnsupportedWatcherTypeError.
func (e UnsupportedWatcherTypeError) Error() string {
	return fmt.Sprintf("unsupported watcher.type %q", string(e))
}

// Error implements error for UnsupportedDestinationTypeError.
func (e UnsupportedDestinationTypeError) Error() string {
	return fmt.Sprintf("unsupported destination.type %q", string(e))
}
