package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

func SendMessage(ugn *utils.UserGroupNamespace, appCode, content, replyUUID string) error {
	api := CoreServiceURL + "/msg/robot/session/send"
	response := restclient.PostRequest[any](
		ugn,
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
