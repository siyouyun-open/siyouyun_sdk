package sdkentity

import "github.com/siyouyun-open/siyouyun_sdk/utils"

type AppRegisterInfo struct {
	AppCode          string                `json:"appCode"`
	AppName          string                `json:"appName"`
	AppDesc          string                `json:"appDesc"`
	AppVersion       string                `json:"appVersion"`
	DSN              string                `json:"dsn"`
	RegisterUserList []utils.UserNamespace `json:"registerUserList"`
}
