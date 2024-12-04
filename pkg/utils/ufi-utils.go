package utils

import (
	"errors"
	"path/filepath"
	"strings"
)

type UFIProtocol string

const (
	UFIv1 UFIProtocol = "ufi"
)

func (U UFIProtocol) String() string {
	return string(U)
}

type StorageType string

const (
	RawDisk     StorageType = "raw-disk"
	USB         StorageType = "usb"
	Baiduyun    StorageType = "baiduyun"
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
		FullPath:    filepath.Join("/", fullPath),
	}
}

func (ufi *UFI) Serialize() string {
	return filepath.Clean(filepath.Join(
		"/",
		UFIv1.String(),
		ufi.StorageType.String(),
		ufi.UUID,
		ufi.FullPath,
	))
}

func GenUFISerialize(storageType StorageType, uuid string, fullPath string) string {
	return filepath.Clean(filepath.Join("/", UFIv1.String(), storageType.String(), uuid, fullPath))
}

func NewUFIFromSerialize(UFIString string) (*UFI, error) {
	splitUFIString := strings.SplitN(strings.TrimSpace(strings.Trim(UFIString, "/")), "/", 4)
	if len(splitUFIString) < 3 {
		return nil, errors.New("ufi format error")
	}
	var fullPath string
	if len(splitUFIString) == 3 {
		fullPath = "/"
	} else {
		fullPath = splitUFIString[3]
	}
	ufi := &UFI{
		ufiProtocol: UFIProtocol(strings.ReplaceAll(splitUFIString[0], "/", "")),
		StorageType: StorageType(splitUFIString[1]),
		UUID:        splitUFIString[2],
		FullPath:    filepath.Join("/", fullPath),
	}
	if !ufi.Validate() {
		return nil, errors.New("ufi invalid")
	}
	return ufi, nil
}
