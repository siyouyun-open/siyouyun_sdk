package siyouyunsdk

import (
	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/ability"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/internal/mysql"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	AppCodeEnvKey = "APPCODE"
	IconPath      = "/home/app/icon.png"
)

type AppStruct struct {
	AppCode string
	Event   *EventHolder
	Ability *Ability     // app ability
	Api     SiyouFaaSApi // app interfaces

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

	// detect env
	App.detectEnv()

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

	// init ability
	App.Ability = &Ability{}
	App.WithFS()

	// init api
	App.Api = make(SiyouFaaSApi)
	App.Api.Get("/alive", Alive)
	App.Api.Get("/icon", GetIcon)

	// 注册应用消息
	err = gateway.RegisterAppMessageRobot(App.AppCode, App.appInfo.AppName)
	if err != nil {
		log.Printf("[ERROR] RegisterAppMessageRobot err: %v", err)
	}
	// listen sys message
	App.listenSysMsg()

	return App
}

func (a *AppStruct) Destroy() {
	a.Ability.Destroy()
	if a.db != nil {
		sqlDB, err := a.db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (a *AppStruct) listenSysMsg() {
	robotCode := a.AppCode + "_msg"
	// 开启监听
	go func() {
		log.Printf("[INFO] start ListenSysMsg at:%v", robotCode)
		_, err := a.nc.Subscribe(robotCode, func(msg *nats.Msg) {
			var mes []ability.MessageEvent
			defer func() {
				if err := recover(); err != nil {
					log.Printf("nats panic:[%v]-[%v]", err, mes)
				}
			}()
			err := json.Unmarshal(msg.Data, &mes)
			if err != nil {
				log.Printf("[ERROR] listenSysMsg Unmarshal err: %v", err)
				return
			}
			for i := range mes {
				ugn := utils.NewUserGroupNamespace(mes[i].UGN.Username, mes[i].UGN.GroupName, mes[i].UGN.Namespace)
				if mes[i].SendByAdmin {
					switch mes[i].Content {
					case "autoMigrate":
						log.Printf("[INFO] autoMigrate: %v", mes[i].Content)
						a.setUserWithModel(ugn)
					}
				}
			}
			return
		})
		if err != nil {
			log.Printf("[ERROR] listenSysMsg Subscribe err: %v", err)
		}
	}()
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

// detectEnv detect the environment, docker or host
func (a *AppStruct) detectEnv() {
	inDocker := true
	content, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		if !strings.Contains(string(content), "openfaas") {
			inDocker = false
		}
	}
	os.Setenv("IN_DOCKER", strconv.FormatBool(inDocker))
}
