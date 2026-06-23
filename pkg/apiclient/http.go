package apiclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

// HTTPClient is the raw resty HTTP client for direct HTTP access.
// Only available in host mode; nil in RUNC mode.
var HTTPClient *resty.Client

func initHTTPClient() {
	rc := resty.New()
	rc.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).
		SetRetryCount(2).
		SetRetryWaitTime(1 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return r.StatusCode() == http.StatusInternalServerError
		})
	HTTPClient = rc
}

// httpClient is the host-mode transport that makes real HTTP requests.
type httpClient struct {
	client *resty.Client
}

func newHTTPClient() *httpClient {
	initHTTPClient()
	return &httpClient{client: HTTPClient}
}

func (c *httpClient) Do(method, path string, ugn *utils.UserGroupNamespace, query map[string]string, body any) ([]byte, error) {
	header := make(map[string]string)
	header["Accept"] = "application/json"
	if ugn != nil {
		header[sdkconst.UsernameHeader] = ugn.Username
		header[sdkconst.GroupNameHeader] = ugn.GroupName
		header[sdkconst.NamespaceHeader] = ugn.Namespace
	}

	ip := utils.GetIPByEnv()
	port := "40100"
	if strings.HasPrefix(path, "/os/") || strings.HasPrefix(path, "/os") {
		port = "40000"
	}
	fullURL := fmt.Sprintf("http://%s:%s%s", ip, port, path)

	if query == nil {
		query = map[string]string{}
	}

	req := c.client.R().
		SetQueryParams(query).
		SetHeaders(header)

	if body != nil {
		req.SetBody(body)
	}

	var resp *resty.Response
	var err error
	switch method {
	case "GET":
		resp, err = req.Get(fullURL)
	default:
		resp, err = req.Post(fullURL)
	}
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, fullURL, err)
	}
	return resp.Body(), nil
}
