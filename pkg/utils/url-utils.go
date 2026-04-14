package utils

import (
	"fmt"
	"os"
)

func GetCoreServiceURL() string {
	return fmt.Sprintf("http://%s:40100/syy", getIPByEnv())
}

func GetOSServiceURL() string {
	return fmt.Sprintf("http://%s:40000/os", getIPByEnv())
}

func GetAIServiceURL() string {
	return fmt.Sprintf("%s:40051", getIPByEnv())
}

func GetNatsServiceURL() string {
	return fmt.Sprintf("nats://%s:4222", getIPByEnv())
}

func getIPByEnv() string {
	appRuntime := os.Getenv("APP_RUNTIME")
	var ip string
	switch appRuntime {
	case "DOCKER":
		ip = "10.4.0.1"
	case "RUNC":
		ip = "172.19.0.1"
	default:
		ip = "127.0.0.1"
	}
	return ip
}
