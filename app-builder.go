package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"gorm.io/gorm"
)

// AppBuilder app builder
type AppBuilder struct {
	app *AppStruct
}

// NewAppBuilder new a custom app builder
func NewAppBuilder() *AppBuilder {
	var err error
	customApp := &AppStruct{}

	// init the necessary things
	// init http client
	restclient.InitHttpClient()
	// get app info
	customApp.appInfo, err = gateway.GetAppInfo(App.AppCode)
	if err != nil {
		panic(err)
	}
	return &AppBuilder{
		app: customApp,
	}
}

func (b *AppBuilder) WithAppCode(appCode string) *AppBuilder {
	b.app.AppCode = appCode
	return b
}

func (b *AppBuilder) WithApi(api SiyouFaaSApi) *AppBuilder {
	b.app.Api = api
	return b
}

func (b *AppBuilder) WithDB(db *gorm.DB) *AppBuilder {
	b.app.db = db
	return b
}

func (b *AppBuilder) Build() *AppStruct {
	return b.app
}
