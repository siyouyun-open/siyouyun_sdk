package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/bustest"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

func startTestNATS(t *testing.T) (string, func()) {
	t.Helper()
	opts := &server.Options{
		Host: "127.0.0.1", Port: -1,
		JetStream: true, StoreDir: t.TempDir(),
		NoLog: true, NoSigs: true,
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("embedded nats not ready")
	}
	return srv.ClientURL(), func() { srv.Shutdown() }
}

func TestStartFileEventListenerEndToEnd(t *testing.T) {
	natsURL, shutdown := startTestNATS(t)
	defer shutdown()

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "SYY_INTEREST_SHARED_STREAM",
		Subjects:  []string{"os.fs.event"},
		Retention: jetstream.InterestPolicy,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "interest_lst-test",
		FilterSubject: "os.fs.event",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	}); err != nil {
		t.Fatal(err)
	}

	const listenerID = "lst-test"
	srv := bustest.New(t, bustest.Options{
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestListen:
				data, _ := json.Marshal(interestListenResponse{
					ListenerID: listenerID, Consumer: "interest_lst-test",
					Stream: "SYY_INTEREST_SHARED_STREAM", Subject: "os.fs.event",
					NATSURL: natsURL,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestStop, protocol.TemplateInterestRenew:
				data, _ := json.Marshal(map[string]any{"ok": true})
				return protocol.CodeSuccess, data
			default:
				return "", nil
			}
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()

	got := make(chan FileEvent, 1)
	listener, err := StartFileEventListener(ctx, client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, NATSURL: natsURL,
	}, func(ev FileEvent) error {
		got <- ev
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	time.Sleep(300 * time.Millisecond) // let Fetch start

	payload := []byte(`{"v":2,"ufi":"/ufi/pool-raw/syspool/Photos/a.jpg","action":1,"inode":1,"ugn":{"username":"alice","groupName":"","namespace":"main"},"matched":{"apps":["album"],"listeners":["lst-test"]}}`)
	if _, err := js.Publish(ctx, "os.fs.event", payload); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-got:
		if ev.UFI != "/ufi/pool-raw/syspool/Photos/a.jpg" || ev.Action != 1 {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not receive the event")
	}
}

func TestFileEventListenerRecoversAfterRenewFailure(t *testing.T) {
	natsURL, shutdown := startTestNATS(t)
	defer shutdown()
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: "SYY_INTEREST_SHARED_STREAM", Subjects: []string{"os.fs.event"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: "interest_lst-recover", FilterSubject: "os.fs.event",
		AckPolicy: jetstream.AckExplicitPolicy, DeliverPolicy: jetstream.DeliverNewPolicy,
	}); err != nil {
		t.Fatal(err)
	}
	var listens atomic.Int32
	srv := bustest.New(t, bustest.Options{
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestListen:
				n := listens.Add(1)
				data, _ := json.Marshal(interestListenResponse{
					ListenerID: fmt.Sprintf("lst-%d", n), Consumer: "interest_lst-recover",
					Stream: "SYY_INTEREST_SHARED_STREAM", Subject: "os.fs.event", NATSURL: natsURL,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestRenew:
				return protocol.CodeError, nil
			case protocol.TemplateInterestStop:
				return protocol.CodeSuccess, json.RawMessage(`{}`)
			}
			return "", nil
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()
	listener, err := StartFileEventListener(context.Background(), client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, NATSURL: natsURL, TTL: 300 * time.Millisecond,
	}, func(FileEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && listens.Load() < 2 {
		time.Sleep(50 * time.Millisecond)
	}
	if got := listens.Load(); got < 2 {
		t.Fatalf("renew failure should trigger a new listen, got %d listens", got)
	}
}
