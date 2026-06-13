package cli

import (
	"errors"
	"fmt"
	"io"
)

// printErr writes a formatted error to w and returns it so callers can return
// a non-nil error to Cobra (which prints nothing, since SilenceErrors=true).
func printErr(w io.Writer, format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	_, _ = fmt.Fprintln(w, "error:", err)
	return err
}

// isUsageError is a sentinel so RunE can distinguish "bad args" from runtime
// errors and print usage accordingly.
type usageError struct{ error }

func wrapUsage(err error) error { return usageError{err} }

func isUsage(err error) bool {
	var u usageError
	return errors.As(err, &u)
}
