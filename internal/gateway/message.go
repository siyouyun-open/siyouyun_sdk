package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

func SendMessage(ugn *utils.UserGroupNamespace, appCode, content, replyUUID string) error {
	api := utils.GetCoreServiceURL() + "/msg/robot/session/send"
	response := restclient.PostRequest[any](
		ugn,
		api,
		map[string]string{
			"robotCode":   appCode + "_msg",
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
