package siyouyunsdk

import (
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/internal/mysql"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"time"
)

const (
	AppCodeEnvKey = "APPCODE"
	IconPath      = "/home/app/icon.png"
)

type AppStruct struct {
	AppCode  string
	Event    *EventHolder
	Schedule *ScheduleHandler
	Api      SiyouFaaSApi // app interfaces

	db      *gorm.DB
	nc      *nats.Conn
	models  []interface{}           // app table models
	appInfo *sdkdto.AppRegisterInfo // app register info
}

var App *AppStruct

// NewApp new standard app
func NewApp() *AppStruct {
	var err error
	App = &AppStruct{}

	// init http client
	restclient.InitHttpClient()

	App.AppCode = os.Getenv(AppCodeEnvKey)

	// get app info
	App.appInfo, err = gateway.GetAppInfo(App.AppCode)
	if err != nil {
		panic(err)
	}

	// init db
	db, _ := gorm.Open(mysql.Open(App.appInfo.AppDSN), &gorm.Config{
		Logger: siyoumysql.NewLogger(),
	})
	sqlDB, _ := db.DB()
	sqlDB.SetConnMaxLifetime(time.Minute * 30)
	sqlDB.SetConnMaxIdleTime(time.Minute * 3)
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)
	App.db = db

	// init api
	App.Api = make(SiyouFaaSApi)
	App.Api.Get("/alive", Alive)
	App.Api.Get("/icon", GetIcon)

	// enable message bot
	EnableMessage(App.appInfo.AppName, nil)

	return App
}

func (a *AppStruct) exec(ugn *utils.UserGroupNamespace, f func(*gorm.DB) error) error {
	err := a.db.Transaction(func(tx *gorm.DB) (err error) {
		dbname := ugn.DatabaseName()
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
