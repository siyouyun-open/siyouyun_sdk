package siyouyunsdk

import (
	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/ability"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/internal/rdb"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/driver/postgres"
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
	AppCode         string
	Event           *EventHolder
	Ability         *Ability                              // app ability
	Api             SiyouFaaSApi                          // app interfaces
	appInfo         *sdkdto.AppRegisterInfo               // app register info
	nc              *nats.Conn                            // nats conn
	db              *gorm.DB                              // gorm db instance
	migrateSchema   func(db *gorm.DB) error               // app db migrate schema function
	schemaAfterFunc func(*utils.UserGroupNamespace) error // app db migrate schema postprocessing
	migrateData     func(db *gorm.DB) error               // app db migrate data function
	dataAfterFunc   func(*utils.UserGroupNamespace) error // app db migrate data postprocessing
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

	// init nc
	App.nc, err = nats.Connect(utils.GetNatsServiceURL())
	if err != nil {
		panic(err)
	}

	// init db
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  App.appInfo.AppDSN,
		PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: rdb.NewLogger()})
	if err != nil {
		panic(err)
	}
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
				return
			}
			for i := range mes {
				if mes[i].SendByAdmin {
					switch mes[i].Content {
					case "autoMigrate":
						log.Printf("[INFO] AutoMigrate, ugn: %s, ", mes[i].UGN)
						a.migrateWithUser(&mes[i].UGN)
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
