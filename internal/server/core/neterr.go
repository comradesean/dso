package core

import (
	"errors"
	"io"
	"net"
	"os"
)

// IsBenignDisconnect reports whether err is an ordinary end-of-connection
// rather than a protocol or crypto failure.
//
// This distinction matters for log levels. A client that finishes and hangs up,
// or that is still sitting idle when its read deadline expires, is routine and
// belongs at Debug. Anything else reaching the same code path — a failed CWC tag
// check, a malformed packet, an unexpected message type — is a real failure that
// should be visible at the default log level. Logging both at Debug once hid a
// tag-verification failure behind an info-level run, which cost a debugging
// round to find.
func IsBenignDisconnect(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	// A read/write deadline or a peer reset surfaces as a net.Error timeout.
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}
