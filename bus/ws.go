package bus

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// wsMaxFrame matches the daemon's busWSMaxMessageSize (64 KiB).
const wsMaxFrame = 65536

// wsFrame is the bus transport over WebSocket: one JSON object per text frame.
type wsFrame struct {
	conn *websocket.Conn
}

func dialWS(ctx context.Context, url, callerToken string) (*wsFrame, error) {
	header := http.Header{}
	if callerToken != "" {
		header.Set(protocol.HeaderCallerToken, callerToken)
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, resp, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("bus: dial ws %s: %w (http %d)", url, err, resp.StatusCode)
		}
		return nil, fmt.Errorf("bus: dial ws %s: %w", url, err)
	}
	conn.SetReadLimit(wsMaxFrame)
	return &wsFrame{conn: conn}, nil
}

func (w *wsFrame) Name() string { return "ws" }

func (w *wsFrame) writeFrame(raw []byte) error {
	return w.conn.WriteMessage(websocket.TextMessage, raw)
}

func (w *wsFrame) readFrame() ([]byte, error) {
	_, data, err := w.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (w *wsFrame) close() error { return w.conn.Close() }
