package bus

import (
	"errors"
	"fmt"
	"io"
	"net"
)

// ErrTimeout indicates that a call did not receive a response within the timeout.
// Test for it with errors.Is(err, bus.ErrTimeout).
var ErrTimeout = errors.New("bus: call timed out")

// CallError indicates that the daemon explicitly returned a failure (capability does not
// exist, invalid arguments, provider failed, and so on).
// It differs from a "connection failure": retrying the same call will not help unless the
// input is fixed first or the provider recovers.
type CallError struct {
	Template string
	Code     string
	Message  string
}

func (e *CallError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("bus: %s: %s", e.Template, e.Message)
	}
	return fmt.Sprintf("bus: %s: %s (%s)", e.Template, e.Message, e.Code)
}

// connError marks a "connection-level" failure (peer closed, write failed, connection
// discarded).
// Only this class of failure is worth retrying on a new connection.
type connError struct{ err error }

func (e *connError) Error() string { return "bus: connection error: " + e.err.Error() }
func (e *connError) Unwrap() error { return e.err }

func isConnError(err error) bool {
	if err == nil {
		return false
	}
	var ce *connError
	if errors.As(err, &ce) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne)
}
