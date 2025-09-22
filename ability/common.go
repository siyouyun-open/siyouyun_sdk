package ability

import (
	"net/http"

	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

func isCoreServiceReady() bool {
	statusURL := utils.GetCoreServiceURL() + "/status/ready"
	if restclient.Client == nil {
		return false
	}
	resp, err := restclient.Client.R().Head(statusURL)
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}

func isOSServiceReady() bool {
	statusURL := utils.GetOSServiceURL() + "/status/ready"
	if restclient.Client == nil {
		return false
	}
	resp, err := restclient.Client.R().Head(statusURL)
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}
