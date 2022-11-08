package siyouyunsdk

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

type app struct {
	AppCode string
	Api     SiyouFaasApi
	AppInfo *entity.AppRegisterInfo
	Event   *EventHolder
	Model   []interface{}

	DB *gorm.DB
}

var App *app

func NewApp() *app {
	var err error

	// init http client
	restclient.InitHttpClient()

	App.AppCode = os.Getenv(AppCodeEnvKey)

	// get app info
	App.AppInfo, err = gateway.GetAppInfo(App.AppCode)
	if err != nil {
		panic(err)
	}

	// init db
	db, _ := gorm.Open(mysql.Open(App.AppInfo.DSN), &gorm.Config{
		Logger: siyoumysql.NewLogger(),
	})
	App.DB = db

	// init api
	App.Api = make(SiyouFaasApi)

	return App
}

func (a *app) exec(un *utils.UserNamespace, f func(*gorm.DB) error) error {
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
