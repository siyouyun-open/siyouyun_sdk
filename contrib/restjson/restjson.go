// Package restjson provides the unified response envelope applications expose:
// `{"code": ..., "data": ..., "msg": ...}`.
//
// Why the new SDK still needs it: the webui and frontend of the four FaaS applications unwrap
// this envelope, and swapping the SDK must not casually swap the protocol. In the old SDK
// this package was `pkg/restjson` and was aliased to `rj` in applications; here the shape and
// semantics stay **identical field by field and value by value**, so application-side changes
// can be limited to the import path.
//
// Why it is a separate module (rather than being folded into the appsdk main package): it
// depends on iris, while the appsdk main package deliberately binds no Web framework — an
// application that writes no HTTP service and only runs background jobs should not be forced
// to pull in iris. This is the same tradeoff as protocol being its own module.
package restjson

import (
	"errors"

	"github.com/kataras/iris/v12"

	"github.com/siyouyun-open/siyouyun_sdk/v2/appconst"
	"github.com/siyouyun-open/siyouyun_sdk/v2/apperr"
	"github.com/siyouyun-open/siyouyun_sdk/v2/paging"
)

// Response is the unified response envelope.
//
// Data is a pointer: `null` and a "zero-value object" are two different semantics on the
// client, and a success with no data must be null.
type Response[T any] struct {
	Code string `json:"code"`
	Data *T     `json:"data"`
	Msg  string `json:"msg"`
}

// PagingData is the data part of a paging response.
//
// Pagination is embedded anonymously: page / pageSize must be flattened onto the same level as
// total / data, which is the shape the frontend already uses.
type PagingData struct {
	*paging.Pagination
	Total int64       `json:"total"`
	Data  interface{} `json:"data"`
}

func buildResponse[T any](code string, data *T, msg string) Response[T] {
	return Response[T]{Code: code, Data: data, Msg: msg}
}

// ResJson builds a response with the given code.
func ResJson[T any](code string, data *T, msg string) Response[T] {
	return buildResponse[T](code, data, msg)
}

// SuccessResJson builds a success response with no data.
func SuccessResJson() Response[any] {
	return buildResponse[any](appconst.Success, nil, "ok")
}

// SuccessResJsonWithData builds a success response carrying data.
func SuccessResJsonWithData[T any](data *T) Response[T] {
	return buildResponse[T](appconst.Success, data, "ok")
}

// SuccessResJsonWithPagingData builds a paging success response.
func SuccessResJsonWithPagingData(data *PagingData) Response[PagingData] {
	return buildResponse[PagingData](appconst.Success, data, "ok")
}

// ErrorResJson builds an error response with the given code.
func ErrorResJson(code string, errMsg string) Response[any] {
	return buildResponse[any](code, nil, errMsg)
}

// ErrorResJsonWithMsg builds a server error response (the code is fixed to ServerError).
func ErrorResJsonWithMsg(errMsg string) Response[any] {
	return buildResponse[any](appconst.ServerError, nil, errMsg)
}

// ErrorResJsonWithError renders by error type: apperr.I18nError is translated using the
// language of the current request, and any other error uses err.Error() directly.
//
// One **intentional difference** from the old implementation: the old code passed
// `i18nErr.Args` ([]any) to ctx.Tr as a single argument, which actually became "one []any
// argument", so messages with placeholders rendered as `%!v(MISSING)`. Here `args...` expands
// correctly — the envelope shape is unchanged, and messages with arguments simply render for
// real.
func ErrorResJsonWithError(ctx iris.Context, err error) Response[any] {
	if i18nErr := (*apperr.I18nError)(nil); errors.As(err, &i18nErr) {
		return buildResponse[any](appconst.ServerError, nil, ctx.Tr(i18nErr.Key, i18nErr.Args...))
	}
	if err == nil {
		return buildResponse[any](appconst.ServerError, nil, "")
	}
	return buildResponse[any](appconst.ServerError, nil, err.Error())
}
