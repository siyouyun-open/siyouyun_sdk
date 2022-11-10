package entity

type AppRegisterInfo struct {
	AppCode          string   `json:"appCode"`
	AppName          string   `json:"appName"`
	AppDesc          string   `json:"appDesc"`
	AppVersion       string   `json:"appVersion"`
	DSN              string   `json:"dsn"`
	RegisterUserList []string `json:"registerUserList"`
}

type Model struct {
	ID        uint  `json:"id"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

type ActionAppRegisterInfo struct {
	Model
	AppCodeName string
	EventType   int
	FileType    string
	Description string
	Priority    int
	Code        string
}

func (ActionAppRegisterInfo) TableName() string {
	return "siyou_action_app_register_info"
}

type Apps struct {
	Model
	CodeName    string `gorm:"type:varchar(255);comment:程序标识"`
	Name        string `gorm:"type:varchar(255);comment:程序名称"`
	Description string `gorm:"type:text;comment:描述"`
}

func (Apps) TableName() string {
	return "siyou_apps"
}
