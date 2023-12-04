package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

const (
	KVApiPut    = "/put"
	KVApiDelete = "/delete"
	KVApiGet    = "/value"
)

const (
	AppCodeQuery = "appCode"
	TypeQuery    = "type"
	KeyQuery     = "key"
	ValueQuery   = "value"
)

type KVCoreApi struct {
	Host    string
	AppCode string
	UGN     *utils.UserGroupNamespace
}

var kvCoreGatewayAddr = CoreServiceURL + "/kv"

func NewKVCoreApi(appCode string, un *utils.UserGroupNamespace) *KVCoreApi {
	return &KVCoreApi{
		Host:    kvCoreGatewayAddr,
		AppCode: appCode,
		UGN:     un,
	}
}

// PutKV PutKV
func (kv *KVCoreApi) PutKV(kvType, key, value string) error {
	api := kv.Host + KVApiPut
	response := restclient.PostRequest[any](
		kv.UGN,
		api,
		map[string]string{
			AppCodeQuery: kv.AppCode,
			TypeQuery:    kvType,
			KeyQuery:     key,
			ValueQuery:   value,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// DeleteKV DeleteKV
func (kv *KVCoreApi) DeleteKV(kvType, key string) error {
	api := kv.Host + KVApiDelete
	response := restclient.PostRequest[any](
		kv.UGN,
		api,
		map[string]string{
			AppCodeQuery: kv.AppCode,
			TypeQuery:    kvType,
			KeyQuery:     key,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// GetKV GetKV
func (kv *KVCoreApi) GetKV(kvType, key string) (*sdkdto.KV, bool) {
	api := kv.Host + KVApiGet
	response := restclient.GetRequest[sdkdto.KV](
		kv.UGN,
		api,
		map[string]string{
			AppCodeQuery: kv.AppCode,
			TypeQuery:    kvType,
			KeyQuery:     key,
		},
	)
	if response.Code != sdkconst.Success {
		return nil, false
	}
	return response.Data, true
}
