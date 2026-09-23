package bus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// httpMaxResponse caps how much of a single HTTP capability response is read, so an abnormal provider cannot blow up memory.
const httpMaxResponse = 16 << 20

// httpTransport is the daemon's HTTP capability channel (/api/v1/capability/call).
//
// It serves two purposes:
//   - an RPC fallback when there is no streaming connection;
//   - the only channel for file/streaming calls (JSON frames cannot carry binary).
type httpTransport struct {
	baseURL string
	token   string
	client  *http.Client
}

func newHTTPTransport(baseURL, callerToken string, timeout time.Duration) *httpTransport {
	return &httpTransport{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(callerToken),
		client:  &http.Client{Timeout: timeout},
	}
}

// call uses JSON mode: the request body is {template, signature, provider_app_id, payload, headers},
// and the response body is the daemon's CapabilityResponse {success, error, result}.
func (h *httpTransport) call(ctx context.Context, req *Request) (*protocol.Response, error) {
	body := map[string]any{"template": req.Template}
	if req.Signature != "" {
		body["signature"] = req.Signature
	}
	if req.ProviderAppID != "" {
		body["provider_app_id"] = req.ProviderAppID
	}
	if req.Payload != nil {
		body["payload"] = req.Payload
	}
	if len(req.Headers) > 0 {
		body["headers"] = req.Headers
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("bus: encode http request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+protocol.PathCapabilityCall, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	h.decorate(httpReq)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bus: http capability call: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, httpMaxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("bus: read http capability response: %w", err)
	}
	if len(raw) > httpMaxResponse {
		return nil, fmt.Errorf("bus: http capability response exceeds %d bytes", httpMaxResponse)
	}

	var envelope struct {
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("bus: decode http capability response (http %d): %w", resp.StatusCode, err)
	}
	if !envelope.Success {
		msg := strings.TrimSpace(envelope.Error)
		if msg == "" {
			msg = fmt.Sprintf("capability call failed (http %d)", resp.StatusCode)
		}
		return &protocol.Response{Type: protocol.FrameResponse, Code: protocol.CodeError, Error: msg}, nil
	}
	return &protocol.Response{
		Type: protocol.FrameResponse,
		Code: protocol.CodeSuccess,
		Data: envelope.Result,
	}, nil
}

// decorate adds the caller identity header to the request. The daemon treats it only as "identifying the caller", not as an authorization credential.
func (h *httpTransport) decorate(req *http.Request) {
	if h.token != "" {
		req.Header.Set(protocol.HeaderCallerToken, h.token)
	}
}
