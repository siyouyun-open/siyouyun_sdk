package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
)

var messageGatewayAddr = CoreServiceURL + "/msg"

func RegisterMessageRobot(appCode, robotDesc string) error {
	api := messageGatewayAddr + "/robot/register"
	response := restclient.PostRequest[any](
		nil,
		api,
		map[string]string{
			"appCode":   appCode,
			"robotCode": appCode + "_msg", // todo use uuid
			"robotDesc": robotDesc,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

func SendMessage(un *utils.UserGroupNamespace, appCode, content, replyUUID string) error {
	api := messageGatewayAddr + "/robot/session/send"
	response := restclient.PostRequest[any](
		un,
		api,
		map[string]string{
			"robotCode":   appCode + "_msg", // todo use uuid
			"content":     content,
			"replyToUUID": replyUUID,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}
