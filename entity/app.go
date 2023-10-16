package sdkentity

import "github.com/siyouyun-open/siyouyun_sdk/utils"

type AppRegisterInfo struct {
	AppCode     string                     `json:"appCode"`
	AppName     string                     `json:"appName"`
	AppVersion  string                     `json:"appVersion"`
	AppDSN      string                     `json:"appDSN"`
	Description string                     `json:"description"`
	UGNList     []utils.UserGroupNamespace `json:"ugnList"`
}
