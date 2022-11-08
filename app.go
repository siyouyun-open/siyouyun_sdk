package siyouyunfaas

import (
	"github.com/siyouyun-open/siyouyun_sdk/entity"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/mysql"
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
	AppCode string
	Api     SiyouFaasApi
	AppInfo *entity.AppRegisterInfo
	Event   *EventHolder
	Model   []interface{}

	DB *gorm.DB
}

func NewApp() *App {
	var app App
	var err error

	// init http client
	restclient.InitHttpClient()

	app.AppCode = os.Getenv(AppCodeEnvKey)

	// get app info
	app.AppInfo, err = gateway.GetAppInfo(app.AppCode)
	if err != nil {
		panic(err)
	}

	// init db
	db, _ := gorm.Open(mysql.Open(app.AppInfo.DSN), &gorm.Config{
		Logger: siyoumysql.NewLogger(),
	})
	app.DB = db

	// init api
	app.Api = make(SiyouFaasApi)

	return &app
}

func (a *App) exec(un *utils.UserNamespace, f func(*gorm.DB) error) error {
	err := a.DB.Transaction(func(tx *gorm.DB) (err error) {
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
