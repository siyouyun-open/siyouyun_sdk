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
	// default in docker
	inDocker := true
	value := os.Getenv("IN_DOCKER")
	if value != "" {
		inDocker = value == "true"
	}
	if inDocker {
		return "10.62.0.1"
	} else {
		return "127.0.0.1"
	}
}
