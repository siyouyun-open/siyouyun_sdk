package gateway

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
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
	*utils.UserNamespace
}

var storageOSGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, OSHTTPPort, "storage")

func newStorageOSApi(un *utils.UserNamespace) *storageOSApi {
	return &storageOSApi{
		Host:          storageOSGatewayAddr,
		UserNamespace: un,
	}
}

// Open  打开文件
func (sos *storageOSApi) Open(path string) (_ *os.File, _ *net.UnixConn, _ string, err error) {
	// 建立unix socket文件,链接并监听
	usuuid := uuid.New().String() + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	usuuidFp := filepath.Join(UnixSocketPrefix, usuuid)
	_, err = os.Create(usuuidFp)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	// 返回file对象
	err = syscall.Unlink(usuuidFp)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	laddr, err := net.ResolveUnixAddr("unix", usuuidFp)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	l, err := net.ListenUnix("unix", laddr)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	conn, err := l.AcceptUnix()
	if err != nil {
		return nil, nil, usuuidFp, err
	}

	// 发送开启文件请求
	api := sos.Host + "/open"
	response := restclient.PostRequest[any](
		sos.UserNamespace,
		api,
		map[string]string{
			"parentPath": filepath.Dir(path),
			"name":       filepath.Base(path),
			"usuuid":     usuuid,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return nil, nil, usuuidFp, errors.New(response.Msg)
	}

	// msg分为两部分数据
	buf := make([]byte, 32)
	oob := make([]byte, 32)
	_, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	// 解出SocketControlMessage数组
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	if len(scms) == 0 {
		return nil, nil, usuuidFp, errors.New("scms is 0")
	}
	// 从SocketControlMessage中得到UnixRights
	fds, err := syscall.ParseUnixRights(&(scms[0]))
	if err != nil {
		panic(err)
	}
	// os.NewFile()将文件描述符转为 *os.File对象, 并不创建新文件, 通常很少使用到
	f := os.NewFile(uintptr(fds[0]), "")
	return f, conn, usuuidFp, nil
}

// OpenFile 打开或创建文件
func (sos *storageOSApi) OpenFile(path string, flag int, perm os.FileMode) (_ *os.File, _ *net.UnixConn, _ string, err error) {
	// 建立unix socket文件,链接并监听
	usuuid := uuid.New().String() + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	usuuidFp := filepath.Join(UnixSocketPrefix, usuuid)
	_, err = os.Create(usuuidFp)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	err = syscall.Unlink(usuuidFp)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	laddr, err := net.ResolveUnixAddr("unix", usuuidFp)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	l, err := net.ListenUnix("unix", laddr)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	conn, err := l.AcceptUnix()
	if err != nil {
		return nil, nil, usuuidFp, err
	}

	// 发送开启文件请求
	api := sos.Host + "/open/file"
	response := restclient.PostRequest[any](
		sos.UserNamespace,
		api,
		map[string]string{
			"parentPath": filepath.Dir(path),
			"name":       filepath.Base(path),
			"usuuid":     usuuid,
			"flag":       strconv.Itoa(flag),
			"perm":       strconv.Itoa(int(perm)),
		},
		nil,
	)
	
	if response.Code != sdkconst.Success {
		return nil, nil, usuuidFp, errors.New(response.Msg)
	}
	// msg分为两部分数据
	buf := make([]byte, 32)
	oob := make([]byte, 32)
	_, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	// 解出SocketControlMessage数组
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, nil, usuuidFp, err
	}
	if len(scms) == 0 {
		return nil, nil, usuuidFp, errors.New("scms is 0")
	}
	// 从SocketControlMessage中得到UnixRights
	fds, err := syscall.ParseUnixRights(&(scms[0]))
	if err != nil {
		panic(err)
	}
	// os.NewFile()将文件描述符转为 *os.File对象, 并不创建新文件, 通常很少使用到
	f := os.NewFile(uintptr(fds[0]), "")
	return f, conn, usuuidFp, nil
}

// MkdirAll 递归创建目录
func (sos *storageOSApi) MkdirAll(path string) error {
	api := sos.Host + "/fs/mkdir"
	response := restclient.PostRequest[any](
		sos.UserNamespace,
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
	api := sos.Host + "/fs/remove"
	response := restclient.PostRequest[any](
		sos.UserNamespace,
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
	api := sos.Host + "/fs/rename"
	response := restclient.PostRequest[any](
		sos.UserNamespace,
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
	api := sos.Host + "/fs/chtimes"
	response := restclient.PostRequest[any](
		sos.UserNamespace,
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
	api := sos.Host + "/fs/object/exists"
	response := restclient.PostRequest[any](
		sos.UserNamespace,
		api,
		map[string]string{
			"parentPath": filepath.Dir(path),
			"name":       filepath.Base(path),
		},
		nil,
	)
	return response.Code == sdkconst.Success
}

// EnsureDirExist 确保目录存在
func (sos *storageOSApi) EnsureDirExist(ps ...string) {
	api := sos.Host + "/fs/ensure/dir/exist"
	_ = restclient.PostRequest[any](sos.UserNamespace, api, map[string]string{"paths": strings.Join(ps, ",")}, nil)
}
