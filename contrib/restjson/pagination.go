package restjson

import (
	"github.com/kataras/iris/v12"

	"github.com/siyouyun-open/siyouyun_sdk/v2/paging"
)

// NewPagination parses the pagination parameters from the request query, defaulting to page=1
// and pageSize=10.
//
// The parameter names come from constants in the paging package: they are the contract agreed
// with the frontend.
func NewPagination(ctx iris.Context) *paging.Pagination {
	page := ctx.URLParamIntDefault(paging.PageCurrent, 1)
	pageSize := ctx.URLParamIntDefault(paging.PageSize, 10)
	return paging.New(page, pageSize)
}
