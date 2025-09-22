package ability

import (
	"encoding/json"
	"errors"
	"runtime/debug"
	"strconv"

	"github.com/nats-io/nats.go"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"golang.org/x/exp/maps"
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
