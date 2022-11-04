package simpleapp

import (
	"github.com/siyouyun-open/siyouyun_sdk"
)

var app *siyouyunfaas.App

func Init() {
	app = siyouyunfaas.NewApp()
	AddRouter()
}

func AddRouter() {
	app.Api = make(siyouyunfaas.SiyouFaasApi)
	app.Api.Get("/test/un", TestUN)
	app.Api.Post("/test/un", TestUN)
	app.Api.Put("/test/un", TestUN)
	app.Api.Delete("/test/un", TestUN)
	app.Api.Get("/test/page", TestPage)
	app.Api.Get("/test/use/db", TestUseDB)
}
