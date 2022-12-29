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
func (sos *storageOSApi) Open(path string) (*os.File, *net.UnixConn, string, error) {
	// 建立unix socket文件,链接并监听
	usuuid := uuid.New().String() + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	usuuidFp := filepath.Join(UnixSocketPrefix, usuuid)
	_, err := os.Create(usuuidFp)
	if err != nil {
		return nil, nil, "", err
	}
	err = syscall.Unlink(usuuidFp)
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	laddr, err := net.ResolveUnixAddr("unix", usuuidFp)
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	l, err := net.ListenUnix("unix", laddr)
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}

	go func() {
		// 发送开启文件请求
		api := sos.Host + "/open"
		resp := restclient.PostRequest[any](
			sos.UserNamespace,
			api,
			map[string]string{
				"parentPath": filepath.Dir(path),
				"name":       filepath.Base(path),
				"usuuid":     usuuid,
			},
			nil,
		)
		if resp.Code != sdkconst.Success {
			// 开启文件请求错误，停止unix监听
			_ = l.Close()
		}
	}()

	conn, err := l.AcceptUnix()
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}

	// msg分为两部分数据
	buf := make([]byte, 32)
	oob := make([]byte, 32)
	_, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	// 解出SocketControlMessage数组
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	if len(scms) == 0 {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", errors.New("scms is 0")
	}
	// 从SocketControlMessage中得到UnixRights
	fds, err := syscall.ParseUnixRights(&(scms[0]))
	if err != nil {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	// os.NewFile()将文件描述符转为 *os.File对象, 并不创建新文件, 通常很少使用到
	f := os.NewFile(uintptr(fds[0]), "")
	return f, conn, usuuidFp, nil
}

// OpenFile 打开或创建文件
func (sos *storageOSApi) OpenFile(path string, flag int, perm os.FileMode) (*os.File, *net.UnixConn, string, error) {
	// 建立unix socket文件,链接并监听
	usuuid := uuid.New().String() + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	usuuidFp := filepath.Join(UnixSocketPrefix, usuuid)
	_, err := os.Create(usuuidFp)
	if err != nil {
		return nil, nil, "", err
	}
	err = syscall.Unlink(usuuidFp)
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	laddr, err := net.ResolveUnixAddr("unix", usuuidFp)
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	l, err := net.ListenUnix("unix", laddr)
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}

	go func() {
		// 发送开启文件请求
		api := sos.Host + "/open/file"
		resp := restclient.PostRequest[any](sos.UserNamespace, api,
			map[string]string{
				"parentPath": filepath.Dir(path),
				"name":       filepath.Base(path),
				"usuuid":     usuuid,
				"flag":       strconv.Itoa(flag),
				"perm":       strconv.Itoa(int(perm)),
			},
			nil,
		)
		if resp.Code != sdkconst.Success {
			// 开启文件请求错误，停止unix监听
			_ = l.Close()
		}
	}()

	conn, err := l.AcceptUnix()
	if err != nil {
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}

	// msg分为两部分数据
	buf := make([]byte, 32)
	oob := make([]byte, 32)
	_, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	// 解出SocketControlMessage数组
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
	}
	if len(scms) == 0 {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", errors.New("scms is 0")
	}
	// 从SocketControlMessage中得到UnixRights
	fds, err := syscall.ParseUnixRights(&(scms[0]))
	if err != nil {
		_ = conn.Close()
		_ = os.Remove(usuuidFp)
		return nil, nil, "", err
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
	api := sos.Host + "/fs/file/exist"
	response := restclient.PostRequest[bool](
		sos.UserNamespace,
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
	api := sos.Host + "/fs/ensure/dir/exist"
	_ = restclient.PostRequest[any](sos.UserNamespace, api, map[string]string{"paths": strings.Join(ps, ",")}, nil)
}
