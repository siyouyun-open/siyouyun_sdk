package restclient

import (
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/restjson"
	"net"
	"net/http"
	"strings"
)

var Client *resty.Client

func InitHttpClient() {
	Client = resty.New()
}

// PostRequest 发起rest post请求
func PostRequest[T any](fullApi string, query map[string]string, body any) restjson.Response[T] {
	if query == nil {
		query = map[string]string{}
	}
	resp := restjson.Response[T]{}
	_, err := Client.R().
		SetQueryParams(query).
		SetHeaders(map[string]string{
			"Accept": "application/json",
		}).
		SetBody(body).
		SetResult(&resp).
		Post(fullApi)
	if err != nil {
		return restjson.ResJson[T](sdkconst.RPCError, nil, fmt.Sprintf("远程调用错误:%v", err))
	}
	return resp
}

// GetRequest 发起rest get请求
func GetRequest[T any](fullApi string, query map[string]string) restjson.Response[T] {
	if query == nil {
		query = map[string]string{}
	}
	resp := restjson.Response[T]{}
	_, err := Client.R().
		SetQueryParams(query).
		SetHeaders(map[string]string{
			"Accept": "application/json",
		}).
		SetResult(&resp).
		Get(fullApi)
	if err != nil {
		return restjson.ResJson[T](sdkconst.RPCError, nil, fmt.Sprintf("远程调用错误:%v", err))
	}
	return resp
}

// GetIP returns request real ip.
func GetIP(r *http.Request) (string, error) {
	ip := r.Header.Get("X-Real-IP")
	if net.ParseIP(ip) != nil {
		return ip, nil
	}

	ip = r.Header.Get("X-Forward-For")
	for _, i := range strings.Split(ip, ",") {
		if net.ParseIP(i) != nil {
			return i, nil
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}

	if net.ParseIP(ip) != nil {
		return ip, nil
	}

	return "", errors.New("no valid ip found")
}
