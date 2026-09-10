package gateway

import (
	"errors"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// GetAppInfo returns the app registration info (appAddr / appDSN / UGNList).
//
// Under the minictl native-legacy runtime the listen address is injected through
// the environment, and the injected value wins:
//   - the legacy OS knows the app: keep its appDSN/UGNList and override appAddr;
//   - the legacy OS has no row (a brand new app): do not panic, start with the
//     minimal info instead.
//
// When nothing is injected the behaviour is exactly as before.
func GetAppInfo(code string) (*sdkdto.AppRegisterInfo, error) {
	injectedAddr := utils.RuntimeAppAddr()
	api := utils.GetOSServiceURL() + "/app/info"
	response := restclient.GetRequest[sdkdto.AppRegisterInfo](nil, api, map[string]string{"appCode": code})
	if response.Code != sdkconst.Success || response.Data == nil {
		msg := response.Msg
		if msg == "" {
			msg = "app not exist"
		}
		if injectedAddr != "" {
			sdklog.Logger.Warnf("GetAppInfo(%s) failed (%s), falling back to the runtime-injected listen address %s", code, msg, injectedAddr)
			return &sdkdto.AppRegisterInfo{AppCode: code, AppAddr: injectedAddr}, nil
		}
		sdklog.Logger.Errorf("GetAppInfo err: %v", msg)
		return nil, errors.New(msg)
	}
	if injectedAddr != "" && response.Data.AppAddr != injectedAddr {
		sdklog.Logger.Infof("using runtime-injected listen address: %s -> %s", response.Data.AppAddr, injectedAddr)
		response.Data.AppAddr = injectedAddr
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
