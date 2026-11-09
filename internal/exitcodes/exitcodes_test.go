package exitcodes

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestDetermine(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		err      error
		expected int
	}{
		{
			name:     "nil error is success",
			ctx:      context.Background(),
			err:      nil,
			expected: ExitSuccess,
		},
		{
			name:     "generic error is general error",
			ctx:      context.Background(),
			err:      errors.New("something went wrong"),
			expected: ExitGeneralError,
		},
		{
			name:     "coded error upgrade failure",
			ctx:      context.Background(),
			err:      New(ExitUpgradeFailure, "package failed"),
			expected: ExitUpgradeFailure,
		},
		{
			name:     "wrapped coded error",
			ctx:      context.Background(),
			err:      fmt.Errorf("outer: %w", New(ExitUpgradeFailure, "inner")),
			expected: ExitUpgradeFailure,
		},
		{
			name:     "context canceled in err",
			ctx:      context.Background(),
			err:      context.Canceled,
			expected: ExitCanceled,
		},
		{
			name: "context canceled in context",
			ctx: func() context.Context {
				c, cancel := context.WithCancel(context.Background())
				cancel()
				return c
			}(),
			err:      errors.New("interrupted"),
			expected: ExitCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Determine(tt.ctx, tt.err)
			if got != tt.expected {
				t.Errorf("Determine() = %d, expected %d", got, tt.expected)
			}
		})
	}
}

func TestCodedError(t *testing.T) {
	baseErr := errors.New("underlying issue")
	wrapped := Wrap(ExitUpgradeFailure, baseErr)

	if wrapped.Error() != "underlying issue" {
		t.Errorf("expected Error() 'underlying issue', got %q", wrapped.Error())
	}

	var coder ExitCoder
	if !errors.As(wrapped, &coder) {
		t.Fatalf("expected wrapped error to implement ExitCoder")
	}
	if coder.ExitCode() != ExitUpgradeFailure {
		t.Errorf("expected ExitCode %d, got %d", ExitUpgradeFailure, coder.ExitCode())
	}

	if !errors.Is(wrapped, baseErr) {
		t.Errorf("expected errors.Is to match baseErr via Unwrap")
	}

	if Wrap(ExitGeneralError, nil) != nil {
		t.Errorf("expected Wrap(code, nil) to be nil")
	}
}
