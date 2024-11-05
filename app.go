package siyouyunsdk

import (
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/internal/rdb"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	AppCode       string
	Event         *EventHolder
	Ability       *Ability                              // app ability
	Api           SiyouFaaSApi                          // app interfaces
	appInfo       *sdkdto.AppRegisterInfo               // app register info
	nc            *nats.Conn                            // nats conn
	db            *gorm.DB                              // gorm db instance
	migrateSchema func(*utils.UserGroupNamespace) error // app migrate schema function
	migrateData   func(*utils.UserGroupNamespace) error // app migrate data function
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

	// listen sys event
	go App.listenSysEvent()

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
