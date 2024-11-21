package siyouyunsdk

import (
	"bytes"
	"fmt"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"io"
	"os"
)

// GetAPIBuilder get web server api builder
func (a *AppStruct) GetAPIBuilder() *router.APIBuilder {
	if a.server == nil {
		return nil
	}
	return a.server.APIBuilder
}

// GetIcon get app icon file
func (a *AppStruct) GetIcon(ctx iris.Context) {
	iconPath := fmt.Sprintf("/siyouyun/app/miniapp/static/%s/icon.png", a.AppCode)
	stat, err := os.Stat(iconPath)
	if err != nil || stat == nil || stat.IsDir() {
		ctx.JSON(restjson.ErrorResJsonWithMsg("no icon"))
		return
	}
	iconData, err := os.ReadFile(iconPath)
	if err != nil || len(iconData) == 0 {
		ctx.JSON(restjson.ErrorResJsonWithMsg("read icon error"))
		return
	}
	io.Copy(ctx.ResponseWriter(), io.NopCloser(bytes.NewReader(iconData)))
}
