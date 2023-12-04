package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
)

var appGatewayAddr = CoreServiceURL + "/faas"

func GetAppInfo(code string) (*sdkdto.AppRegisterInfo, error) {
	api := appGatewayAddr + "/app/info"
	response := restclient.GetRequest[sdkdto.AppRegisterInfo](nil, api, map[string]string{"appCode": code})
	if response.Code != sdkconst.Success {
		return nil, errors.New(response.Msg)
	}
	data := response.Data
	if data == nil {
		return nil, nil
	}
	return response.Data, nil
}
