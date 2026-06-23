package siyouyunsdk

import (
	stdContext "context"
	"net/http"
	"os"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/rdb"
	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/transport"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/localize"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

const (
	AppCodeEnvKey = "APPCODE"
)

type AppStruct struct {
	AppCode      string                  // app code
	Ability      *Ability                // app ability
	server       *iris.Application       // app iris web server
	appInfo      *sdkdto.AppRegisterInfo // app register info
	nc           *nats.Conn              // nats conn (host mode only)
	db           *gorm.DB                // gorm db instance (host mode only)
	dataVersion  int                     // app data version
	shutdownHook func()                  // shutdown hook func
	isRUNC       bool                    // true when APP_RUNTIME=RUNC
}

var App *AppStruct

// NewApp creates a new standard app.
// In host mode (default): connects NATS + PostgreSQL via OS/Core services.
// In RUNC mode (APP_RUNTIME=RUNC): uses bus WebSocket only, NATS and PG are disabled.
func NewApp() *AppStruct {
	App = &AppStruct{
		server: iris.New(),
	}

	sdklog.InitLogger(logrus.InfoLevel)
	apiclient.Init()

	App.AppCode = os.Getenv(AppCodeEnvKey)
	App.isRUNC = os.Getenv("APP_RUNTIME") == "RUNC"

	if App.isRUNC {
		App.initRUNC()
	} else {
		App.initHost()
	}

	App.InitAbility()
	return App
}

// initRUNC initializes in RUNC mode: bus WebSocket only, no NATS/PG.
func (a *AppStruct) initRUNC() {
	sdklog.Logger.Infof("[App] Running in RUNC mode, using bus transport only")

	if err := transport.InitBus(a.AppCode); err != nil {
		panic("RUNC bus transport init failed: " + err.Error())
	}

	bus := transport.GetBus()
	info, err := bus.GetAppInfo()
	if err != nil {
		panic("RUNC get app info via bus failed: " + err.Error())
	}
	a.appInfo = info

	a.nc = nil
	a.db = nil

	sdklog.Logger.Infof("[App] RUNC init done: appCode=%s addr=%s", a.AppCode, a.appInfo.AppAddr)
}

// initHost initializes in host mode: standard NATS + PG setup.
func (a *AppStruct) initHost() {
	var err error

	a.appInfo, err = gateway.GetAppInfo(a.AppCode)
	if err != nil {
		panic(err)
	}

	a.nc, err = nats.Connect(utils.GetNatsServiceURL())
	if err != nil {
		panic(err)
	}

	if a.appInfo.AppDSN != "" {
		db, err := gorm.Open(postgres.New(postgres.Config{
			DSN:                  a.appInfo.AppDSN,
			PreferSimpleProtocol: true,
		}), &gorm.Config{Logger: rdb.NewLogger()})
		if err != nil {
			panic(err)
		}
		sqlDB, _ := db.DB()
		sqlDB.SetConnMaxLifetime(time.Minute * 30)
		sqlDB.SetConnMaxIdleTime(time.Minute * 3)
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(2)
		a.db = db
	}
}

func (a *AppStruct) ConfigureIris(configurators ...iris.Configurator) {
	if a == nil || a.server == nil {
		return
	}
	a.server.Configure(configurators...)
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
	if localize.Instance != nil {
		localize.Instance.ConfigIris(a.server)
	}
	a.server.Head("/health", func(ctx iris.Context) { ctx.StatusCode(http.StatusOK) })
	a.server.Get("/icon", a.GetIcon)
	a.server.Get("/data/status", a.CheckAppDataStatus)
	a.server.Post("/data/refresh", a.RefreshAppData)
	a.server.Listen(a.appInfo.AppAddr, iris.WithoutInterruptHandler, iris.WithoutServerError(iris.ErrServerClosed))
	<-idleConnsClosed
}

func (a *AppStruct) OnShutdown(hook func()) {
	a.shutdownHook = hook
}

func (a *AppStruct) WithDataVersion(version int) error {
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
	if a.isRUNC {
		transport.CloseBus()
	}
}

func (a *AppStruct) GetUGNList() []utils.UserGroupNamespace {
	if a.appInfo == nil {
		return nil
	}
	return a.appInfo.UGNList
}
