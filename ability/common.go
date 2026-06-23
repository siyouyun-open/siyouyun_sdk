package ability

import (
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

func isCoreServiceReady() bool {
	api := utils.GetCoreServiceURL() + "/status/ready"
	resp := apiclient.GetRequest[any](nil, api, nil)
	return resp.Code == sdkconst.Success
}

func isOSServiceReady() bool {
	api := utils.GetOSServiceURL() + "/status/ready"
	resp := apiclient.GetRequest[any](nil, api, nil)
	return resp.Code == sdkconst.Success
}
