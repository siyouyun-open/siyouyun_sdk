package gateway

import (
	"errors"
	"fmt"
	"github.com/siyouyun-open/siyouyun_sdk"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
)

var eventGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, CoreHTTPPort, "faas")

func RegisterAndGetAppEvent(appCode string, options []siyouyunsdk.PreferOptions) error {
	api := eventGatewayAddr + "/app/event/register"
	response := restclient.PostRequest[any](
		nil,
		api,
		map[string]string{"appCode": appCode},
		options,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}
