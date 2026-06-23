package gateway

import (
	"errors"

	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

func SendMessage(ugn *utils.UserGroupNamespace, appCode, content, replyUUID string) error {
	api := utils.GetCoreServiceURL() + "/v2/app/msg/session/send"
	response := apiclient.PostRequest[any](
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
