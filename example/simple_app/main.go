package simpleapp

import (
	"github.com/siyouyun-open/siyouyun_sdk"
)

func Init() {
	siyouyunsdk.NewApp()
	AddRouter()
}

func AddRouter() {
	siyouyunsdk.App.Api.Get("/test/ugn", TestUGN)
	siyouyunsdk.App.Api.Post("/test/ugn", TestUGN)
	siyouyunsdk.App.Api.Put("/test/ugn", TestUGN)
	siyouyunsdk.App.Api.Delete("/test/ugn", TestUGN)
	siyouyunsdk.App.Api.Get("/test/page", TestPage)
	siyouyunsdk.App.Api.Get("/test/use/db", TestUseDB)
	siyouyunsdk.App.Api.Get("/test/use/file", TestUseFile)
}
