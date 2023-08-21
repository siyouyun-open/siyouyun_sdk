package sdkentity

import "github.com/siyouyun-open/siyouyun_sdk/utils"

type AppRegisterInfo struct {
	AppCode           string                     `json:"appCode"`
	AppName           string                     `json:"appName"`
	AppVersion        string                     `json:"appVersion"`
	Description       string                     `json:"description"`
	AppDSN            string                     `json:"appDSN"`
	UserNamespaceList []utils.UserGroupNamespace `json:"userNamespace"`
}
