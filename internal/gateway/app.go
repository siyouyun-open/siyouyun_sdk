package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"log"
)

func GetAppInfo(code string) (*sdkdto.AppRegisterInfo, error) {
	api := utils.GetOSServiceURL() + "/app/info"
	response := restclient.GetRequest[sdkdto.AppRegisterInfo](nil, api, map[string]string{"appCode": code})
	if response.Code != sdkconst.Success {
		log.Printf("[ERROR] GetAppInfo err: %v", response.Msg)
		return nil, errors.New(response.Msg)
	}
	if response.Data == nil {
		return nil, errors.New("app not exist")
	}
	return response.Data, nil
}

func RegisterAppMessageRobot(appCode, robotDesc string) error {
	api := utils.GetOSServiceURL() + "/app/robot/register"
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
