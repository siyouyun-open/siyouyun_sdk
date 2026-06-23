package ability

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/nats-io/nats.go"

	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"

	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/transport"
)

type FileEventMonitor struct {
	appCode       *string
	transport     transport.EventTransport
	preferOptions map[string]sdkdto.PreferOptions
}

func NewFileEventMonitor(appCode *string, nc *nats.Conn, preferOps ...sdkdto.PreferOptions) *FileEventMonitor {
	fem := &FileEventMonitor{
		appCode:       appCode,
		preferOptions: make(map[string]sdkdto.PreferOptions),
	}
	for i := range preferOps {
		if preferOps[i].Priority == 0 {
			preferOps[i].Priority = sdkconst.LowLevel
		}
		fem.preferOptions[preferOps[i].ParseToEventCode(*fem.appCode)] = preferOps[i]
	}

	if os.Getenv("APP_RUNTIME") == "RUNC" {
		var err error
		fem.transport, err = transport.NewBus(*appCode)
		if err != nil {
			sdklog.Logger.Errorf("FileEventMonitor BusTransport init failed: %v", err)
			return fem
		}
	} else {
		fem.transport = transport.NewNATSTransport(nc)
	}

	fem.listen()
	return fem
}

func (m *FileEventMonitor) Name() string  { return "FileEventMonitor" }
func (m *FileEventMonitor) IsReady() bool { return true }

func (m *FileEventMonitor) Close() {
	if m.transport != nil {
		_ = m.transport.Close()
	}
}

func (m *FileEventMonitor) FileExistsByEvent(ugn *utils.UserGroupNamespace, ufi string) bool {
	api := utils.GetOSServiceURL() + "/app/event/file/exist"
	resp := apiclient.GetRequest[bool](ugn, api, map[string]string{"ufi": ufi, "appCode": *m.appCode})
	if resp.Code != sdkconst.Success {
		return false
	}
	return *resp.Data
}

func (m *FileEventMonitor) GetUserAppEventConfig(ugn *utils.UserGroupNamespace) *sdkdto.UserAppEventConfig {
	api := utils.GetOSServiceURL() + "/app/event/config"
	resp := apiclient.GetRequest[sdkdto.UserAppEventConfig](ugn, api, map[string]string{"appCode": *m.appCode})
	if resp.Code != sdkconst.Success {
		return nil
	}
	return resp.Data
}

func (m *FileEventMonitor) SetUserAppEventConfig(ugn *utils.UserGroupNamespace, followUFIs []string) error {
	api := utils.GetOSServiceURL() + "/app/event/config"
	params := sdkdto.UserAppEventConfig{AppCode: *m.appCode, FollowDirs: followUFIs}
	resp := apiclient.PostRequest[any](ugn, api, nil, params)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (m *FileEventMonitor) RegisterAppEvent(preferOps ...sdkdto.PreferOptions) error {
	m.preferOptions = make(map[string]sdkdto.PreferOptions)
	for i := range preferOps {
		if preferOps[i].Priority == 0 {
			preferOps[i].Priority = sdkconst.LowLevel
		}
		m.preferOptions[preferOps[i].ParseToEventCode(*m.appCode)] = preferOps[i]
	}
	return registerAppEvent(*m.appCode, preferOps)
}

func (m *FileEventMonitor) TriggerAppEvents(ugn *utils.UserGroupNamespace, ufi string, eventType int, payload any) error {
	raw, err := encodePayload(payload)
	if err != nil {
		return err
	}
	return m.doTriggerAppEvents(ugn, ufi, eventType, raw)
}

func (m *FileEventMonitor) doTriggerAppEvents(ugn *utils.UserGroupNamespace, ufi string, eventType int, payload json.RawMessage) error {
	api := utils.GetOSServiceURL() + "/storage/restore/meta"
	body := map[string]any{"ufi": ufi, "onlyEvent": true, "appCode": *m.appCode, "eventType": eventType}
	if len(payload) > 0 {
		body["payload"] = payload
	}
	resp := apiclient.PostRequest[any](ugn, api, nil, body)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func encodePayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	switch v := payload.(type) {
	case json.RawMessage:
		if len(v) == 0 {
			return nil, nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("TriggerAppEvents: payload is json.RawMessage but not valid JSON")
		}
		return v, nil
	case []byte:
		if len(v) == 0 {
			return nil, nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("TriggerAppEvents: payload is []byte but not valid JSON")
		}
		return json.RawMessage(v), nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("TriggerAppEvents: marshal payload: %w", err)
	}
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	return json.RawMessage(data), nil
}

func (m *FileEventMonitor) listen() {
	if len(m.preferOptions) == 0 || m.transport == nil {
		return
	}

	m.transport.OnEvent(func(eventCode string, fe *sdkdto.FileEvent) {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					sdklog.Logger.Errorf("FileEventMonitor handleEvent panic: %v\nStack trace:\n%s", r, debug.Stack())
				}
			}()
			if options, ok := m.preferOptions[eventCode]; ok {
				options.Handler(fe)
			}
		}()
	})

	if err := m.transport.Subscribe(*m.appCode, m.preferOptions); err != nil {
		sdklog.Logger.Errorf("FileEventMonitor subscribe err: %v", err)
	}
}

func registerAppEvent(appCode string, options []sdkdto.PreferOptions) error {
	api := utils.GetOSServiceURL() + "/app/event/register"
	response := apiclient.PostRequest[any](nil, api, map[string]string{"appCode": appCode}, options)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}
