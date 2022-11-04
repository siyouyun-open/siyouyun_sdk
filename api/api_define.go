package api

import (
	"github.com/kataras/iris/v12"
)

type SiyouFaasApi map[string]func(iris.Context)

func NewSiyouyunApi() SiyouFaasApi {
	var api SiyouFaasApi
	// maybe do something
	return api
}

func (api SiyouFaasApi) AddRouter(uri string, f func(iris.Context)) {
	api[uri] = f
}
