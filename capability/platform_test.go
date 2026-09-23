package capability

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// envelopeOK wraps a Core/OS envelope to simulate the daemon passing it through verbatim.
func envelopeOK(t *testing.T, data string) protocol.Response {
	t.Helper()
	return protocol.Response{
		Code: protocol.CodeSuccess,
		Data: json.RawMessage(`{"code":"A00200","msg":"ok","data":` + data + `}`),
	}
}

func lastPayload(t *testing.T, fake *fakeBusServer) map[string]any {
	t.Helper()
	reqs := fake.Requests()
	if len(reqs) == 0 {
		t.Fatal("no request captured")
	}
	var payload map[string]any
	if err := json.Unmarshal(reqs[len(reqs)-1].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func TestMediaFFmpegInfo(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(req protocol.Request) protocol.Response {
		if req.Template != protocol.TemplateMediaFFmpegInfo {
			t.Fatalf("unexpected template %q", req.Template)
		}
		return envelopeOK(t, `{"format":{"duration":"12.5","size":"1024","bit_rate":"640000"},"streams":[{"index":0,"codec_type":"video"}]}`)
	}
	client := testClient(fake)
	defer client.Close()

	info, err := NewMedia(client).FFmpegInfo(context.Background(),
		Scope{Username: "alice", Namespace: "main"}, "/ufi/pool-raw/syspool/a.mp4")
	if err != nil {
		t.Fatalf("FFmpegInfo: %v", err)
	}
	if info.Format.Duration != "12.5" || len(info.Streams) != 1 || info.Streams[0].CodecType != "video" {
		t.Fatalf("unexpected info: %#v", info)
	}

	payload := lastPayload(t, fake)
	if payload["ufi"] != "/ufi/pool-raw/syspool/a.mp4" {
		t.Fatalf("ufi not sent: %#v", payload)
	}
	if payload["username"] != "alice" || payload["namespace"] != "main" {
		t.Fatalf("scope not sent: %#v", payload)
	}
}

func TestDeviceServiceEndpoints(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return envelopeOK(t, `{"endpoints":{"docker":{"endpoint":"127.0.0.1:2375"},"share":{"endpoint":"127.0.0.1:40100","hostname":"nas"}},"refresh":{"intervalSeconds":300}}`)
	}
	client := testClient(fake)
	defer client.Close()

	eps, err := NewDevice(client).ServiceEndpoints(context.Background())
	if err != nil {
		t.Fatalf("ServiceEndpoints: %v", err)
	}
	if eps.Endpoints.Docker.Endpoint != "127.0.0.1:2375" || eps.Endpoints.Share.Hostname != "nas" || eps.Refresh.IntervalSeconds != 300 {
		t.Fatalf("unexpected endpoints: %#v", eps)
	}
}

func TestStorageUserSpacesAndTrash(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(req protocol.Request) protocol.Response {
		switch req.Template {
		case protocol.TemplateStorageUserSpaces:
			return envelopeOK(t, `[{"namespace":"team","namespaceAlias":"Team","poolName":"team-pool","quota":1024}]`)
		default:
			return envelopeOK(t, `{}`)
		}
	}
	client := testClient(fake)
	defer client.Close()
	storage := NewStorage(client)
	scope := Scope{Username: "alice", Namespace: "main"}

	spaces, err := storage.UserSpaces(context.Background(), scope)
	if err != nil {
		t.Fatalf("UserSpaces: %v", err)
	}
	if len(spaces) != 1 || spaces[0].Namespace != "team" || spaces[0].Quota != 1024 {
		t.Fatalf("unexpected spaces: %#v", spaces)
	}

	if err := storage.Trash(context.Background(), scope, "/ufi/pool-raw/syspool/x.png"); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	reqs := fake.Requests()
	last := reqs[len(reqs)-1]
	if last.Template != protocol.TemplateFSTrash {
		t.Fatalf("unexpected template %q", last.Template)
	}
	if payload := lastPayload(t, fake); payload["ufi"] != "/ufi/pool-raw/syspool/x.png" {
		t.Fatalf("trash ufi not sent: %#v", payload)
	}
}

func TestMessageSend(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(protocol.Request) protocol.Response { return envelopeOK(t, `{}`) }
	client := testClient(fake)
	defer client.Close()

	err := NewMessage(client).Send(context.Background(), Scope{Username: "alice", Namespace: "main"}, "hi", "uuid-1")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	payload := lastPayload(t, fake)
	if payload["content"] != "hi" || payload["replyUUID"] != "uuid-1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

// Scheduled task inputs contain integers: payload must be able to carry non-string values.
func TestScheduleCreateAndUpdate(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(req protocol.Request) protocol.Response {
		if req.Template == protocol.TemplateScheduleOnceCreate ||
			req.Template == protocol.TemplateScheduleCronCreate {
			return envelopeOK(t, `42`)
		}
		return envelopeOK(t, `{}`)
	}
	client := testClient(fake)
	defer client.Close()
	schedule := NewSchedule(client)
	scope := Scope{Username: "alice", Namespace: "main"}

	eventID, err := schedule.AddOnce(context.Background(), scope, OnceSpec{
		Name: "cleanup", Payload: `{"deep":1}`, RemindTime: 1893456000000,
	})
	if err != nil || eventID != 42 {
		t.Fatalf("AddOnce = %d err=%v", eventID, err)
	}
	payload := lastPayload(t, fake)
	if payload["name"] != "cleanup" || payload["payload"] != `{"deep":1}` {
		t.Fatalf("unexpected once payload: %#v", payload)
	}
	if got, ok := payload["remindTime"].(float64); !ok || int64(got) != 1893456000000 {
		t.Fatalf("remindTime not sent as a number: %#v", payload["remindTime"])
	}

	cronID, err := schedule.AddCron(context.Background(), scope, CronSpec{Name: "nightly", Cron: "0 3 * * *"})
	if err != nil || cronID != 42 {
		t.Fatalf("AddCron = %d err=%v", cronID, err)
	}
	if payload := lastPayload(t, fake); payload["cron"] != "0 3 * * *" {
		t.Fatalf("cron not sent: %#v", payload)
	}

	if err := schedule.UpdateOnce(context.Background(), scope, 7, 1893456000000); err != nil {
		t.Fatalf("UpdateOnce: %v", err)
	}
	if payload := lastPayload(t, fake); int64(payload["eventId"].(float64)) != 7 {
		t.Fatalf("eventId not sent as a number: %#v", payload["eventId"])
	}
	if err := schedule.UpdateCron(context.Background(), scope, 8, "0 4 * * *"); err != nil {
		t.Fatalf("UpdateCron: %v", err)
	}
}
