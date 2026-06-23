package apiclient

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

// Client is the API transport interface.
// Implementations: httpClient (host mode), busClient (RUNC mode).
type Client interface {
	Do(method, path string, ugn *utils.UserGroupNamespace, query map[string]string, body any) ([]byte, error)
}

var instance Client

// Init initializes the API client. In host mode this sets up the HTTP transport.
// This is a no-op; the client is auto-initialized based on APP_RUNTIME.
func Init() {}

func init() {
	if os.Getenv("APP_RUNTIME") == "RUNC" {
		instance = &busClient{}
	} else {
		instance = newHTTPClient()
	}
}

// SetClient replaces the current transport. Called by transport.InitBus
// to activate the bus client after BusTransport is initialized.
func SetClient(c Client) { instance = c }

// ResetClient resets to the default client for the current runtime mode.
func ResetClient() {
	if os.Getenv("APP_RUNTIME") == "RUNC" {
		instance = &busClient{}
	} else {
		instance = newHTTPClient()
	}
}

// PostRequest sends a POST request through the current transport.
func PostRequest[T any](ugn *utils.UserGroupNamespace, fullApi string, query map[string]string, body any) restjson.Response[T] {
	path := extractPath(fullApi)
	raw, err := instance.Do("POST", path, ugn, query, body)
	if err != nil {
		return restjson.ResJson[T](sdkconst.RPCError, nil, "remote invoke err: "+err.Error())
	}
	var resp restjson.Response[T]
	if err := json.Unmarshal(raw, &resp); err != nil {
		return restjson.ResJson[T](sdkconst.RPCError, nil, "unmarshal response: "+err.Error())
	}
	return resp
}

// GetRequest sends a GET request through the current transport.
func GetRequest[T any](ugn *utils.UserGroupNamespace, fullApi string, query map[string]string) restjson.Response[T] {
	path := extractPath(fullApi)
	raw, err := instance.Do("GET", path, ugn, query, nil)
	if err != nil {
		return restjson.ResJson[T](sdkconst.RPCError, nil, "remote invoke err: "+err.Error())
	}
	var resp restjson.Response[T]
	if err := json.Unmarshal(raw, &resp); err != nil {
		return restjson.ResJson[T](sdkconst.RPCError, nil, "unmarshal response: "+err.Error())
	}
	return resp
}

// extractPath extracts the REST path from a full URL.
// e.g. "http://172.19.0.1:40100/syy/v2/app/file/stat" → "/syy/v2/app/file/stat"
func extractPath(fullURL string) string {
	if idx := strings.Index(fullURL, "://"); idx >= 0 {
		fullURL = fullURL[idx+3:]
	}
	if idx := strings.Index(fullURL, "/"); idx >= 0 {
		return fullURL[idx:]
	}
	return fullURL
}

// GetIP returns the real client IP from the request.
func GetIP(r *http.Request) (string, error) {
	ip := r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip, nil
	}

	ip = r.Header.Get("X-Forward-For")
	if ip != "" {
		return ip, nil
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}

	if net.ParseIP(ip) != nil {
		return ip, nil
	}

	return "", errors.New("no valid ip found")
}
