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
	"time"
)

const (
	AppCodeEnvKey = "APPCODE"
)

type AppStruct struct {
	AppCode  string
	Api      SiyouFaasApi
	AppInfo  *sdkentity.AppRegisterInfo
	Event    *EventHolder
	Schedule *ScheduleHandler
	Model    []interface{}

	DB *gorm.DB
}

var App *AppStruct

func NewApp() *AppStruct {
	var err error
	App = &AppStruct{}

	// init http client
	restclient.InitHttpClient()

	App.AppCode = os.Getenv(AppCodeEnvKey)

	// get app info
	App.AppInfo, err = gateway.GetAppInfo(App.AppCode)
	if err != nil {
		panic(err)
	}

	// init db
	db, _ := gorm.Open(mysql.Open(App.AppInfo.AppDSN), &gorm.Config{
		Logger: siyoumysql.NewLogger(),
	})
	sqlDB, _ := db.DB()
	sqlDB.SetConnMaxLifetime(time.Minute * 5)
	sqlDB.SetConnMaxIdleTime(time.Minute * 1)
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)
	App.DB = db

	// init api
	App.Api = make(SiyouFaasApi)
	App.Api.Get("/alive", Alive)

	EnableMessage(App.AppInfo.AppName, nil)

	return App
}

func (a *AppStruct) exec(un *utils.UserGroupNamespace, f func(*gorm.DB) error) error {
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
