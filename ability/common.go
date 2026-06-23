package ability

import (
	"net/http"

	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

func isCoreServiceReady() bool {
	statusURL := utils.GetCoreServiceURL() + "/status/ready"
	if apiclient.HTTPClient == nil {
		return false
	}
	resp, err := apiclient.HTTPClient.R().Head(statusURL)
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}

func isOSServiceReady() bool {
	statusURL := utils.GetOSServiceURL() + "/status/ready"
	if apiclient.HTTPClient == nil {
		return false
	}
	resp, err := apiclient.HTTPClient.R().Head(statusURL)
	if err != nil {
		return false
	}
	return resp.StatusCode() == http.StatusOK
}
