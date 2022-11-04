package siyouyunfaas

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/entity"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
)

const (
	AppCodeEnvKey = "AppCode"
)

type App struct {
	Api     SiyouFaasApi
	AppInfo *entity.AppRegisterInfo

	db *gorm.DB
}

func NewApp() *App {
	restclient.InitHttpClient()

	var app App
	app.init()

	return &app
}

func (a *App) init() {
	appCode := os.Getenv(AppCodeEnvKey)
	a.AppInfo = gateway.GetAppInfo(appCode)

	db, _ := gorm.Open(mysql.Open(a.AppInfo.DSN), &gorm.Config{})
	a.db = db
}

func (a *App) DBExec(ctx iris.Context, f func(*gorm.DB) error) error {
	un := utils.NewUserNamespaceFromIris(ctx)
	err := a.db.Transaction(func(tx *gorm.DB) (err error) {
		dbname := un.DatabaseName()
		if dbname == "" {
			return
		}
		err = tx.Exec("use " + dbname).Error
		if err != nil {
			return err
		}
		err = f(tx)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
