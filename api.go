package siyouyunsdk

import (
	"bytes"
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"io"
	"os"
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

// Alive 激活函数接口
func Alive(ctx iris.Context) {
	ctx.JSON(restjson.SuccessResJson("alive"))
}

// GetIcon 获取图标
func GetIcon(ctx iris.Context) {
	stat, err := os.Stat(IconPath)
	if err != nil || stat == nil || stat.IsDir() {
		ctx.JSON(restjson.ErrorResJsonWithMsg("未找到默认图标"))
		return
	}
	iconData, err := os.ReadFile(IconPath)
	if err != nil || len(iconData) == 0 {
		ctx.JSON(restjson.ErrorResJsonWithMsg("图标读取失败"))
		return
	}
	io.Copy(ctx.ResponseWriter(), io.NopCloser(bytes.NewReader(iconData)))
}
