package simpleapp

import (
	"github.com/siyouyun-open/siyouyun_sdk"
)

func Init() {
	siyouyunsdk.NewApp()
	AddRouter()
}

func AddRouter() {
	siyouyunsdk.App.Api.Get("/test/un", TestUN)
	siyouyunsdk.App.Api.Post("/test/un", TestUN)
	siyouyunsdk.App.Api.Put("/test/un", TestUN)
	siyouyunsdk.App.Api.Delete("/test/un", TestUN)
	siyouyunsdk.App.Api.Get("/test/page", TestPage)
	siyouyunsdk.App.Api.Get("/test/use/db", TestUseDB)
	siyouyunsdk.App.Api.Get("/test/use/file", TestUseFile)
}
