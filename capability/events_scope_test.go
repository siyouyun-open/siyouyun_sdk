package capability_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/bustest"
	"github.com/siyouyun-open/siyouyun_sdk/v2/capability"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// P3: the SDK's app.Events() — subscribe + watched-scope read/write + local defensive filtering.

// scopeServer returns a fake daemon: it answers get/set with the given ceiling and effective set,
// and records the calls.
func scopeServer(t *testing.T, ceiling, scope []string, setReply func(values []any) (string, any)) *bustest.Server {
	t.Helper()
	current := append([]string(nil), scope...)
	return bustest.New(t, bustest.Options{AppID: "music", OnCall: func(req protocol.Request) (string, json.RawMessage) {
		switch string(req.Template) {
		case protocol.TemplateEventScopeGet:
			return "", mustJSON(t, map[string]any{
				"success": true, "appCode": "music", "namespace": "alice-main",
				"ceiling": ceiling, "scope": current, "updatedBy": "manifest",
				"source": "appstore",
			})
		case protocol.TemplateEventScopeSet:
			var payload struct {
				Op     string   `json:"op"`
				Values []string `json:"values"`
			}
			_ = json.Unmarshal(req.Payload, &payload)
			if setReply != nil {
				code, body := setReply(toAnySlice(payload.Values))
				if code != "" || body != nil {
					return code, mustJSON(t, body)
				}
			}
			switch payload.Op {
			case "", "set":
				current = payload.Values
			case "add":
				current = append(current, payload.Values...)
			case "remove":
				keep := make([]string, 0, len(current))
				for _, v := range current {
					drop := false
					for _, r := range payload.Values {
						if v == r {
							drop = true
						}
					}
					if !drop {
						keep = append(keep, v)
					}
				}
				current = keep
			}
			return "", mustJSON(t, map[string]any{
				"success": true, "appCode": "music", "namespace": "alice-main",
				"ceiling": ceiling, "scope": current, "updatedBy": "user",
			})
		}
		return "", nil
	}})
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func toAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

func newEventsClient(t *testing.T, srv *bustest.Server) *capability.Events {
	t.Helper()
	client := bus.New(bus.Options{WSURL: srv.URL()})
	t.Cleanup(func() { _ = client.Close() })
	return capability.New(client).Events().
		// Authorized spaces: for native they come from the mount table; the unit test provides
		// them directly. The space-axis check must use them as the baseline.
		WithSpaces(func() []string { return []string{"alice-main"} }).
		For(capability.Scope{Username: "alice", Namespace: "main"})
}

func TestEventsStatusAndScopeOps(t *testing.T) {
	srv := scopeServer(t, []string{"space"}, []string{"space"}, nil)
	events := newEventsClient(t, srv)
	ctx := context.Background()

	status, err := events.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.AppCode != "music" || status.Namespace != "alice-main" {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Ceiling) != 1 || status.Ceiling[0] != "space" {
		t.Fatalf("ceiling = %v", status.Ceiling)
	}

	// set
	scope, err := events.SetScopes(ctx, "space.music/2024")
	if err != nil {
		t.Fatalf("SetScopes: %v", err)
	}
	if len(scope) != 1 || scope[0] != "space.music/2024" {
		t.Fatalf("scope = %v", scope)
	}
	payload, ok := srv.LastCall(protocol.TemplateEventScopeSet)
	if !ok {
		t.Fatal("no set call received")
	}
	if payload["op"] != "set" {
		t.Errorf("op = %v", payload["op"])
	}
	if payload["username"] != "alice" || payload["namespace"] != "main" {
		t.Errorf("UGN must be carried with the call: %v", payload)
	}

	// add / remove use the same template with a different op
	if _, err := events.AddScopes(ctx, "space.photos"); err != nil {
		t.Fatalf("AddScopes: %v", err)
	}
	if _, err := events.RemoveScopes(ctx, "space.photos"); err != nil {
		t.Fatalf("RemoveScopes: %v", err)
	}
	ops := []string{}
	for _, req := range srv.Requests() {
		if string(req.Template) != protocol.TemplateEventScopeSet {
			continue
		}
		var p struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(req.Payload, &p)
		ops = append(ops, p.Op)
	}
	want := []string{"set", "add", "remove"}
	if len(ops) != len(want) {
		t.Fatalf("ops = %v", ops)
	}
	for i := range want {
		if ops[i] != want[i] {
			t.Fatalf("ops = %v, want %v", ops, want)
		}
	}
}

// "No values" (reset to the ceiling) and "explicit empty array" (the empty set) are two states, and
// both must be expressible.
func TestEventsSetScopesNilVersusEmpty(t *testing.T) {
	srv := scopeServer(t, []string{"space"}, []string{"space"}, nil)
	events := newEventsClient(t, srv)

	// No values = reset to the manifest ceiling.
	if _, err := events.SetScopes(context.Background()); err != nil {
		t.Fatalf("SetScopes(): %v", err)
	}
	payload, ok := srv.LastCall(protocol.TemplateEventScopeSet)
	if !ok {
		t.Fatal("no set call received")
	}
	if values, present := payload["values"]; !present || values != nil {
		t.Fatalf("passing no values should send values: null (reset to the ceiling), got present=%v values=%v", present, values)
	}

	// An explicit empty slice = the empty set.
	if _, err := events.SetScopes(context.Background(), []string{}...); err != nil {
		t.Fatalf("SetScopes([]): %v", err)
	}
	payload, _ = srv.LastCall(protocol.TemplateEventScopeSet)
	values, present := payload["values"]
	if !present {
		t.Fatal("the empty set must explicitly send values: [] (otherwise it is treated as a reset to the ceiling)")
	}
	list, ok := values.([]any)
	if !ok || len(list) != 0 {
		t.Fatalf("values = %v, want an explicit empty array", values)
	}

	// remove still sends the values to remove as-is; AppStore computes the resulting set.
	if _, err := events.RemoveScopes(context.Background(), "space.music"); err != nil {
		t.Fatalf("RemoveScopes: %v", err)
	}
	payload, _ = srv.LastCall(protocol.TemplateEventScopeSet)
	if payload["op"] != "remove" {
		t.Fatalf("op = %v", payload["op"])
	}
	if list, _ := payload["values"].([]any); len(list) != 1 || list[0] != "space.music" {
		t.Fatalf("values for remove = %v", payload["values"])
	}
}

func TestEventsSetScopesOutOfCeiling(t *testing.T) {
	// The provider may return success but carry an error message (cases other than 400), or the
	// daemon may fold it into an error frame; both must be recognized.
	srv := scopeServer(t, []string{"space.music"}, []string{"space.music"},
		func([]any) (string, any) {
			return "", map[string]any{"success": false, "error": "作用域值 \"space.movies\" 不在 manifest 的监听范围内"}
		})
	events := newEventsClient(t, srv)

	_, err := events.SetScopes(context.Background(), "space.movies")
	if !errors.Is(err, capability.ErrScopeOutOfCeiling) {
		t.Fatalf("an out-of-ceiling value must map to ErrScopeOutOfCeiling, got %v", err)
	}
}

func TestEventsUnsupportedDevice(t *testing.T) {
	// Old device: the daemon answers "no provider available" for an unknown template.
	srv := bustest.New(t, bustest.Options{AppID: "music", OnCall: func(protocol.Request) (string, json.RawMessage) {
		return protocol.CodeError, nil
	}})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()
	events := capability.New(client).Events()

	_, err := events.Ceiling(context.Background())
	if err == nil {
		t.Fatal("an old device should return an error")
	}
	// The exact error type depends on the daemon's wording; here we only require that
	// "unavailability can be distinguished", not that it is necessarily a sentinel.
	t.Logf("unsupported path error: %v", err)
}

// Local filtering: let events through before the scope has been read (the daemon is already
// filtering); once it has been read, filter by space axis + path.
func TestEventsLocalFiltering(t *testing.T) {
	srv := scopeServer(t, []string{"space"}, []string{"space.music/2024"}, nil)
	events := newEventsClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan capability.FileEvent, 8)
	sub, err := events.Subscribe(ctx, func(ev capability.FileEvent) {
		select {
		case got <- ev:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	publish := func(ufi, srcUFI, user, ns string) {
		payload := map[string]any{
			"action": 1,
			"ufi":    ufi,
			"ugn":    map[string]any{"username": user, "namespace": ns},
		}
		if srcUFI != "" {
			payload["srcUfi"] = srcUFI
			payload["action"] = 3
		}
		srv.Publish(t, protocol.TopicFileEvent, payload)
	}
	expect := func(name string, want bool) {
		t.Helper()
		select {
		case <-got:
			if !want {
				t.Fatalf("%s: must not be delivered to application code", name)
			}
		case <-time.After(300 * time.Millisecond):
			if want {
				t.Fatalf("%s: must be delivered to application code", name)
			}
		}
	}

	// Within scope
	publish("/ufi/pool-raw/syspool/Music/2024/a.mp3", "", "alice", "main")
	expect("file within scope", true)
	// Out of scope (a parent of the same key)
	publish("/ufi/pool-raw/syspool/Music/a.mp3", "", "alice", "main")
	expect("file in a parent directory", false)
	// Out of scope (a different directory), and in the same space as its own
	publish("/ufi/pool-raw/syspool/Documents/a.pdf", "", "alice", "main")
	expect("file in another business directory", false)
	// A different space: the space axis does not cover it
	publish("/ufi/pool-raw/bob_syspool/Music/2024/a.mp3", "", "bob", "main")
	expect("another user's space", false)
	// rename: a source path match must also pass
	publish("/ufi/pool-raw/syspool/Downloads/a.mp3", "/ufi/pool-raw/syspool/Music/2024/a.mp3", "alice", "main")
	expect("rename source path matched", true)
}

// native is "installed once, serves multiple spaces": each space's watched scope is independent,
// and the per-space lazy scope load happens only once (the same topic is subscribed only once, not
// once per space).
func TestEventsLocalFilteringPerSpace(t *testing.T) {
	calls := map[string]int{}
	srv := bustest.New(t, bustest.Options{AppID: "music", OnCall: func(req protocol.Request) (string, json.RawMessage) {
		if string(req.Template) != protocol.TemplateEventScopeGet {
			return "", nil
		}
		var payload struct {
			Namespace string `json:"namespace"`
		}
		_ = json.Unmarshal(req.Payload, &payload)
		calls[payload.Namespace]++
		// alice is narrowed to /Music, bob to /Photos.
		scope := "space.music"
		if strings.Contains(string(req.Payload), `"username":"bob"`) {
			scope = "space.photos"
		}
		return "", mustJSON(t, map[string]any{
			"success": true, "appCode": "music",
			"ceiling": []string{"space"}, "scope": []string{scope},
			"updatedBy": "user",
		})
	}})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()
	events := capability.New(client).Events().
		WithSpaces(func() []string { return []string{"alice-main", "bob-main"} })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan capability.FileEvent, 8)
	sub, err := events.Subscribe(ctx, func(ev capability.FileEvent) {
		select {
		case got <- ev:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	publish := func(ufi, user, ns string) {
		srv.Publish(t, protocol.TopicFileEvent, map[string]any{
			"action": 1, "ufi": ufi,
			"ugn": map[string]any{"username": user, "namespace": ns},
		})
	}
	expect := func(name string, want bool) {
		t.Helper()
		select {
		case <-got:
			if !want {
				t.Fatalf("%s: must not be delivered to application code", name)
			}
		case <-time.After(300 * time.Millisecond):
			if want {
				t.Fatalf("%s: must be delivered to application code", name)
			}
		}
	}

	// alice's space: only Music matters
	publish("/ufi/pool-raw/alice_syspool/Music/a.mp3", "alice", "main")
	expect("alice's Music", true)
	publish("/ufi/pool-raw/alice_syspool/Photos/a.jpg", "alice", "main")
	expect("alice's Photos", false)
	// bob's space: only Photos matters (same instance, different space)
	publish("/ufi/pool-raw/bob_syspool/Photos/a.jpg", "bob", "main")
	expect("bob's Photos", true)
	publish("/ufi/pool-raw/bob_syspool/Music/a.mp3", "bob", "main")
	expect("bob's Music", false)

	// Each space's scope should only be read a few times (once before subscribing + once for the
	// per-space lazy load), not once per event.
	if calls["main"] > 3 {
		t.Fatalf("scope reads should be cached per space, actual calls %d: %v", calls["main"], calls)
	}
}

// When the scope cannot be read (old device / bus flakiness), events must not all be dropped.
func TestEventsLocalFilteringFailsOpen(t *testing.T) {
	srv := bustest.New(t, bustest.Options{AppID: "music", OnCall: func(protocol.Request) (string, json.RawMessage) {
		return protocol.CodeError, nil // scope read failed
	}})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()
	events := capability.New(client).Events().
		WithSpaces(func() []string { return []string{"alice-main"} }).
		For(capability.Scope{Username: "alice", Namespace: "main"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan capability.FileEvent, 2)
	sub, err := events.Subscribe(ctx, func(ev capability.FileEvent) {
		select {
		case got <- ev:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe must not fail because the scope cannot be read: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	srv.Publish(t, protocol.TopicFileEvent, map[string]any{
		"action": 1, "ufi": "/ufi/pool-raw/syspool/Documents/a.pdf",
		"ugn": map[string]any{"username": "alice", "namespace": "main"},
	})
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("events must pass through when the watched scope cannot be read (local filtering is a defense, not the authority)")
	}
}

func TestSortScopes(t *testing.T) {
	got := capability.SortScopes([]string{"space.music", " space.photos ", "space.music", ""})
	if len(got) != 2 || got[0] != "space.music" || got[1] != "space.photos" {
		t.Fatalf("SortScopes = %v", got)
	}
}
