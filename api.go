package siyouyunsdk

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/router"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	rj "github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
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
		ctx.JSON(rj.ErrorResJsonWithMsg("no icon"))
		return
	}
	iconData, err := os.ReadFile(iconPath)
	if err != nil || len(iconData) == 0 {
		ctx.JSON(rj.ErrorResJsonWithMsg("read icon error"))
		return
	}
	io.Copy(ctx.ResponseWriter(), io.NopCloser(bytes.NewReader(iconData)))
}

// CheckAppDataStatus check app data status
func (a *AppStruct) CheckAppDataStatus(ctx iris.Context) {
	status := sdkdto.AppDataStatus{}
	if a.Ability == nil || a.Ability.kv == nil {
		ctx.JSON(rj.SuccessResJsonWithData(&status))
		return
	}
	ugn := utils.NewUserNamespaceFromIris(ctx)
	kv, ok := a.Ability.kv.GetKV(ugn, sdkconst.DefaultAppKeyType, sdkconst.AppDataVersionKey)
	if ok {
		status.CurrentVersion, _ = strconv.Atoi(kv.Value)
	}
	status.LatestVersion = a.dataVersion
	status.NeedRefresh = status.LatestVersion > status.CurrentVersion
	ctx.JSON(rj.SuccessResJsonWithData(&status))
}

// RefreshAppData refresh app data
func (a *AppStruct) RefreshAppData(ctx iris.Context) {
	url := utils.GetOSServiceURL() + "/app/data/refresh"
	if restclient.Client == nil {
		ctx.JSON(rj.ErrorResJsonWithMsg("not ready"))
		return
	}
	ugn := utils.NewUserNamespaceFromIris(ctx)
	resp := restclient.PostRequest[any](ugn, url, map[string]string{"appCode": a.AppCode}, nil)
	if resp.Code != sdkconst.Success {
		ctx.JSON(rj.ErrorResJsonWithMsg(resp.Msg))
		return
	}
	// update current data version
	if a.Ability != nil && a.Ability.kv != nil {
		_ = a.Ability.kv.PutKV(ugn, sdkconst.DefaultAppKeyType, sdkconst.AppDataVersionKey, strconv.Itoa(a.dataVersion))
	}
	ctx.JSON(rj.SuccessResJson())
}
