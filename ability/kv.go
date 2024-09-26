package ability

import (
	"errors"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

const (
	appCodeQuery = "appCode"
	typeQuery    = "type"
	keyQuery     = "key"
	valueQuery   = "value"
)

type KV struct {
	gatewayAddr string
	appCode     *string
}

func NewKV(appCode *string) *KV {
	return &KV{
		gatewayAddr: utils.GetCoreServiceURL() + "/v2/faas/kv",
		appCode:     appCode,
	}
}

func (kv *KV) Name() string {
	return "KV"
}

func (kv *KV) Close() {
}

func (kv *KV) PutKV(ugn *utils.UserGroupNamespace, kvType, key, value string) error {
	api := kv.gatewayAddr + "/put"
	response := restclient.PostRequest[any](
		ugn,
		api,
		map[string]string{
			appCodeQuery: *kv.appCode,
			typeQuery:    kvType,
			keyQuery:     key,
			valueQuery:   value,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

func (kv *KV) DeleteKV(ugn *utils.UserGroupNamespace, kvType, key string) error {
	api := kv.gatewayAddr + "/delete"
	response := restclient.PostRequest[any](
		ugn,
		api,
		map[string]string{
			appCodeQuery: *kv.appCode,
			typeQuery:    kvType,
			keyQuery:     key,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

func (kv *KV) GetKV(ugn *utils.UserGroupNamespace, kvType, key string) (*sdkdto.KV, bool) {
	api := kv.gatewayAddr + "/value"
	response := restclient.GetRequest[sdkdto.KV](
		ugn,
		api,
		map[string]string{
			appCodeQuery: *kv.appCode,
			typeQuery:    kvType,
			keyQuery:     key,
		},
	)
	if response.Code != sdkconst.Success {
		return nil, false
	}
	return response.Data, true
}
