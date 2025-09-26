package siyouyunsdk

import (
	stdContext "context"
	"net/http"
	"os"
	"strconv"
	"time"

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
)

const (
	AppCodeEnvKey   = "APPCODE"
	AppFirstInitKey = "FIRST_INIT"
)

type AppStruct struct {
	AppCode      string                  // app code
	Ability      *Ability                // app ability
	server       *iris.Application       // app iris web server
	appInfo      *sdkdto.AppRegisterInfo // app register info
	nc           *nats.Conn              // nats conn
	db           *gorm.DB                // gorm db instance
	dataVersion  int                     // app data version
	isFirstInit  bool                    // is app being initialized for the first time
	shutdownHook func()                  // shutdown hook func
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
	App.isFirstInit, _ = strconv.ParseBool(os.Getenv(AppFirstInitKey))

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
		a.destroy()
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
	a.server.Get("/data/status", a.CheckAppDataStatus)
	a.server.Post("/data/refresh", a.RefreshAppData)
	a.server.Listen(a.appInfo.AppAddr, iris.WithoutInterruptHandler, iris.WithoutServerError(iris.ErrServerClosed))
	<-idleConnsClosed
}

// OnShutdown set shutdown hook
func (a *AppStruct) OnShutdown(hook func()) {
	a.shutdownHook = hook
}

// WithDataVersion set data version (need enable kv ability)
func (a *AppStruct) WithDataVersion(version int) error {
	// check if kv ability is enabled
	_, err := a.Ability.KV()
	if err != nil {
		return err
	}
	a.dataVersion = version
	return nil
}

func (a *AppStruct) destroy() {
	if a.shutdownHook != nil {
		a.shutdownHook()
	}
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

// IsFirstInit is app being initialized for the first time
func (a *AppStruct) IsFirstInit() bool {
	return a.isFirstInit
}
