package utils

import "github.com/kataras/iris/v12"

const (
	PageCurrent = "page"
	PageSize    = "pageSize"
)

type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func NewPaginationFromIris(ctx iris.Context) *Pagination {
	page := ctx.URLParamIntDefault(PageCurrent, 1)
	if page <= 0 {
		page = 1
	}
	pageSize := ctx.URLParamIntDefault(PageSize, 10)
	return &Pagination{
		Page:     page,
		PageSize: pageSize,
	}
}
