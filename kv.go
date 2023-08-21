package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
)

type KV struct {
	*gateway.KVCoreApi
}

func (fs *FS) NewKV() *KV {
	return &KV{
		KVCoreApi: gateway.NewKVCoreApi(fs.AppCodeName, fs.UGN),
	}
}

func (kv *KV) PutKV(kvType, key, value string) error {
	return kv.KVCoreApi.PutKV(kvType, key, value)
}

func (kv *KV) DeleteKV(kvType, key string) error {
	return kv.KVCoreApi.DeleteKV(kvType, key)
}

func (kv *KV) GetKV(kvType, key string) (*sdkdto.KV, bool) {
	return kv.KVCoreApi.GetKV(kvType, key)
}
