package watchdirs

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kataras/iris/v12"

	"github.com/siyouyun-open/siyouyun_sdk/v2"
	"github.com/siyouyun-open/siyouyun_sdk/v2/bustest"
	"github.com/siyouyun-open/siyouyun_sdk/v2/capability"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Acceptance criteria for watchdirs: the user sees dirs, the backend stores scope values.

type capServerOptions struct {
	ceiling []string
	scope   []string
	failGet bool
}

// newCapServer starts a fake daemon that handles the watch scope get/set and records the
// arguments of each set.
func newCapServer(t *testing.T, opts capServerOptions) (*bustest.Server, *[]map[string]any) {
	t.Helper()
	current := append([]string(nil), opts.scope...)
	sets := &[]map[string]any{}
	srv := bustest.New(t, bustest.Options{AppID: "music", OnCall: func(req protocol.Request) (string, json.RawMessage) {
		switch string(req.Template) {
		case protocol.TemplateEventScopeGet:
			if opts.failGet {
				return protocol.CodeError, nil
			}
			return "", mustJSON(t, map[string]any{
				"success": true, "appCode": "music", "namespace": "alice-main",
				"ceiling": opts.ceiling, "scope": current, "updatedBy": "user",
			})
		case protocol.TemplateEventScopeSet:
			var payload map[string]any
			_ = json.Unmarshal(req.Payload, &payload)
			*sets = append(*sets, payload)
			if values, ok := payload["values"].([]any); ok {
				current = make([]string, 0, len(values))
				for _, v := range values {
					current = append(current, v.(string))
				}
			}
			return "", mustJSON(t, map[string]any{
				"success": true, "appCode": "music", "namespace": "alice-main",
				"ceiling": opts.ceiling, "scope": current, "updatedBy": "user",
			})
		}
		return "", nil
	}})
	return srv, sets
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// newRouter assembles the App + iris router.
func newRouter(t *testing.T, srv *bustest.Server, defaults ...string) *iris.Application {
	t.Helper()
	app := appsdk.NewWithOptions(appsdk.FromEnv(), appsdk.WithBusWS(srv.URL()))
	t.Cleanup(func() { _ = app.Close() })
	handler := New(app, Options{
		Scope: func(iris.Context) (capability.Scope, bool) {
			return capability.Scope{Username: "alice", Namespace: "main"}, true
		},
		Defaults: defaults,
		Logger:   log.New(io.Discard, "", 0),
	})
	router := iris.New()
	handler.Register(router, "/v2/watch")
	if err := router.Build(); err != nil {
		t.Fatalf("build router: %v", err)
	}
	return router
}

// do issues one request and returns the status code and the envelope data.
//
// A non-JSON response (for example the 404 "Not Found") is not a test failure: the caller
// often wants to assert that this route does not exist, and then there is no envelope to
// decode.
func do(t *testing.T, router *iris.Application, method, path string, body any) (int, json.RawMessage, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var envelope struct {
		Code string          `json:"code"`
		Data json.RawMessage `json:"data"`
		Msg  string          `json:"msg"`
	}
	if rec.Body.Len() > 0 && strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("response is not JSON: %s", rec.Body.String())
		}
	}
	return rec.Code, envelope.Data, envelope.Msg
}

func decodeDirs(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var dirs []string
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &dirs); err != nil {
		t.Fatalf("data is not a directory array: %s", string(raw))
	}
	return dirs
}

// The route mount point is determined by prefix: `Register(party, "/watch")` must land on
// `/watch/dirs`.
//
// This pins down a pitfall hit on a real device: during the migration prefix was passed as
// "", so the route became `/dirs`, and the frontend's request for `/watch/dirs` was swallowed
// by the SPA fallback, returning an HTML page instead of JSON.
func TestRegisterMountsExpectedPaths(t *testing.T) {
	srv, _ := newCapServer(t, capServerOptions{ceiling: []string{"space.music"}, scope: []string{"space.music"}})
	app := newRouter(t, srv, "/Music")

	// Paths outside prefix must not be mounted: passing prefix as "" during the migration
	// turns the route into /dirs, so the frontend's /watch/dirs falls through to the SPA
	// fallback (hit on a real device).
	if code, _, _ := do(t, app, http.MethodGet, "/v2/dirs", nil); code == http.StatusOK {
		t.Fatal("/v2/dirs must not be routed: the prefix decides")
	}
	if code, _, _ := do(t, app, http.MethodGet, "/v2/watch/dirs", nil); code != http.StatusOK {
		t.Fatalf("GET /v2/watch/dirs must be routed (prefix=\"/v2/watch\"), got %d", code)
	}
}

func TestGetReturnsDirsFromScopes(t *testing.T) {
	srv, _ := newCapServer(t, capServerOptions{ceiling: []string{"space.music"}, scope: []string{"space.music/2024"}})
	router := newRouter(t, srv, "/Music")

	code, data, msg := do(t, router, http.MethodGet, "/v2/watch/dirs", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d (%s)", code, msg)
	}
	dirs := decodeDirs(t, data)
	if len(dirs) != 1 || dirs[0] != "/Music/2024" {
		t.Fatalf("dirs = %v", dirs)
	}
}

func TestGetDegradesWhenCapUnavailable(t *testing.T) {
	srv, _ := newCapServer(t, capServerOptions{failGet: true})
	router := newRouter(t, srv, "/Music")

	code, data, _ := do(t, router, http.MethodGet, "/v2/watch/dirs", nil)
	if code != http.StatusOK {
		t.Fatalf("an unavailable capability must not fail the request, status = %d", code)
	}
	dirs := decodeDirs(t, data)
	if len(dirs) != 1 || dirs[0] != "/Music" {
		t.Fatalf("must fall back to the default directories, dirs = %v", dirs)
	}
}

func TestSetConvertsDirsToScopeValues(t *testing.T) {
	srv, sets := newCapServer(t, capServerOptions{ceiling: []string{"space.music"}, scope: []string{"space"}})
	router := newRouter(t, srv, "/Music")

	cases := []struct {
		name string
		body any
		want any // nil = JSON null; []string{} = the empty set
	}{
		{"directory", map[string]any{"dirs": []string{"/Music/2024"}}, []string{"space.music/2024"}},
		{"legacy UFI-shaped input", map[string]any{"dirs": []string{"/ufi/pool-raw/syspool/Music"}}, []string{"space.music"}},
		{"whole space", map[string]any{"dirs": []string{"/"}}, []string{"space"}},
		{"scope value passed through", map[string]any{"scopes": []string{"space.music/2024"}}, []string{"space.music/2024"}},
		{"explicit empty array = empty set", map[string]any{"dirs": []string{}}, []string{}},
		{"explicit null = back to the ceiling", map[string]any{"dirs": nil}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, msg := do(t, router, http.MethodPost, "/v2/watch/dirs", tc.body)
			if code != http.StatusOK {
				t.Fatalf("status = %d (%s)", code, msg)
			}
			payload := (*sets)[len(*sets)-1]
			got, present := payload["values"]
			if !present {
				t.Fatalf("values must be sent explicitly (null and [] mean different things): %v", payload)
			}
			switch want := tc.want.(type) {
			case nil:
				if got != nil {
					t.Fatalf("values = %v, want null", got)
				}
			case []string:
				list, _ := got.([]any)
				if len(list) != len(want) {
					t.Fatalf("values = %v, want %v", got, want)
				}
				for i := range want {
					if list[i] != want[i] {
						t.Fatalf("values = %v, want %v", got, want)
					}
				}
			}
		})
	}
}

func TestSetRejectsUnknownDirAndMissingFields(t *testing.T) {
	srv, sets := newCapServer(t, capServerOptions{ceiling: []string{"space"}, scope: []string{"space"}})
	router := newRouter(t, srv)

	// Not in the platform's registered dir table: return the available dirs to the UI along
	// with the error.
	code, _, msg := do(t, router, http.MethodPost, "/v2/watch/dirs", map[string]any{"dirs": []string{"/NotADir"}})
	if code != http.StatusOK {
		t.Fatalf("a business error must travel in the envelope, not as an HTTP status, status=%d", code)
	}
	if msg == "" || !contains(msg, "unsupported directory") || !contains(msg, "/NotADir") {
		t.Fatalf("msg = %q", msg)
	}

	// Neither field given → reject, and no outgoing call may be made.
	before := len(*sets)
	_, _, msg = do(t, router, http.MethodPost, "/v2/watch/dirs", map[string]any{})
	if contains(msg, "ok") || msg == "" {
		t.Fatalf("a missing field must be rejected, msg = %q", msg)
	}
	if len(*sets) != before {
		t.Fatal("a missing field must not reach the capability")
	}
}

// A dir outside the ceiling (which this fake cap reports as a failed response) must return
// the real reason to the UI.
func TestSetSurfacesCeilingRejection(t *testing.T) {
	srv := bustest.New(t, bustest.Options{AppID: "music", OnCall: func(req protocol.Request) (string, json.RawMessage) {
		if string(req.Template) == protocol.TemplateEventScopeSet {
			return "", mustJSON(t, map[string]any{
				"success": false,
				"error":   "作用域值 \"space.movies\" 不在 manifest 的监听范围内",
			})
		}
		return "", mustJSON(t, map[string]any{"success": true, "ceiling": []string{"space.music"}, "scope": []string{"space.music"}})
	}})
	router := newRouter(t, srv)

	code, _, msg := do(t, router, http.MethodPost, "/v2/watch/dirs", map[string]any{"dirs": []string{"/Movies"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if !contains(msg, "监听范围") {
		t.Fatalf("the ceiling rejection reason must reach the UI, msg = %q", msg)
	}
}

func TestOptionsExposesCeiling(t *testing.T) {
	srv, _ := newCapServer(t, capServerOptions{ceiling: []string{"space.music"}, scope: []string{"space.music/2024"}})
	router := newRouter(t, srv, "/Music")

	code, data, msg := do(t, router, http.MethodGet, "/v2/watch/dirs/options", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d (%s)", code, msg)
	}
	var payload struct {
		Dirs        []string `json:"dirs"`
		Scopes      []string `json:"scopes"`
		Ceiling     []string `json:"ceiling"`
		CeilingDirs []string `json:"ceilingDirs"`
		Degraded    bool     `json:"degraded"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode: %v (%s)", err, string(data))
	}
	if payload.Degraded {
		t.Fatal("a healthy capability must not be marked as degraded")
	}
	if len(payload.Dirs) != 1 || payload.Dirs[0] != "/Music/2024" {
		t.Fatalf("dirs = %v", payload.Dirs)
	}
	if len(payload.CeilingDirs) != 1 || payload.CeilingDirs[0] != "/Music" {
		t.Fatalf("ceilingDirs = %v", payload.CeilingDirs)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && bytes.Contains([]byte(haystack), []byte(needle))
}
