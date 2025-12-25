package utils

import (
	"errors"
	"path/filepath"
	"strings"
)

type UFIProtocol string

const (
	UFIv1     UFIProtocol = "ufi"
	Separator             = "/"
)

func (U UFIProtocol) String() string {
	return string(U)
}

type StorageType string

const (
	RawDisk     StorageType = "raw-disk"
	USB         StorageType = "usb"
	Alipan      StorageType = "alipan"
	Baiduyun    StorageType = "baiduyun"
	GoogleDrive StorageType = "google-drive"
	CommonOSS   StorageType = "oss"
	AliyunOSS   StorageType = "aliyun-oss"
	TencentOSS  StorageType = "tencent-oss"
	SambaClient StorageType = "smb-client"
	UFSMeta     StorageType = "pool-meta"
	UFSRaw      StorageType = "pool-raw"
	UFSSandbox  StorageType = "pool-sandbox"
	Snapshot    StorageType = "pool-snapshot"
	Trash       StorageType = "pool-trash"
	Webdav      StorageType = "webdav"
	FTP         StorageType = "ftp"
)

func (t StorageType) String() string {
	return string(t)
}

func (t StorageType) IsSiyouyunStorage() bool {
	return strings.HasPrefix(string(t), "pool")
}

// UFI Uniform Resource Identifier
type UFI struct {
	ufiProtocol UFIProtocol
	StorageType StorageType `json:"storageType"`
	UUID        string      `json:"uuid"`
	FullPath    string      `json:"fullPath"`
}

func (ufi *UFI) Validate() bool {
	switch ufi.ufiProtocol {
	case UFIv1:
	default:
		return false
	}
	return true
}

func NewUFI(storageType StorageType, uuid string, fullPath string) *UFI {
	return &UFI{
		ufiProtocol: UFIv1,
		StorageType: storageType,
		UUID:        uuid,
		FullPath:    filepath.Join(Separator, fullPath),
	}
}

func (ufi *UFI) Serialize() string {
	var ufiStr string
	switch {
	case ufi.StorageType == "":
		ufiStr = filepath.Join(Separator, ufi.ufiProtocol.String())
	case ufi.UUID == "":
		ufiStr = filepath.Join(Separator, ufi.ufiProtocol.String(), ufi.StorageType.String())
	default:
		ufiStr = filepath.Join(Separator, ufi.ufiProtocol.String(), ufi.StorageType.String(), ufi.UUID, ufi.FullPath)
	}
	return ufiStr
}

func GenUFISerialize(storageType StorageType, uuid string, fullPath string) string {
	return filepath.Join(Separator, UFIv1.String(), storageType.String(), uuid, fullPath)
}

func GenRealPathByUFISerialize(ugn *UserGroupNamespace, ufi string) string {
	ufiEntity, err := NewUFIFromSerialize(ufi)
	if err != nil {
		return ""
	}
	if !ufiEntity.StorageType.IsSiyouyunStorage() {
		return ""
	}
	return filepath.Join(ugn.GetRealPrefix(ufiEntity.UUID), ufiEntity.FullPath)
}

func NewUFIFromSerialize(UFIString string) (*UFI, error) {
	splitUFISlice := strings.SplitN(strings.TrimSpace(strings.Trim(UFIString, Separator)), Separator, 4)
	ufiEntity := &UFI{
		ufiProtocol: UFIProtocol(splitUFISlice[0]),
	}
	if !ufiEntity.Validate() {
		return nil, errors.New("ufi validate error")
	}
	switch len(splitUFISlice) {
	case 1:
		// example: /ufi
		ufiEntity.StorageType = ""
		ufiEntity.UUID = ""
		ufiEntity.FullPath = ""
	case 2:
		// example: /ufi/pool-raw
		ufiEntity.StorageType = StorageType(splitUFISlice[1])
		ufiEntity.UUID = ""
		ufiEntity.FullPath = ""
	case 3:
		// example: /ufi/pool-raw/syspool
		ufiEntity.StorageType = StorageType(splitUFISlice[1])
		ufiEntity.UUID = splitUFISlice[2]
		ufiEntity.FullPath = Separator
	default:
		// example: /ufi/pool-raw/syspool/Photos
		ufiEntity.StorageType = StorageType(splitUFISlice[1])
		ufiEntity.UUID = splitUFISlice[2]
		ufiEntity.FullPath = filepath.Join(Separator, splitUFISlice[3])
	}
	return ufiEntity, nil
}
