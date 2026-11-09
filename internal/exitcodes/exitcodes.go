package exitcodes

import (
	"context"
	"errors"
)

// Standard CLI exit codes for Patchwork.
const (
	// ExitSuccess indicates all operations completed successfully.
	ExitSuccess = 0

	// ExitGeneralError indicates an operational or user error (e.g. invalid flags,
	// missing arguments, corrupt configuration file, or system error).
	ExitGeneralError = 1

	// ExitUpgradeFailure indicates that the upgrade command executed, but one or more
	// packages failed to upgrade.
	ExitUpgradeFailure = 2

	// ExitCanceled indicates that the operation was canceled or interrupted by the user
	// (e.g. SIGINT, Ctrl+C, or prompt abortion).
	ExitCanceled = 3
)

// ExitCoder is an interface implemented by errors that specify a process exit code.
type ExitCoder interface {
	ExitCode() int
}

// CodedError pairs a standard error with a specific CLI exit code.
type CodedError struct {
	Code int
	Err  error
}

func (e *CodedError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return ""
}

func (e *CodedError) Unwrap() error {
	return e.Err
}

func (e *CodedError) ExitCode() int {
	return e.Code
}

// New creates a new CodedError with the specified code and message.
func New(code int, msg string) error {
	return &CodedError{Code: code, Err: errors.New(msg)}
}

// Wrap wraps an existing error with the specified exit code.
func Wrap(code int, err error) error {
	if err == nil {
		return nil
	}
	return &CodedError{Code: code, Err: err}
}

// Determine extracts the appropriate exit code from an error and context.
func Determine(ctx context.Context, err error) int {
	if err == nil {
		return ExitSuccess
	}
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return ExitCanceled
	}
	if errors.Is(err, context.Canceled) {
		return ExitCanceled
	}
	var coder ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return ExitGeneralError
}
