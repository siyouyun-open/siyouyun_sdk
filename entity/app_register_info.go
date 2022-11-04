package entity

type AppRegisterInfo struct {
	AppCode          string
	AppName          string
	AppDesc          string
	AppVersion       string
	DSN              string
	RegisterUserList []string
}
