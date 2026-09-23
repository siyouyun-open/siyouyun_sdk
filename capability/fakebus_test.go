package capability

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// fakeBusServer is the minimal bus server used by capability tests (UDS + JSON-lines).
type fakeBusServer struct {
	t        *testing.T
	path     string
	listener net.Listener

	mu    sync.Mutex
	conns []net.Conn
	reqs  []protocol.Request

	callFn func(protocol.Request) protocol.Response
}

func newFakeBusServer(t *testing.T) *fakeBusServer {
	t.Helper()
	path := filepath.Join(os.TempDir(), fmt.Sprintf("appsdk-cap-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	f := &fakeBusServer{t: t, path: path, listener: listener}
	go f.acceptLoop()
	t.Cleanup(func() {
		_ = listener.Close()
		f.mu.Lock()
		conns := append([]net.Conn(nil), f.conns...)
		f.mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
		_ = os.Remove(path)
	})
	return f
}

func (f *fakeBusServer) acceptLoop() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns = append(f.conns, conn)
		f.mu.Unlock()
		go f.serve(conn)
	}
}

func (f *fakeBusServer) serve(conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var req protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		f.mu.Lock()
		f.reqs = append(f.reqs, req)
		f.mu.Unlock()

		switch req.Type {
		case protocol.TypeSubscribe, protocol.TypeUnsubscribe:
			f.write(conn, protocol.Response{Type: protocol.FrameResponse, ID: req.ID, Code: protocol.CodeSuccess})
		case protocol.TypeCall:
			resp := protocol.Response{Type: protocol.FrameResponse, ID: req.ID, Code: protocol.CodeSuccess}
			if f.callFn != nil {
				resp = f.callFn(req)
				resp.Type = protocol.FrameResponse
				resp.ID = req.ID
			}
			f.write(conn, resp)
		}
	}
}

func (f *fakeBusServer) write(conn net.Conn, resp protocol.Response) {
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(raw, '\n'))
}

func (f *fakeBusServer) pushEvent(topic string, payload any) {
	raw, _ := json.Marshal(payload)
	resp := protocol.Response{Type: protocol.FrameEvent, ID: "e", Topic: topic, Event: raw}
	f.mu.Lock()
	conns := append([]net.Conn(nil), f.conns...)
	f.mu.Unlock()
	for _, conn := range conns {
		f.write(conn, resp)
	}
}

func (f *fakeBusServer) Requests() []protocol.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]protocol.Request(nil), f.reqs...)
}
