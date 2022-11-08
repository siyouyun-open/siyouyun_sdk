package gateway

import (
	"errors"
	"fmt"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/entity"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
)

var appGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, OSHTTPPort, "faas")

func GetAppInfo(code string) (*entity.AppRegisterInfo, error) {
	api := appGatewayAddr + "/app/info"
	fmt.Println(api)
	response := restclient.GetRequest[entity.AppRegisterInfo](api, map[string]string{"appCode": code})
	if response.Code != sdkconst.Success {
		return nil, errors.New(response.Msg)
	}
	data := response.Data
	if data == nil {
		return nil, nil
	}
	return response.Data, nil
}
