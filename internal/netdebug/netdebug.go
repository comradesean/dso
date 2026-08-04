// Package netdebug wraps a net.Conn to hexdump everything read and written,
// for reverse-engineering an unknown client's wire format. It makes no
// assumptions about framing: it logs the raw bytes exactly as they arrive,
// which is how we confirm empirically whether the PS3 client encrypts its
// login handshake (a ~256-byte RSA block) or sends plaintext.
package netdebug

import (
	"encoding/hex"
	"log/slog"
	"net"
)

// Conn is a net.Conn that logs raw I/O.
type Conn struct {
	net.Conn
	log *slog.Logger
}

// Wrap returns a logging wrapper around c.
func Wrap(c net.Conn, log *slog.Logger) *Conn {
	return &Conn{Conn: c, log: log}
}

func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.log.Info("raw recv", "bytes", n, "peer", c.RemoteAddr().String())
		c.dump(b[:n])
	}
	return n, err
}

func (c *Conn) Write(b []byte) (int, error) {
	if len(b) > 0 {
		c.log.Info("raw send", "bytes", len(b), "peer", c.RemoteAddr().String())
		c.dump(b)
	}
	return c.Conn.Write(b)
}

func (c *Conn) dump(b []byte) {
	// Cap the dump so a large payload doesn't flood the log.
	const max = 512
	d := b
	truncated := false
	if len(d) > max {
		d = d[:max]
		truncated = true
	}
	c.log.Info("hexdump\n" + hex.Dump(d))
	if truncated {
		c.log.Info("hexdump truncated", "shown", max, "total", len(b))
	}
}
