package utils

import (
	"fmt"
	"os"
)

// GetCoreServiceURL returns the Core Service base URL.
// In RUNC mode this should not be used; all calls route through the bus.
func GetCoreServiceURL() string {
	return fmt.Sprintf("http://%s:40100/syy", GetIPByEnv())
}

// GetOSServiceURL returns the OS Service base URL.
// In RUNC mode this should not be used; all calls route through the bus.
func GetOSServiceURL() string {
	return fmt.Sprintf("http://%s:40000/os", GetIPByEnv())
}

// GetAIServiceURL returns the AI gRPC service address.
func GetAIServiceURL() string {
	return fmt.Sprintf("%s:40051", GetIPByEnv())
}

// GetNatsServiceURL returns the NATS connection URL.
// In RUNC mode this should not be used; NATS is disabled.
func GetNatsServiceURL() string {
	return fmt.Sprintf("nats://%s:4222", GetIPByEnv())
}

// GetIPByEnv returns the service IP based on APP_RUNTIME env var.
func GetIPByEnv() string {
	appRuntime := os.Getenv("APP_RUNTIME")
	switch appRuntime {
	case "DOCKER":
		return "10.4.0.1"
	case "RUNC":
		return "172.19.0.1"
	default:
		return "127.0.0.1"
	}
}
