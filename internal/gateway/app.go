package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
)

func GetAppInfo(code string) (*sdkdto.AppRegisterInfo, error) {
	api := OSURL + "/faas/app/info"
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

func RegisterAppMessageRobot(appCode, robotDesc string) error {
	api := OSURL + "/faas/app/robot/register"
	response := restclient.PostRequest[any](
		nil,
		api,
		map[string]string{
			"appCode":   appCode,
			"robotCode": appCode + "_msg",
			"robotDesc": robotDesc,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}
