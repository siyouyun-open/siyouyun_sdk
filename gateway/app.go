package gateway

import (
	"errors"
	"fmt"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/entity"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
)

var appGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, CoreHTTPPort, "faas")

func GetAppInfo(code string) (*sdkentity.AppRegisterInfo, error) {
	api := appGatewayAddr + "/app/info"
	response := restclient.GetRequest[sdkentity.AppRegisterInfo](nil, api, map[string]string{"appCode": code})
	if response.Code != sdkconst.Success {
		return nil, errors.New(response.Msg)
	}
	data := response.Data
	if data == nil {
		return nil, nil
	}
	return response.Data, nil
}
