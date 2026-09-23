package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Scope is the scope of one user-scoped operation.
//
// Why it is needed: a container app is deployed inside a user space, and the daemon can derive
// username/namespace from the connection namespace; a native app is a shared host process that is
// "installed once and serves multiple spaces", whose connection namespace is always default, so the
// user cannot be derived. Therefore native apps must pass scope explicitly when calling user-scoped
// capabilities, and the daemon validates it against the instance's authorized space set.
type Scope struct {
	Username  string
	GroupName string
	Namespace string
}

// IsZero reports whether scope is empty (that is, "derived by the daemon from the connection").
func (s Scope) IsZero() bool {
	return strings.TrimSpace(s.Username) == "" &&
		strings.TrimSpace(s.GroupName) == "" &&
		strings.TrimSpace(s.Namespace) == ""
}

// payload builds the user identity fields of a capability request.
//
// It returns map[string]any rather than map[string]string: capabilities such as scheduled tasks also
// put integers (event ID, timestamps) into the same payload.
func (s Scope) payload() map[string]any {
	out := map[string]any{}
	if v := strings.TrimSpace(s.Username); v != "" {
		out["username"] = v
	}
	if v := strings.TrimSpace(s.GroupName); v != "" {
		out["groupName"] = v
	}
	if v := strings.TrimSpace(s.Namespace); v != "" {
		out["namespace"] = v
	}
	return out
}

// Value is one KV record (fields and types match Core's model: id/timestamps are integers).
type Value struct {
	ID        int64  `json:"id,omitempty"`
	AppCode   string `json:"appCode,omitempty"`
	Type      string `json:"type,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

// KV is the entry point for the KV capability. Its two dimensions are explicitly separated:
//   - User: the user dimension (requires a user scope; native must provide one)
//   - App: the app dimension (follows the app and is unrelated to the user)
type KV struct {
	client *bus.Client
}

// NewKV creates a KV wrapper.
func NewKV(client *bus.Client) *KV { return &KV{client: client} }

// User returns the KV view under a user scope; an empty scope means the daemon derives it from the
// connection (available only to container apps).
func (k *KV) User(scope Scope) *UserKV { return &UserKV{client: k.client, scope: scope} }

// App returns the app-dimension KV view.
func (k *KV) App() *AppKV { return &AppKV{client: k.client} }

// UserKV is the user-dimension KV.
type UserKV struct {
	client *bus.Client
	scope  Scope
}

// Get reads one record; found=false means the key does not exist.
func (k *UserKV) Get(ctx context.Context, kvType, key string) (Value, bool, error) {
	payload := k.scope.payload()
	payload["type"] = kvType
	payload["key"] = key

	raw, err := k.call(ctx, protocol.TemplateKVGet, payload)
	if err != nil {
		return Value{}, false, err
	}
	var res struct {
		Value
		Found bool `json:"found"`
	}
	if err := decodeInto(raw, &res); err != nil {
		return Value{}, false, err
	}
	return res.Value, res.Found || res.Value.Key != "", nil
}

// Put writes one record.
func (k *UserKV) Put(ctx context.Context, kvType, key, value string) error {
	payload := k.scope.payload()
	payload["type"] = kvType
	payload["key"] = key
	payload["value"] = value
	_, err := k.call(ctx, protocol.TemplateKVPut, payload)
	return err
}

// List lists all records under a type.
func (k *UserKV) List(ctx context.Context, kvType string) ([]Value, error) {
	payload := k.scope.payload()
	payload["type"] = kvType
	return decodeList(k.call(ctx, protocol.TemplateKVList, payload))
}

// Delete deletes one record.
func (k *UserKV) Delete(ctx context.Context, kvType, key string) error {
	payload := k.scope.payload()
	payload["type"] = kvType
	payload["key"] = key
	_, err := k.call(ctx, protocol.TemplateKVDelete, payload)
	return err
}

// call invokes the user-dimension KV capability through the path that **unwraps the envelope**.
//
// This used to use client.CallInto: it judged success only by the bus-layer code, while a daemon
// proxying to Core/OS capabilities puts the upstream {code,data,msg} into data verbatim. So a Core
// failure was treated as success — the most direct consequence being that Put silently succeeded
// (the write had actually failed), and the subsequent Get reported "not found", leaving the user
// with an inexplicable inconsistency instead of "A00500 relation siyou_kv does not exist".
func (k *UserKV) call(ctx context.Context, template string, payload any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := callEnvelope(ctx, k.client, &bus.Request{Template: template, Payload: payload}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AppKV is the app-dimension KV: data follows the app and is unrelated to the user.
type AppKV struct {
	client *bus.Client
}

// Get reads one app record.
func (k *AppKV) Get(ctx context.Context, kvType, key string) (Value, bool, error) {
	payload := map[string]string{"type": kvType, "key": key}
	raw, err := callRaw(ctx, k.client, protocol.TemplateKVSysGet, payload)
	if err != nil {
		return Value{}, false, err
	}
	var res struct {
		Value
		Found bool `json:"found"`
	}
	if err := decodeInto(raw, &res); err != nil {
		return Value{}, false, err
	}
	return res.Value, res.Found || res.Value.Key != "", nil
}

// Put writes one app record.
func (k *AppKV) Put(ctx context.Context, kvType, key, value string) error {
	_, err := callRaw(ctx, k.client, protocol.TemplateKVSysPut, map[string]string{
		"type": kvType, "key": key, "value": value,
	})
	return err
}

// List lists all app records under a type.
func (k *AppKV) List(ctx context.Context, kvType string) ([]Value, error) {
	raw, err := callRaw(ctx, k.client, protocol.TemplateKVSysList, map[string]string{"type": kvType})
	return decodeList(raw, err)
}

// Delete deletes one app record.
func (k *AppKV) Delete(ctx context.Context, kvType, key string) error {
	_, err := callRaw(ctx, k.client, protocol.TemplateKVSysDelete, map[string]string{"type": kvType, "key": key})
	return err
}

// callRaw invokes the app-dimension KV capability, which likewise has to unwrap the envelope and
// check the upstream error code (for the reason see UserKV.call).
//
// When out is a json.RawMessage, decodeInto restores the data inside the envelope to raw JSON, so
// the Get/List below still receive business data rather than {code,data,msg}.
func callRaw(ctx context.Context, client *bus.Client, template string, payload any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := callEnvelope(ctx, client, &bus.Request{Template: template, Payload: payload}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decodeList unwraps a list response. Core's return shape for list differs slightly between
// versions (array / {values:[...]} / {list:[...]}), and it is normalized here.
func decodeList(raw json.RawMessage, err error) ([]Value, error) {
	if err != nil {
		return nil, err
	}
	data, uerr := unwrapResult(raw)
	if uerr != nil {
		return nil, uerr
	}
	if len(data) == 0 {
		return nil, nil
	}
	var direct []Value
	if err := json.Unmarshal(data, &direct); err == nil {
		return direct, nil
	}
	var wrapped struct {
		Values []Value `json:"values"`
		List   []Value `json:"list"`
		Items  []Value `json:"items"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("decode kv list: %w", err)
	}
	switch {
	case wrapped.Values != nil:
		return wrapped.Values, nil
	case wrapped.List != nil:
		return wrapped.List, nil
	default:
		return wrapped.Items, nil
	}
}
