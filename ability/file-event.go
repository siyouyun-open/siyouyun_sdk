package ability

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"

	"github.com/nats-io/nats.go"
	"golang.org/x/exp/maps"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type FileEventMonitor struct {
	appCode       *string
	nc            *nats.Conn
	sub           *nats.Subscription
	preferOptions map[string]sdkdto.PreferOptions
}

func NewFileEventMonitor(appCode *string, nc *nats.Conn, preferOps ...sdkdto.PreferOptions) *FileEventMonitor {
	fem := &FileEventMonitor{
		appCode:       appCode,
		nc:            nc,
		preferOptions: make(map[string]sdkdto.PreferOptions),
	}
	for i := range preferOps {
		if preferOps[i].Priority == 0 {
			preferOps[i].Priority = sdkconst.LowLevel
		}
		fem.preferOptions[preferOps[i].ParseToEventCode(*fem.appCode)] = preferOps[i]
	}
	// start listener
	fem.listen()
	return fem
}

func (m *FileEventMonitor) Name() string {
	return "FileEventMonitor"
}

func (m *FileEventMonitor) IsReady() bool {
	return true
}

func (m *FileEventMonitor) Close() {
	if m.sub != nil {
		_ = m.sub.Unsubscribe()
	}
}

// FileExistsByEvent checks if the file exists by event
func (m *FileEventMonitor) FileExistsByEvent(ugn *utils.UserGroupNamespace, ufi string) bool {
	api := utils.GetOSServiceURL() + "/app/event/file/exist"
	resp := restclient.GetRequest[bool](ugn, api,
		map[string]string{
			"ufi":     ufi,
			"appCode": *m.appCode,
		})
	if resp.Code != sdkconst.Success {
		return false
	}
	return *resp.Data
}

// GetUserAppEventConfig gets user app event config
func (m *FileEventMonitor) GetUserAppEventConfig(ugn *utils.UserGroupNamespace) *sdkdto.UserAppEventConfig {
	api := utils.GetOSServiceURL() + "/app/event/config"
	resp := restclient.GetRequest[sdkdto.UserAppEventConfig](ugn, api,
		map[string]string{
			"appCode": *m.appCode,
		})
	if resp.Code != sdkconst.Success {
		return nil
	}
	return resp.Data
}

// SetUserAppEventConfig sets user app event config
func (m *FileEventMonitor) SetUserAppEventConfig(ugn *utils.UserGroupNamespace, followUFIs []string) error {
	api := utils.GetOSServiceURL() + "/app/event/config"
	params := sdkdto.UserAppEventConfig{
		AppCode:    *m.appCode,
		FollowDirs: followUFIs,
	}
	resp := restclient.PostRequest[any](ugn, api, nil, params)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

// RegisterAppEvent registers app events
func (m *FileEventMonitor) RegisterAppEvent(preferOps ...sdkdto.PreferOptions) error {
	m.preferOptions = make(map[string]sdkdto.PreferOptions)
	for i := range preferOps {
		if preferOps[i].Priority == 0 {
			preferOps[i].Priority = sdkconst.LowLevel
		}
		m.preferOptions[preferOps[i].ParseToEventCode(*m.appCode)] = preferOps[i]
	}
	// re-register
	return registerAppEvent(*m.appCode, preferOps)
}

// TriggerAppEvents fires the given app eventType for every file under the
// directory identified by ufi. The server enumerates the files in that
// scope and emits one FileEvent per file, so callers receive N downstream
// events for a single TriggerAppEvents call (N >= 0, depending on how many
// files match). The same payload, if provided, is delivered verbatim to
// every fired event; consumers can recover the original type with
// FileEvent.BindPayload(&myStruct).
//
// payload may be:
//   - nil                          -> no payload is sent
//   - any struct / map[string]any  -> JSON-marshaled once and forwarded
//   - json.RawMessage              -> inlined as raw JSON, not re-encoded
//   - []byte                       -> treated as raw JSON bytes (validated)
func (m *FileEventMonitor) TriggerAppEvents(
	ugn *utils.UserGroupNamespace,
	ufi string,
	eventType int,
	payload any,
) error {
	raw, err := encodePayload(payload)
	if err != nil {
		return err
	}
	return m.doTriggerAppEvents(ugn, ufi, eventType, raw)
}

func (m *FileEventMonitor) doTriggerAppEvents(
	ugn *utils.UserGroupNamespace,
	ufi string,
	eventType int,
	payload json.RawMessage,
) error {
	api := utils.GetOSServiceURL() + "/storage/restore/meta"
	body := map[string]any{
		"ufi":       ufi,
		"onlyEvent": true,
		"appCode":   *m.appCode,
		"eventType": eventType,
	}
	if len(payload) > 0 {
		body["payload"] = payload
	}
	resp := restclient.PostRequest[any](ugn, api, nil, body)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

// encodePayload normalizes the user-supplied payload into a json.RawMessage
// that will be embedded as-is into the outer request body (and later into
// FileEvent.Payload on the receiving end).
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

// Listen start listening file event
func (m *FileEventMonitor) listen() {
	if len(m.preferOptions) == 0 {
		return
	}
	var err error
	if m.nc == nil {
		return
	}
	err = registerAppEvent(*m.appCode, maps.Values(m.preferOptions))
	if err != nil {
		panic(err)
	}
	m.sub, err = m.nc.Subscribe(*m.appCode+"_event", func(msg *nats.Msg) {
		var fe sdkdto.FileEvent
		err := json.Unmarshal(msg.Data, &fe)
		if err != nil {
			return
		}
		eventCode := msg.Header.Get("eventCode")

		// Execute specific task asynchronously
		go func() {
			defer func() {
				if r := recover(); r != nil {
					sdklog.Logger.Errorf("FileEventMonitor handleEvent panic: %v\nStack trace:\n%s", r, debug.Stack())
					return
				}
			}()
			var cs sdkconst.ConsumeStatus
			var message string
			options, ok := m.preferOptions[eventCode]
			if ok {
				cs, message = options.Handler(&fe)
			} else {
				cs = sdkconst.ConsumeFail
				message = "file event handler not exist"
			}
			var resMsg = nats.NewMsg(msg.Reply)
			resMsg.Data = []byte(message)
			resMsg.Header.Set("status", strconv.Itoa(int(cs)))
			_ = m.nc.PublishMsg(resMsg)
		}()
	})
	if err != nil {
		sdklog.Logger.Errorf("FileEventMonitor subscribe err: %v", err)
	}
}

func registerAppEvent(appCode string, options []sdkdto.PreferOptions) error {
	api := utils.GetOSServiceURL() + "/app/event/register"
	response := restclient.PostRequest[any](
		nil,
		api,
		map[string]string{"appCode": appCode},
		options,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}
