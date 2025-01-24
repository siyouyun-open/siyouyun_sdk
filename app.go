package siyouyunsdk

import (
	stdContext "context"
	"github.com/kataras/iris/v12"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/internal/rdb"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/localize"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http"
	"os"
	"time"
)

const (
	AppCodeEnvKey = "APPCODE"
)

type AppStruct struct {
	AppCode string                  // app code
	Ability *Ability                // app ability
	server  *iris.Application       // app iris web server
	appInfo *sdkdto.AppRegisterInfo // app register info
	nc      *nats.Conn              // nats conn
	db      *gorm.DB                // gorm db instance
}

var App *AppStruct

// NewApp new standard app
func NewApp() *AppStruct {
	var err error
	App = &AppStruct{
		server: iris.New(),
	}

	// init log
	sdklog.InitLogger(logrus.InfoLevel)

	// init http client
	restclient.InitHttpClient()

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

	// init postgres db
	if App.appInfo.AppDSN != "" {
		db, err := gorm.Open(postgres.New(postgres.Config{
			DSN: App.appInfo.AppDSN,
			//PreferSimpleProtocol: true,
		}), &gorm.Config{Logger: rdb.NewLogger()})
		if err != nil {
			panic(err)
		}
		sqlDB, _ := db.DB()
		sqlDB.SetConnMaxLifetime(time.Minute * 30)
		sqlDB.SetConnMaxIdleTime(time.Minute * 3)
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(2)
		App.db = db
	}

	// init ability
	App.InitAbility()

	return App
}

func (a *AppStruct) StartWebServer() {
	idleConnsClosed := make(chan struct{})
	iris.RegisterOnInterrupt(func() {
		timeout := 10 * time.Second
		ctx, cancel := stdContext.WithTimeout(stdContext.Background(), timeout)
		defer cancel()
		a.Destroy()
		a.server.Shutdown(ctx)
		close(idleConnsClosed)
	})
	// config iris i18n
	if localize.Instance != nil {
		localize.Instance.ConfigIris(a.server)
	}
	// add default router
	a.server.Head("/health", func(ctx iris.Context) { ctx.StatusCode(http.StatusOK) })
	a.server.Get("/icon", a.GetIcon)
	a.server.Listen(a.appInfo.AppAddr, iris.WithoutInterruptHandler, iris.WithoutServerError(iris.ErrServerClosed))
	<-idleConnsClosed
}

func (a *AppStruct) Destroy() {
	if a.Ability != nil {
		a.Ability.Destroy()
	}
	if a.db != nil {
		sqlDB, err := a.db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

// GetUGNList get all ugn list
func (a *AppStruct) GetUGNList() []utils.UserGroupNamespace {
	if a.appInfo == nil {
		return nil
	}
	return a.appInfo.UGNList
}
