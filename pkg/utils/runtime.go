package utils

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// The minictl native-legacy runtime injects the listen address into the process:
//
//	MINI_APP_ENDPOINT=http://127.0.0.1:42000
//	MINI_APP_PORT=42000
//
// When the injection is present the app runs under minictl's private runtime:
// both the install directory and the port are decided by minictl, and the legacy
// siyouyun OS registry knows nothing about that port. In that case the injected
// value wins, and the app is not required to be registered with the legacy OS.
const (
	// RuntimeEndpointEnv holds the full listen address (http://host:port or host:port).
	RuntimeEndpointEnv = "MINI_APP_ENDPOINT"
	// RuntimePortEnv holds the port number only.
	RuntimePortEnv = "MINI_APP_PORT"
)

// RuntimeAppAddr returns the listen address injected by the runtime, in the form
// "127.0.0.1:42000". It returns an empty string when nothing was injected or the
// value is malformed, so callers keep their original behaviour.
func RuntimeAppAddr() string {
	if raw := strings.TrimSpace(os.Getenv(RuntimeEndpointEnv)); raw != "" {
		if addr := normalizeAppAddr(raw); addr != "" {
			return addr
		}
	}
	if raw := strings.TrimSpace(os.Getenv(RuntimePortEnv)); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 && port <= 65535 {
			return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
	}
	return ""
}

// RuntimeManaged reports whether the process runs under minictl's private runtime.
func RuntimeManaged() bool { return RuntimeAppAddr() != "" }

// normalizeAppAddr normalises the injected address into host:port form.
// It accepts http://host:port, host:port and a bare port; anything else yields
// an empty string.
func normalizeAppAddr(raw string) string {
	value := strings.TrimSpace(raw)
	if i := strings.Index(value, "://"); i >= 0 {
		value = value[i+3:]
	}
	if i := strings.IndexAny(value, "/?#"); i >= 0 {
		value = value[:i]
	}
	if value == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(value); err == nil {
		return value
	}
	if port, err := strconv.Atoi(value); err == nil && port > 0 && port <= 65535 {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	return ""
}
