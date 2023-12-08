package simpleapp

import (
	"fmt"
	"github.com/kataras/iris/v12"
	siyouyunsdk "github.com/siyouyun-open/siyouyun_sdk"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"io"
)

func TestUGN(ctx iris.Context) {
	ugn := utils.NewUserNamespaceFromIris(ctx)
	ctx.JSON(restjson.SuccessResJson(fmt.Sprintf("[%v]%v-%v", ctx.Method(), ugn.Namespace, ugn.Username)))
}

func TestPage(ctx iris.Context) {
	page := utils.NewPaginationFromIris(ctx)
	ctx.JSON(restjson.SuccessResJson(fmt.Sprintf("page:%v;pageSize:%v", page.Page, page.PageSize)))
}

func TestUseDB(ctx iris.Context) {
	fs := siyouyunsdk.App.NewFSFromCtx(ctx)
	err := fs.Exec(func(db *gorm.DB) error {
		var apps []Apps
		err := db.Find(&apps).Error
		if err != nil {
			return err
		}
		fmt.Printf("%+v", apps)
		return nil
	})
	if err != nil {
		ctx.JSON(restjson.ErrorResJsonWithMsg("error"))
		return
	}
	ctx.JSON(restjson.SuccessResJson("success"))
}

func TestUseFile(ctx iris.Context) {
	fs := siyouyunsdk.App.NewFSFromCtx(ctx)
	appfs := siyouyunsdk.App.NewAppFSFromCtx(ctx)
	f1, _ := fs.Open("download-1.jpg")
	f2, _ := appfs.Open("123")
	io.Copy(f2, f1)
	f1.Close()
	f2.Close()
}

type Model struct {
	ID        uint  `json:"id" gorm:"primarykey;comment:ID"`
	CreatedAt int64 `json:"createdAt" gorm:"type:bigint(20);not null;autoUpdateTime:milli;comment:创建时间"`
	UpdatedAt int64 `json:"updatedAt" gorm:"type:bigint(20);not null;autoUpdateTime:milli;comment:更新时间"`
}

type Apps struct {
	Model
	CodeName    string `gorm:"type:varchar(255);comment:程序标识"`
	Name        string `gorm:"type:varchar(255);comment:程序名称"`
	Description string `gorm:"type:text;comment:描述"`
}

func (Apps) TableName() string {
	return "siyou_apps"
}
