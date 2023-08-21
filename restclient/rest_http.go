package restclient

import (
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"net"
	"net/http"
	"strings"
)

var Client *resty.Client

func InitHttpClient() {
	Client = resty.New()
}

// PostRequest 发起rest post请求
func PostRequest[T any](un *utils.UserGroupNamespace, fullApi string, query map[string]string, body any) restjson.Response[T] {
	if query == nil {
		query = map[string]string{}
	}
	header := make(map[string]string)
	header["Accept"] = "application/json"
	if un != nil {
		header[sdkconst.UsernameHeader] = un.Username
		header[sdkconst.GroupNameHeader] = un.GroupName
		header[sdkconst.NamespaceHeader] = un.Namespace
	}
	resp := restjson.Response[T]{}
	req := Client.R().
		SetQueryParams(query).
		SetHeaders(header).
		SetResult(&resp)
	if body != nil {
		req.SetBody(body)
	}
	_, err := req.Post(fullApi)
	if err != nil {
		return restjson.ResJson[T](sdkconst.RPCError, nil, fmt.Sprintf("远程调用错误:%v", err))
	}
	return resp
}

// GetRequest 发起rest get请求
func GetRequest[T any](un *utils.UserGroupNamespace, fullApi string, query map[string]string) restjson.Response[T] {
	if query == nil {
		query = map[string]string{}
	}
	header := make(map[string]string)
	header["Accept"] = "application/json"
	if un != nil {
		header[sdkconst.UsernameHeader] = un.Username
		header[sdkconst.GroupNameHeader] = un.GroupName
		header[sdkconst.NamespaceHeader] = un.Namespace
	}
	resp := restjson.Response[T]{}
	_, err := Client.R().
		SetQueryParams(query).
		SetHeaders(header).
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
