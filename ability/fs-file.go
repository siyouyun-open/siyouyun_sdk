package ability

import (
	"encoding/json"
	"errors"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	rj "github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"io"
	"strconv"
)

var (
	fsRequestErr = errors.New("fs request error")
)

type bfsApiRet struct {
	N       int64  `json:"n"`
	Error   string `json:"error"`
	Content []byte `json:"content"`
}

// HTTPFile file implement by http
type HTTPFile struct {
	ugn *utils.UserGroupNamespace
	fd  int64
}

func (H *HTTPFile) Close() error {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
		}).Post(utils.GetCoreServiceURL() + "/v2/faas/file/close")
	if err != nil || res.Data == nil {
		return fsRequestErr
	}
	if res.Data.Error != "" {
		return errors.New(res.Data.Error)
	}
	return nil
}

func (H *HTTPFile) Read(p []byte) (int, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd":     strconv.FormatInt(H.fd, 10),
			"bufLen": strconv.Itoa(len(p)),
		}).Get(utils.GetCoreServiceURL() + "/v2/faas/file/read")
	if err != nil || res.Data == nil {
		return 0, fsRequestErr
	}
	if res.Data.Error != "" && res.Data.Error != io.EOF.Error() {
		return 0, errors.New(res.Data.Error)
	}
	copy(p, res.Data.Content)
	if res.Data.Error == io.EOF.Error() {
		return int(res.Data.N), io.EOF
	}
	return int(res.Data.N), nil
}

func (H *HTTPFile) ReadAt(p []byte, off int64) (int, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd":     strconv.FormatInt(H.fd, 10),
			"bufLen": strconv.Itoa(len(p)),
			"offset": strconv.FormatInt(off, 10),
		}).Get(utils.GetCoreServiceURL() + "/v2/faas/file/read/at")
	if err != nil || res.Data == nil {
		return 0, fsRequestErr
	}
	if res.Data.Error != "" && res.Data.Error != io.EOF.Error() {
		return 0, errors.New(res.Data.Error)
	}
	copy(p, res.Data.Content)
	if res.Data.Error == io.EOF.Error() {
		return int(res.Data.N), io.EOF
	}
	return int(res.Data.N), nil
}

func (H *HTTPFile) Seek(offset int64, whence int) (int64, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd":     strconv.FormatInt(H.fd, 10),
			"offset": strconv.FormatInt(offset, 10),
			"whence": strconv.Itoa(whence),
		}).Post(utils.GetCoreServiceURL() + "/v2/faas/file/seek")
	if err != nil || res.Data == nil {
		return 0, fsRequestErr
	}
	if res.Data.Error != "" {
		return res.Data.N, errors.New(res.Data.Error)
	}
	return res.Data.N, nil
}

func (H *HTTPFile) Write(p []byte) (int, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetBody(p).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
		}).Post(utils.GetCoreServiceURL() + "/v2/faas/file/write")
	if err != nil || res.Data == nil {
		return 0, fsRequestErr
	}
	if res.Data.Error != "" {
		return int(res.Data.N), errors.New(res.Data.Error)
	}
	return int(res.Data.N), nil
}

func (H *HTTPFile) WriteAt(p []byte, off int64) (int, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetBody(p).
		SetQueryParams(map[string]string{
			"fd":     strconv.FormatInt(H.fd, 10),
			"offset": strconv.FormatInt(off, 10),
		}).Post(utils.GetCoreServiceURL() + "/v2/faas/file/write/at")
	if err != nil || res.Data == nil {
		return 0, fsRequestErr
	}
	if res.Data.Error != "" {
		return int(res.Data.N), errors.New(res.Data.Error)
	}
	return int(res.Data.N), nil
}

func (H *HTTPFile) Name() string {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
		}).Get(utils.GetCoreServiceURL() + "/v2/faas/file/name")
	if err != nil || res.Data == nil {
		return ""
	}
	if res.Data.Error != "" {
		return ""
	}
	return string(res.Data.Content)
}

func (H *HTTPFile) Readdir(n int) ([]*sdkdto.SiyouFileBasicInfo, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
			"n":  strconv.Itoa(n),
		}).Get(utils.GetCoreServiceURL() + "/v2/faas/file/readdir")
	if err != nil || res.Data == nil {
		return nil, fsRequestErr
	}
	if res.Data.Error != "" {
		return nil, errors.New(res.Data.Error)
	}
	var resJson []*sdkdto.SiyouFileBasicInfo
	err = json.Unmarshal(res.Data.Content, &resJson)
	if err != nil {
		return nil, err
	}
	return resJson, nil
}

func (H *HTTPFile) Readdirnames(n int) ([]string, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
			"n":  strconv.Itoa(n),
		}).Get(utils.GetCoreServiceURL() + "/v2/faas/file/readdirnames")
	if err != nil || res.Data == nil {
		return nil, fsRequestErr
	}
	if res.Data.Error != "" {
		return nil, errors.New(res.Data.Error)
	}
	var resJson []string
	err = json.Unmarshal(res.Data.Content, &resJson)
	if err != nil {
		return nil, err
	}
	return resJson, nil
}

func (H *HTTPFile) Stat() (*sdkdto.SiyouFileBasicInfo, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
		}).Get(utils.GetCoreServiceURL() + "/v2/faas/file/stat")
	if err != nil || res.Data == nil {
		return nil, fsRequestErr
	}
	if res.Data.Error != "" {
		return nil, errors.New(res.Data.Error)
	}
	var resJson sdkdto.SiyouFileBasicInfo
	err = json.Unmarshal(res.Data.Content, &resJson)
	if err != nil {
		return nil, err
	}
	return &resJson, nil
}

func (H *HTTPFile) Sync() error {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
		}).Post(utils.GetCoreServiceURL() + "/v2/faas/file/sync")
	if err != nil || res.Data == nil {
		return fsRequestErr
	}
	if res.Data.Error != "" {
		return errors.New(res.Data.Error)
	}
	return nil
}

func (H *HTTPFile) Truncate(size int64) error {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"fd":   strconv.FormatInt(H.fd, 10),
			"size": strconv.FormatInt(size, 10),
		}).Post(utils.GetCoreServiceURL() + "/v2/faas/file/truncate")
	if err != nil || res.Data == nil {
		return fsRequestErr
	}
	if res.Data.Error != "" {
		return errors.New(res.Data.Error)
	}
	return nil
}

func (H *HTTPFile) WriteString(s string) (int, error) {
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  H.ugn.Username,
			sdkconst.GroupNameHeader: H.ugn.GroupName,
			sdkconst.NamespaceHeader: H.ugn.Namespace,
		}).
		SetBody([]byte(s)).
		SetQueryParams(map[string]string{
			"fd": strconv.FormatInt(H.fd, 10),
		}).Post(utils.GetCoreServiceURL() + "/v2/faas/file/write/string")
	if err != nil || res.Data == nil {
		return 0, fsRequestErr
	}
	if res.Data.Error != "" {
		return int(res.Data.N), errors.New(res.Data.Error)
	}
	return int(res.Data.N), nil
}
