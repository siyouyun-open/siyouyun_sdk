// Package capability provides strongly typed capability wrappers, divided by domain, on top of the
// bus client.
//
// Layering principles:
//   - bus is only responsible for "sending the request out and bringing the response/events back";
//   - capability is responsible for "what this domain's inputs look like, how responses are
//     unwrapped, and how success/failure is expressed".
//
// When adding a platform capability, usually you only need to add a file here and add the template
// constant in protocol.
package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Inner envelope: the daemon's KV / management and other capabilities pass the Core/OS response body
// through directly, so the data of a successful response still wraps a {code,data,msg} envelope.
type envelope struct {
	Code string          `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

// unwrapResult unwraps a possible Core/OS envelope and returns the real business data.
//
// Not every capability wraps an envelope (capabilities the daemon provides itself and provider apps
// both return their own JSON), so it unwraps only when the shape really is a platform envelope: the
// code must be A00xxx.
func unwrapResult(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil || !strings.HasPrefix(env.Code, "A00") {
		return raw, nil
	}
	if env.Code != protocol.CodeSuccess && env.Code != "A00102" {
		msg := env.Msg
		if msg == "" {
			msg = "upstream returned " + env.Code
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return env.Data, nil
}

// decodeInto unwraps the envelope and decodes the business data into out.
func decodeInto(raw json.RawMessage, out any) error {
	data, err := unwrapResult(raw)
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode capability result: %w", err)
	}
	return nil
}

// callEnvelope invokes a capability and decodes the result into out, automatically unwrapping the
// Core/OS envelope.
//
// The difference from bus.Client.CallInto: the latter only does "call + judge failure by code +
// decode data" and does not unwrap envelopes — the data of capabilities the daemon proxies to
// Core/OS (KV, media, scheduled tasks…) still wraps a {code,data,msg} envelope, which this layer
// must unwrap.
func callEnvelope(ctx context.Context, client *bus.Client, req *bus.Request, out any) error {
	resp, err := client.Call(ctx, req)
	if err != nil {
		return err
	}
	if resp.Code != protocol.CodeSuccess {
		msg := strings.TrimSpace(resp.Error)
		if msg == "" {
			msg = "capability call failed"
		}
		return &bus.CallError{Template: req.Template, Code: resp.Code, Message: msg}
	}
	return decodeInto(resp.Data, out)
}
