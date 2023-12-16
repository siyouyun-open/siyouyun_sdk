package gateway

import (
	"encoding/json"
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type storageOSApi struct {
	Host string
	UGN  *utils.UserGroupNamespace
}

func newStorageOSApi(ugn *utils.UserGroupNamespace) *storageOSApi {
	return &storageOSApi{
		Host: OSURL + "/fs",
		UGN:  ugn,
	}
}

// Open  打开文件
func (sos *storageOSApi) Open(path string) (*os.File, error) {
	return sos.OpenFile(path, os.O_RDONLY, 0)
}

// OpenFile 打开或创建文件
func (sos *storageOSApi) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	laddr, err := net.ResolveUnixAddr("unix", UnixSocketFile)
	if err != nil {
		panic(err)
	}
	conn, err := net.DialUnix("unix", nil, laddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// 发送打开文件请求
	param := sdkdto.UnixOpenFileParam{
		Name: path,
		Flag: flag,
		Perm: perm,
	}
	marshal, _ := json.Marshal(param)
	req := &sdkdto.UnixFileOperateReq{
		UGN:      sos.UGN,
		Operator: sdkdto.UnixOpen,
		Param:    marshal,
	}
	encoder := json.NewEncoder(conn)
	err = encoder.Encode(req)
	if err != nil {
		return nil, err
	}
	return parseUnixFdResponse(conn)
}

// OpenAvatarFile 打开替身文件
func (sos *storageOSApi) OpenAvatarFile(path string) (*os.File, error) {
	laddr, err := net.ResolveUnixAddr("unix", UnixSocketFile)
	if err != nil {
		panic(err)
	}
	conn, err := net.DialUnix("unix", nil, laddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// 发送打开文件请求
	param := sdkdto.UnixOpenFileParam{
		Name:       path,
		Flag:       os.O_RDONLY,
		Perm:       0,
		WithAvatar: true,
	}
	marshal, _ := json.Marshal(param)
	req := &sdkdto.UnixFileOperateReq{
		UGN:      sos.UGN,
		Operator: sdkdto.UnixOpen,
		Param:    marshal,
	}
	encoder := json.NewEncoder(conn)
	err = encoder.Encode(req)
	if err != nil {
		return nil, err
	}
	return parseUnixFdResponse(conn)
}

// MkdirAll 递归创建目录
func (sos *storageOSApi) MkdirAll(path string) error {
	api := sos.Host + "/mkdir"
	response := restclient.PostRequest[any](
		sos.UGN,
		api,
		map[string]string{
			"parentPath": filepath.Dir(path),
			"name":       filepath.Base(path),
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// Remove 删除文件
func (sos *storageOSApi) Remove(path string) error {
	api := sos.Host + "/remove"
	response := restclient.PostRequest[any](
		sos.UGN,
		api,
		map[string]string{
			"parentPath": filepath.Dir(path),
			"name":       filepath.Base(path),
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// Rename 重命名文件
func (sos *storageOSApi) Rename(oldPath, newPath string) error {
	api := sos.Host + "/rename"
	response := restclient.PostRequest[any](
		sos.UGN,
		api,
		map[string]string{
			"parentPath":    filepath.Dir(oldPath),
			"name":          filepath.Base(oldPath),
			"newParentPath": filepath.Base(newPath),
			"newName":       filepath.Base(newPath),
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// Chtimes 修改文件时间
func (sos *storageOSApi) Chtimes(path string, atime time.Time, mtime time.Time) error {
	api := sos.Host + "/chtimes"
	response := restclient.PostRequest[any](
		sos.UGN,
		api,
		map[string]string{
			"parentPath": filepath.Dir(path),
			"name":       filepath.Base(path),
			"atime":      strconv.FormatInt(atime.UnixMilli(), 10),
			"mtime":      strconv.FormatInt(mtime.UnixMilli(), 10),
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// FileExists 文件是否存在
func (sos *storageOSApi) FileExists(path string) bool {
	api := sos.Host + "/file/exist"
	response := restclient.PostRequest[bool](
		sos.UGN,
		api,
		map[string]string{
			"parentPath": filepath.Dir(path),
			"name":       filepath.Base(path),
		},
		nil,
	)
	if response.Code == sdkconst.Success {
		return *response.Data
	}
	return false
}

// EnsureDirExist 确保目录存在
func (sos *storageOSApi) EnsureDirExist(ps ...string) {
	api := sos.Host + "/ensure/dir/exist"
	_ = restclient.PostRequest[any](sos.UGN, api, map[string]string{"paths": strings.Join(ps, ",")}, nil)
}

func parseUnixFdResponse(conn *net.UnixConn) (*os.File, error) {
	// 接收打开文件响应
	bufp := utils.ReadBufPool.Get().(*[]byte)
	buf := *bufp
	defer utils.ReadBufPool.Put(bufp)
	oob := make([]byte, 32)
	n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, err
	}
	// 处理响应数据
	var resp restjson.Response[any]
	_ = json.Unmarshal(buf[:n], &resp)
	if resp.Code != sdkconst.Success {
		return nil, errors.New(resp.Msg)
	}
	// 解出SocketControlMessage数组
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, err
	}
	if len(scms) == 0 {
		return nil, errors.New("打开文件失败")
	}
	// 从SocketControlMessage中得到UnixRights
	fds, err := syscall.ParseUnixRights(&(scms[0]))
	if err != nil {
		return nil, err
	}
	// os.NewFile()将文件描述符转为 *os.File对象
	f := os.NewFile(uintptr(fds[0]), "")
	return f, nil
}
