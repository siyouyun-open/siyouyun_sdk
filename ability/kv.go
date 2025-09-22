package ability

import (
	"errors"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
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
		gatewayAddr: utils.GetCoreServiceURL() + "/v2/app/kv",
		appCode:     appCode,
	}
}

func (kv *KV) Name() string {
	return "KV"
}

func (kv *KV) IsReady() bool {
	return utils.IsCoreServiceReady()
}

func (kv *KV) Close() {
}

// PutKV put ugn kv
func (kv *KV) PutKV(ugn *utils.UserGroupNamespace, kvType, key, value string) error {
	api := kv.gatewayAddr + "/put"
	data := sdkdto.KV{
		AppCode: *kv.appCode,
		Type:    kvType,
		Key:     key,
		Value:   value,
	}
	response := restclient.PostRequest[any](
		ugn,
		api,
		nil,
		data,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// DeleteKV delete ugn kv
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

// GetKV get ugn kv
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
	if response.Code != sdkconst.Success || response.Data == nil {
		return nil, false
	}
	return response.Data, true
}

// GetKVList get ugn kv list
func (kv *KV) GetKVList(ugn *utils.UserGroupNamespace, kvType string) []sdkdto.KV {
	api := kv.gatewayAddr + "/values"
	response := restclient.GetRequest[[]sdkdto.KV](
		ugn,
		api,
		map[string]string{
			appCodeQuery: *kv.appCode,
			typeQuery:    kvType,
		},
	)
	if response.Code != sdkconst.Success || response.Data == nil {
		return nil
	}
	return *response.Data
}

// PutSysKV pu sys kv
func (kv *KV) PutSysKV(kvType, key, value string) error {
	api := kv.gatewayAddr + "/sys/put"
	data := sdkdto.KV{
		AppCode: *kv.appCode,
		Type:    kvType,
		Key:     key,
		Value:   value,
	}
	response := restclient.PostRequest[any](
		nil,
		api,
		nil,
		data,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

// DeleteSysKV delete sys kv
func (kv *KV) DeleteSysKV(kvType, key string) error {
	api := kv.gatewayAddr + "/sys/delete"
	response := restclient.PostRequest[any](
		nil,
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

// GetSysKV get sys kv
func (kv *KV) GetSysKV(kvType, key string) (*sdkdto.KV, bool) {
	api := kv.gatewayAddr + "/sys/value"
	response := restclient.GetRequest[sdkdto.KV](
		nil,
		api,
		map[string]string{
			appCodeQuery: *kv.appCode,
			typeQuery:    kvType,
			keyQuery:     key,
		},
	)
	if response.Code != sdkconst.Success || response.Data == nil {
		return nil, false
	}
	return response.Data, true
}

// GetSysKVList get sys kv list
func (kv *KV) GetSysKVList(kvType string) []sdkdto.KV {
	api := kv.gatewayAddr + "/sys/values"
	response := restclient.GetRequest[[]sdkdto.KV](
		nil,
		api,
		map[string]string{
			appCodeQuery: *kv.appCode,
			typeQuery:    kvType,
		},
	)
	if response.Code != sdkconst.Success || response.Data == nil {
		return nil
	}
	return *response.Data
}
