package sync

import (
	"context"
	"errors"
)

const (
	errorTypeNone        = "none"
	errorTypeCanceled    = "canceled"
	errorTypeTimeout     = "timeout"
	errorTypeOpenBaoRead = "openbao_read"
	errorTypePolicy      = "policy"
	errorTypeTransform   = "transform"
	errorTypeDestination = "destination"
	errorTypeUnknown     = "unknown"
)

type typedError struct {
	errorType string
	err       error
}

func (e typedError) Error() string {
	return e.err.Error()
}

func (e typedError) Unwrap() error {
	return e.err
}

func (e typedError) ErrorType() string {
	return e.errorType
}

type errorTyper interface {
	ErrorType() string
}

func withErrorType(errorType string, err error) error {
	if err == nil {
		return nil
	}
	var typed errorTyper
	if errors.As(err, &typed) {
		return err
	}
	return typedError{errorType: errorType, err: err}
}

func errorType(err error) string {
	if err == nil {
		return errorTypeNone
	}
	if errors.Is(err, context.Canceled) {
		return errorTypeCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errorTypeTimeout
	}
	var typed errorTyper
	if errors.As(err, &typed) {
		return typed.ErrorType()
	}
	return errorTypeUnknown
}
