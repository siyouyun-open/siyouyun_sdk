package sdkdto

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type AppRegisterInfo struct {
	AppCode string                     `json:"appCode"`
	AppName string                     `json:"appName"`
	AppDSN  string                     `json:"appDSN"`
	AppAddr string                     `json:"appAddr"`
	UGNList []utils.UserGroupNamespace `json:"ugnList"`
}
