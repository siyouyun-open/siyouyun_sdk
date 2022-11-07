package simpleapp

import (
	"fmt"
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
)

func TestUN(ctx iris.Context) {
	un := utils.NewUserNamespaceFromIris(ctx)
	ctx.JSON(restjson.SuccessResJson(fmt.Sprintf("[%v]%v-%v", ctx.Method(), un.Namespace, un.Username)))
}

func TestPage(ctx iris.Context) {
	page := utils.NewPaginationFromIris(ctx)
	ctx.JSON(restjson.SuccessResJson(fmt.Sprintf("page:%v;pageSize:%v", page.Page, page.PageSize)))
}

func TestUseDB(ctx iris.Context) {
	err := app.Exec(ctx, func(db *gorm.DB) error {
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
