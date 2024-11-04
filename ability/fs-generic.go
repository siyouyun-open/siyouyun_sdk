package ability

import (
	"errors"
	"fmt"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
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

func (fs *SyyFS) GetUGN() *utils.UserGroupNamespace {
	return fs.ugn
}

func (fs *SyyFS) GetDB() *gorm.DB {
	return fs.db
}

func (fs *SyyFS) Open(ufi string) (File, error) {
	return fs.openFile(ufi, os.O_RDONLY, 0, false)
}

// OpenFile open file with privilege
func (fs *SyyFS) OpenFile(ufi string, flag int, perm os.FileMode) (File, error) {
	return fs.openFile(ufi, flag, perm, false)
}

// OpenAvatarFile open avatar file
func (fs *SyyFS) OpenAvatarFile(ufi string) (File, error) {
	return fs.openFile(ufi, os.O_RDONLY, 0, true)
}

func (fs *SyyFS) openFile(ufi string, flag int, perm os.FileMode, avatar bool) (File, error) {
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
			"ufi":    ufi,
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

func (fs *SyyFS) Stat(ufi string) (*sdkdto.SiyouFileInfo, error) {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/object/detail"
	resp := restclient.GetRequest[sdkdto.SiyouFileInfo](fs.ugn, api, map[string]string{"ufi": ufi})
	if resp.Code != sdkconst.Success {
		return nil, errors.New(resp.Msg)
	}
	return resp.Data, nil
}

func (fs *SyyFS) MkdirAll(ufi string) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/mkdir/all"
	resp := restclient.PostRequest[any](fs.ugn, api, map[string]string{"ufi": ufi}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) Remove(ufi string) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/remove"
	resp := restclient.PostRequest[any](fs.ugn, api, map[string]string{"ufi": ufi}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) RemoveAll(ufi string) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/remove/all"
	resp := restclient.PostRequest[any](fs.ugn, api, map[string]string{"ufi": ufi}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) Rename(oldUFI string, newUFI string) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/rename"
	resp := restclient.PostRequest[any](fs.ugn, api,
		map[string]string{
			"ufi1": oldUFI,
			"ufi2": newUFI,
		}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) Chtimes(ufi string, atime time.Time, mtime time.Time) error {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/chtimes"
	resp := restclient.PostRequest[any](fs.ugn, api,
		map[string]string{
			"ufi":   ufi,
			"atime": strconv.FormatInt(atime.UnixMilli(), 10),
			"mtime": strconv.FormatInt(mtime.UnixMilli(), 10),
		}, nil)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (fs *SyyFS) FileExists(ufi string) bool {
	api := utils.GetCoreServiceURL() + "/v2/faas/fs/exists"
	resp := restclient.GetRequest[bool](fs.ugn, api, map[string]string{"ufi": ufi})
	if resp.Code != sdkconst.Success {
		return false
	}
	return *resp.Data
}

func (fs *SyyFS) Exec(f func(*gorm.DB) error, transactional ...bool) error {
	if fs.ugn == nil {
		return errors.New("ugn is empty")
	}
	flag := true // default use transaction
	if len(transactional) > 0 {
		flag = transactional[0]
	}
	if flag {
		return fs.db.Transaction(func(tx *gorm.DB) error {
			err := tx.Exec(fmt.Sprintf("SET search_path TO %s, public;", fs.ugn.SchemaName())).Error
			if err != nil {
				return err
			}
			return f(tx)
		})
	} else {
		session := fs.db.Session(&gorm.Session{})
		err := session.Exec(fmt.Sprintf("SET search_path TO %s, public;", fs.ugn.SchemaName())).Error
		if err != nil {
			return err
		}
		return f(session)
	}
}

func (fs *SyyFS) AppOpenFile(path string, flag int, perm os.FileMode) (File, error) {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SiyouSysPool, filepath.Join(fs.appPrefix, path))
	return fs.openFile(ufi.Serialize(), flag, perm, false)
}

func (fs *SyyFS) AppMkdirAll(path string) error {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SiyouSysPool, filepath.Join(fs.appPrefix, path))
	return fs.MkdirAll(ufi.Serialize())
}

func (fs *SyyFS) AppRemoveAll(path string) error {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SiyouSysPool, filepath.Join(fs.appPrefix, path))
	return fs.RemoveAll(ufi.Serialize())
}

func (fs *SyyFS) AppFileExists(path string) bool {
	ufi := utils.NewUFI(utils.UFSMeta, sdkconst.SiyouSysPool, filepath.Join(fs.appPrefix, path))
	return fs.FileExists(ufi.Serialize())
}
