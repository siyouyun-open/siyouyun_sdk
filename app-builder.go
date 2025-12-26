package siyouyunsdk

import (
	"os"

	"github.com/kataras/iris/v12"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
)

// AppBuilder app builder
type AppBuilder struct {
	app *AppStruct
}

// NewAppBuilder new a custom app builder
func NewAppBuilder(appCode string) *AppBuilder {
	var err error
	customApp := &AppStruct{}

	// init log
	sdklog.InitLogger(logrus.InfoLevel)

	// init http client
	restclient.InitHttpClient()

	// get app info
	if appCode == "" {
		appCode = os.Getenv(AppCodeEnvKey)
		if appCode == "" {
			panic("appCode empty")
		}
	}
	customApp.AppCode = appCode
	customApp.appInfo, err = gateway.GetAppInfo(appCode)
	if err != nil {
		panic(err)
	}

	// init ability
	customApp.Ability = &Ability{}

	return &AppBuilder{
		app: customApp,
	}
}

func (b *AppBuilder) WithWebServer() *AppBuilder {
	b.app.server = iris.New()
	return b
}

func (b *AppBuilder) WithNC(nc *nats.Conn) *AppBuilder {
	b.app.nc = nc
	return b
}

func (b *AppBuilder) WithDB(db *gorm.DB) *AppBuilder {
	b.app.db = db
	return b
}

func (b *AppBuilder) Build() {
	App = b.app
}
