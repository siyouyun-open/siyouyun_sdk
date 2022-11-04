package api

import "github.com/kataras/iris/v12"

type SiyouFaasApi map[string]func(iris.Context)

var ApiExport = SiyouFaasApi{}

func (api SiyouFaasApi) AddRouter(uri string, f func(iris.Context)) {
	api[uri] = f
}
