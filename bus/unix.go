package bus

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
)

// unixMaxFrame is the per-frame limit: the daemon-side read buffer and the WS read limit
// are both 64 KiB, so this keeps a limit of the same order of magnitude and prevents a
// misbehaving peer from blowing up local memory with a single frame.
const unixMaxFrame = 1 << 20

// unixFrame is the JSON-lines transport over a Unix Domain Socket:
// one JSON object per line, framed by '\n'.
type unixFrame struct {
	conn   net.Conn
	reader *bufio.Reader
}

func dialUnix(ctx context.Context, path string) (*unixFrame, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("bus: dial unix %s: %w", path, err)
	}
	return &unixFrame{
		conn:   conn,
		reader: bufio.NewReaderSize(conn, 64<<10),
	}, nil
}

func (u *unixFrame) Name() string { return "unix" }

func (u *unixFrame) writeFrame(raw []byte) error {
	if len(raw) > unixMaxFrame {
		return fmt.Errorf("bus: frame too large (%d bytes)", len(raw))
	}
	buf := make([]byte, 0, len(raw)+1)
	buf = append(buf, raw...)
	buf = append(buf, '\n')
	_, err := u.conn.Write(buf)
	return err
}

func (u *unixFrame) readFrame() ([]byte, error) {
	line, err := u.reader.ReadBytes('\n')
	if err != nil {
		if len(bytes.TrimSpace(line)) > 0 {
			return bytes.TrimRight(line, "\n"), nil
		}
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	if len(line) > unixMaxFrame {
		return nil, fmt.Errorf("bus: frame too large (%d bytes)", len(line))
	}
	return bytes.TrimRight(line, "\r\n"), nil
}

func (u *unixFrame) close() error { return u.conn.Close() }
