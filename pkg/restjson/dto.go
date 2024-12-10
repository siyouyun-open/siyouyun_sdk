package restjson

import (
	"errors"
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	syyerrors "github.com/siyouyun-open/siyouyun_sdk/pkg/sdkerr"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type Response[T any] struct {
	Code string `json:"code"`
	Data *T     `json:"data"`
	Msg  string `json:"msg"`
}

type PagingData struct {
	*utils.Pagination
	Total int64       `json:"total"`
	Data  interface{} `json:"data"`
}

func buildResponse[T any](code string, data *T, msg string) Response[T] {
	return Response[T]{
		Code: code,
		Data: data,
		Msg:  msg,
	}
}

func ResJson[T any](code string, data *T, msg string) Response[T] {
	return buildResponse[T](code, data, msg)
}

func SuccessResJson() Response[any] {
	return buildResponse[any](sdkconst.Success, nil, "ok")
}

func SuccessResJsonWithData[T any](data *T) Response[T] {
	return buildResponse[T](sdkconst.Success, data, "ok")
}

func SuccessResJsonWithPagingData(data *PagingData) Response[PagingData] {
	return buildResponse[PagingData](sdkconst.Success, data, "ok")
}

func ErrorResJson(code string, errMsg string) Response[any] {
	return buildResponse[any](code, nil, errMsg)
}

func ErrorResJsonWithMsg(errMsg string) Response[any] {
	return buildResponse[any](sdkconst.ServerError, nil, errMsg)
}

func ErrorResJsonWithError(ctx iris.Context, err error) Response[any] {
	if i18nErr := (*syyerrors.I18nError)(nil); errors.As(err, &i18nErr) {
		return buildResponse[any](sdkconst.ServerError, nil, ctx.Tr(i18nErr.Key, i18nErr.Args))
	}
	return buildResponse[any](sdkconst.ServerError, nil, err.Error())
}
