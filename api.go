package siyouyunfaas

import (
	"github.com/kataras/iris/v12"
)

type SiyouFaasApi map[string]func(iris.Context)

func (api SiyouFaasApi) Get(uri string, f func(iris.Context)) {
	api[iris.MethodGet+" "+uri] = f
}

func (api SiyouFaasApi) Post(uri string, f func(iris.Context)) {
	api[iris.MethodPost+" "+uri] = f
}

func (api SiyouFaasApi) Put(uri string, f func(iris.Context)) {
	api[iris.MethodPut+" "+uri] = f
}

func (api SiyouFaasApi) Delete(uri string, f func(iris.Context)) {
	api[iris.MethodDelete+" "+uri] = f
}

func (api SiyouFaasApi) Exec(uri string, ctx iris.Context) {
	api[iris.MethodDelete+" "+uri](ctx)
}
