package gateway

import (
	"errors"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

func SendMessage(ugn *utils.UserGroupNamespace, appCode, content, replyUUID string) error {
	api := utils.GetCoreServiceURL() + "/v2/app/msg/session/send"
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
