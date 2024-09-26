package ability

import (
	"errors"
	"fmt"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	rj "github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// SyyFS syy fs operations
type SyyFS struct {
	ugn       *utils.UserGroupNamespace
	appPrefix string
	db        *gorm.DB
}

func (fs *SyyFS) Open(ufi *utils.UFI) (File, error) {
	return fs.openFile(ufi, os.O_RDONLY, 0, false)
}

// OpenFile open file with privilege
func (fs *SyyFS) OpenFile(ufi *utils.UFI, flag int, perm os.FileMode) (File, error) {
	return fs.openFile(ufi, flag, perm, false)
}

// OpenAvatarFile open avatar file
func (fs *SyyFS) OpenAvatarFile(ufi *utils.UFI) (File, error) {
	return fs.openFile(ufi, os.O_RDONLY, 0, true)
}

func (fs *SyyFS) openFile(ufi *utils.UFI, flag int, perm os.FileMode, avatar bool) (File, error) {
	file := new(HTTPFile)
	file.ugn = fs.ugn
	res := rj.Response[bfsApiRet]{}
	_, err := restclient.Client.R().
		SetResult(&res).
		SetHeaders(map[string]string{
			sdkconst.UsernameHeader:  file.ugn.Username,
			sdkconst.GroupNameHeader: file.ugn.GroupName,
			sdkconst.NamespaceHeader: file.ugn.Namespace,
		}).
		SetQueryParams(map[string]string{
			"ufi":    ufi.Serialize(),
			"flag":   fmt.Sprintf("%d", flag),
			"perm":   fmt.Sprintf("%d", perm),
			"avatar": strconv.FormatBool(avatar),
		}).Get(utils.GetCoreServiceURL() + "/v2/faas/file/open")
	if err != nil || res.Data == nil {
		return nil, fsRequestErr
	}
	if res.Data.Error != "" {
		return nil, errors.New(res.Data.Error)
	}
	file.fd = res.Data.N
	return file, err
}

func (fs *SyyFS) MkdirAll(ufi *utils.UFI) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/mkdir/all"
	resp := restclient.PostRequest[any](fs.ugn, api, map[string]string{"ufi": ufi.Serialize()}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) Remove(ufi *utils.UFI) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/remove"
	resp := restclient.PostRequest[any](fs.ugn, api, map[string]string{"ufi": ufi.Serialize()}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) RemoveAll(ufi *utils.UFI) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/remove/all"
	resp := restclient.PostRequest[any](fs.ugn, api, map[string]string{"ufi": ufi.Serialize()}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) Rename(oldUFI *utils.UFI, newUFI *utils.UFI) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/rename"
	resp := restclient.PostRequest[any](fs.ugn, api,
		map[string]string{
			"ufi1": oldUFI.Serialize(),
			"ufi2": newUFI.Serialize(),
		}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) Chtimes(ufi *utils.UFI, atime time.Time, mtime time.Time) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/chtimes"
	resp := restclient.PostRequest[any](fs.ugn, api,
		map[string]string{
			"ufi":   ufi.Serialize(),
			"atime": strconv.FormatInt(atime.UnixMilli(), 10),
			"mtime": strconv.FormatInt(mtime.UnixMilli(), 10),
		}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) FileExists(ufi *utils.UFI) bool {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/exists"
	resp := restclient.GetRequest[bool](fs.ugn, api, map[string]string{"ufi": ufi.Serialize()})
	if resp.Code != sdkconst.Success {
		return false
	}
	return *resp.Data
}

func (fs *SyyFS) Exec(f func(*gorm.DB) error) error {
	err := fs.db.Transaction(func(tx *gorm.DB) (err error) {
		dbname := fs.ugn.DatabaseName()
		if dbname == "" {
			return
		}
		err = tx.Exec("use " + dbname).Error
		if err != nil {
			return err
		}
		err = f(tx)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (fs *SyyFS) AppOpenFile(path string, flag int, perm os.FileMode) (File, error) {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SystemPool, utils.PathUFI, filepath.Join(fs.appPrefix, path))
	return fs.openFile(ufi, flag, perm, false)
}

func (fs *SyyFS) AppMkdirAll(path string) error {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SystemPool, utils.PathUFI, filepath.Join(fs.appPrefix, path))
	return fs.MkdirAll(ufi)
}

func (fs *SyyFS) AppRemoveAll(path string) error {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SystemPool, utils.PathUFI, filepath.Join(fs.appPrefix, path))
	return fs.RemoveAll(ufi)
}

func (fs *SyyFS) AppFileExists(path string) bool {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SystemPool, utils.PathUFI, filepath.Join(fs.appPrefix, path))
	return fs.FileExists(ufi)
}
