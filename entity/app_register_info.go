package entity

type AppRegisterInfo struct {
	AppCode          string   `json:"app_code"`
	AppName          string   `json:"appName"`
	AppDesc          string   `json:"appDesc"`
	AppVersion       string   `json:"appVersion"`
	DSN              string   `json:"dsn"`
	RegisterUserList []string `json:"registerUserList"`
}
