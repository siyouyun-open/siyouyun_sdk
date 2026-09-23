package bus

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// newFakeHTTPBus starts a minimal HTTP capability endpoint and returns its base address.
// handler receives a template name and returns (status code, response body).
func newFakeHTTPBus(t *testing.T, handler func(template string) (int, string)) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != protocol.PathCapabilityCall {
			http.NotFound(w, r)
			return
		}
		// The caller token must be present: the daemon uses it to recover the native identity.
		if got := r.Header.Get(protocol.HeaderCallerToken); got == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":"missing caller token"}`))
			return
		}
		var body struct {
			Template string `json:"template"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		status, payload := handler(body.Template)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)
	return server.URL
}
