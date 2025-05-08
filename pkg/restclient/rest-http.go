package restclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

var Client *resty.Client

func InitHttpClient() {
	Client = resty.New()
	Client.
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).
		SetRetryCount(2).
		SetRetryWaitTime(1 * time.Second).
		AddRetryCondition(
			func(r *resty.Response, err error) bool {
				return r.StatusCode() == http.StatusInternalServerError
			},
		)
}

// PostRequest 发起rest post请求
func PostRequest[T any](ugn *utils.UserGroupNamespace, fullApi string, query map[string]string, body any) restjson.Response[T] {
	if query == nil {
		query = map[string]string{}
	}
	header := make(map[string]string)
	header["Accept"] = "application/json"
	if ugn != nil {
		header[sdkconst.UsernameHeader] = ugn.Username
		header[sdkconst.GroupNameHeader] = ugn.GroupName
		header[sdkconst.NamespaceHeader] = ugn.Namespace
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
func GetRequest[T any](ugn *utils.UserGroupNamespace, fullApi string, query map[string]string) restjson.Response[T] {
	if query == nil {
		query = map[string]string{}
	}
	header := make(map[string]string)
	header["Accept"] = "application/json"
	if ugn != nil {
		header[sdkconst.UsernameHeader] = ugn.Username
		header[sdkconst.GroupNameHeader] = ugn.GroupName
		header[sdkconst.NamespaceHeader] = ugn.Namespace
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
