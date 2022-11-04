package restjson

import (
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
)

// Response 四有云响应结构体
type Response[T any] struct {
	Code string `json:"code"`
	Data *T     `json:"data"`
	Msg  string `json:"msg"`
}

// PagingData 四有云分页数据
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

func SuccessResJson(msg string) Response[any] {
	return buildResponse[any](sdkconst.Success, nil, msg)
}

func SuccessResJsonWithData[T any](data *T, msg string) Response[T] {
	return buildResponse[T](sdkconst.Success, data, msg)
}

func SuccessResJsonWithPagingData(data *PagingData, msg string) Response[PagingData] {
	return buildResponse[PagingData](sdkconst.Success, data, msg)
}

func ErrorResJson(code string, errMsg string) Response[any] {
	return buildResponse[any](code, nil, errMsg)
}

func ErrorResJsonWithMsg(errMsg string) Response[any] {
	return buildResponse[any](sdkconst.ServerError, nil, errMsg)
}
