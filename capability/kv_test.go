package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

func testClient(f *fakeBusServer) *bus.Client {
	return bus.New(bus.Options{
		SocketPath:  f.path,
		CallTimeout: 3 * time.Second,
		DialTimeout: time.Second,
		MinBackoff:  20 * time.Millisecond,
	})
}

// The daemon's KV capability passes the Core response body through directly, so the data of a
// successful response still wraps a {code,data,msg} envelope; the SDK must unwrap it, otherwise the
// caller receives the envelope instead of the business value.
func TestUserKVUnwrapsCoreEnvelope(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return protocol.Response{
			Code: protocol.CodeSuccess,
			Data: json.RawMessage(`{"code":"A00200","msg":"success","data":{"type":"config","key":"theme","value":"dark","found":true}}`),
		}
	}

	client := testClient(fake)
	defer client.Close()

	value, found, err := NewKV(client).User(Scope{Username: "alice", Namespace: "main"}).Get(context.Background(), "config", "theme")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || value.Key != "theme" || value.Value != "dark" {
		t.Fatalf("unexpected value: %#v found=%v", value, found)
	}
}

// native must pass the user scope explicitly: the connection namespace is always default, so the
// user cannot be derived.
func TestUserKVSendsExplicitScope(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return protocol.Response{Code: protocol.CodeSuccess, Data: json.RawMessage(`{"code":"A00200","data":{"ok":true}}`)}
	}
	client := testClient(fake)
	defer client.Close()

	kv := NewKV(client)
	if err := kv.User(Scope{Username: "alice", GroupName: "team", Namespace: "common"}).
		Put(context.Background(), "config", "theme", "dark"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0].Template != protocol.TemplateKVPut {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
	var payload map[string]string
	if err := json.Unmarshal(reqs[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"username": "alice", "groupName": "team", "namespace": "common", "type": "config", "key": "theme", "value": "dark"}
	for k, v := range want {
		if payload[k] != v {
			t.Fatalf("payload[%s]=%q, want %q (%#v)", k, payload[k], v, payload)
		}
	}

	// An empty scope sends no identity fields: in the container case the daemon derives them from
	// the connection.
	requestsBefore := len(fake.Requests())
	if _, _, err := kv.User(Scope{}).Get(context.Background(), "config", "theme"); err != nil {
		t.Fatalf("container-scope Get: %v", err)
	}
	var containerPayload map[string]any
	_ = json.Unmarshal(fake.Requests()[requestsBefore].Payload, &containerPayload)
	for _, forbidden := range []string{"username", "groupName", "namespace"} {
		if _, ok := containerPayload[forbidden]; ok {
			t.Fatalf("empty scope must not send %q: %#v", forbidden, containerPayload)
		}
	}
}

// App-dimension KV carries no user identity fields and uses the .sys. template family.
func TestAppKVUsesSysTemplatesWithoutScope(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return protocol.Response{Code: protocol.CodeSuccess, Data: json.RawMessage(`{"code":"A00200","data":{"values":[{"key":"a","value":"1"}]}}`)}
	}
	client := testClient(fake)
	defer client.Close()

	values, err := NewKV(client).App().List(context.Background(), "config")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(values) != 1 || values[0].Key != "a" {
		t.Fatalf("unexpected values: %#v", values)
	}

	req := fake.Requests()[0]
	if req.Template != protocol.TemplateKVSysList {
		t.Fatalf("unexpected template %q", req.Template)
	}
	var payload map[string]any
	_ = json.Unmarshal(req.Payload, &payload)
	if _, ok := payload["username"]; ok {
		t.Fatalf("app-scoped KV must not carry user identity: %#v", payload)
	}
}

// An upstream failure code must become an error, not be silently swallowed as empty data.
func TestEnvelopeFailureBecomesError(t *testing.T) {
	if _, err := unwrapResult(json.RawMessage(`{"code":"A00400","msg":"boom"}`)); err == nil {
		t.Fatal("expected error for upstream failure code")
	}
	// A non-envelope shape (provider-specific JSON) is returned as-is.
	raw := json.RawMessage(`{"items":[1,2]}`)
	got, err := unwrapResult(raw)
	if err != nil || string(got) != string(raw) {
		t.Fatalf("non-envelope must pass through: %s err=%v", got, err)
	}
	if got, err := unwrapResult(nil); err != nil || got != nil {
		t.Fatalf("nil must stay nil: %s err=%v", got, err)
	}
}

// All the historical shapes of a list response must decode.
func TestDecodeListShapes(t *testing.T) {
	cases := []string{
		`{"code":"A00200","data":[{"key":"a"}]}`,
		`{"code":"A00200","data":{"values":[{"key":"a"}]}}`,
		`{"code":"A00200","data":{"list":[{"key":"a"}]}}`,
		`{"code":"A00200","data":{"items":[{"key":"a"}]}}`,
	}
	for _, body := range cases {
		values, err := decodeList(json.RawMessage(body), nil)
		if err != nil {
			t.Fatalf("decodeList(%s): %v", body, err)
		}
		if len(values) != 1 || values[0].Key != "a" {
			t.Fatalf("decodeList(%s) = %#v", body, values)
		}
	}
}

// File event subscription: handler receives the decoded event body.
func TestSubscribeFileEventsDecodes(t *testing.T) {
	fake := newFakeBusServer(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return protocol.Response{Code: protocol.CodeSuccess}
	}
	client := testClient(fake)
	defer client.Close()

	received := make(chan FileEvent, 1)
	sub, err := SubscribeFileEvents(context.Background(), client, func(event FileEvent) {
		received <- event
	})
	if err != nil {
		t.Fatalf("SubscribeFileEvents: %v", err)
	}
	defer sub.Close()

	// It subscribes to the base topic; the daemon is responsible for expanding it to the caller's
	// own scope.
	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0].Type != protocol.TypeSubscribe ||
		len(reqs[0].Topics) != 1 || reqs[0].Topics[0] != protocol.TopicFileEvent {
		t.Fatalf("unexpected subscribe request: %#v", reqs)
	}

	fake.pushEvent(protocol.TopicFileEvent+".com.example.app", map[string]any{
		"action": 1, "actionName": "add", "ufi": "/ufi/pool-raw/syspool/a.txt",
	})

	select {
	case event := <-received:
		if event.Action != 1 || event.ActionName != "add" || event.UFI == "" {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for file event")
	}
}

// User/app-dimension KV must **unwrap the envelope and check the upstream error code**.
//
// This locks down a fail-open defect: KV's call once used bus.Client.CallInto, which looks only at
// the bus-layer code, while a daemon proxying to Core/OS capabilities puts the upstream
// {code,data,msg} into data verbatim. The result was that when Core clearly failed (A00500) Put
// silently succeeded and the subsequent Get reported "not found" — on a real-device regression this
// showed up as "the KV write succeeded but cannot be read back", and was extremely costly to
// troubleshoot.
func TestKVSurfacesUpstreamEnvelopeError(t *testing.T) {
	const upstreamMsg = `ERROR: relation "siyou_kv" does not exist (SQLSTATE 42P01)`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The daemon's HTTP capability channel: the bus layer succeeds and data holds Core's error
		// envelope.
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"result":{"code":"A00500","data":null,"msg":%q}}`, upstreamMsg)
	}))
	defer server.Close()

	client := bus.New(bus.Options{HTTPURL: server.URL, CallTimeout: 3 * time.Second})
	defer client.Close()
	kv := NewKV(client)

	// Put: a write failure must report an error rather than silently succeed.
	err := kv.User(Scope{Username: "alice", Namespace: "main"}).Put(context.Background(), "t", "k", "v")
	if err == nil {
		t.Fatal("user KV Put must surface the upstream error")
	}
	if !strings.Contains(err.Error(), "siyou_kv") {
		t.Fatalf("error should carry the upstream message, got %v", err)
	}

	// Same for the app dimension.
	if err := kv.App().Put(context.Background(), "t", "k", "v"); err == nil {
		t.Fatal("app KV Put must surface the upstream error")
	}
}
