package transport

import (
	"encoding/json"
	"errors"
	"runtime/debug"
	"strconv"

	"github.com/nats-io/nats.go"

	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

// NATSTransport is the host-mode file event transport using NATS.
type NATSTransport struct {
	nc      *nats.Conn
	sub     *nats.Subscription
	handler func(eventCode string, fe *sdkdto.FileEvent)
}

func NewNATSTransport(nc *nats.Conn) *NATSTransport {
	return &NATSTransport{nc: nc}
}

func (t *NATSTransport) Subscribe(appCode string, opts map[string]sdkdto.PreferOptions) error {
	if t.nc == nil {
		return errors.New("NATS connection is nil")
	}
	if len(opts) == 0 {
		return nil
	}

	preferList := make([]sdkdto.PreferOptions, 0, len(opts))
	for _, v := range opts {
		preferList = append(preferList, v)
	}
	if err := t.registerAppEvent(appCode, preferList); err != nil {
		return err
	}

	var err error
	t.sub, err = t.nc.Subscribe(appCode+"_event", func(msg *nats.Msg) {
		var fe sdkdto.FileEvent
		if err := json.Unmarshal(msg.Data, &fe); err != nil {
			return
		}
		eventCode := msg.Header.Get("eventCode")

		go func() {
			defer func() {
				if r := recover(); r != nil {
					sdklog.Logger.Errorf("FileEventMonitor handleEvent panic: %v\nStack trace:\n%s", r, debug.Stack())
				}
			}()

			if t.handler != nil {
				t.handler(eventCode, &fe)
			}

			cs := sdkconst.ConsumeSuccess
			resMsg := nats.NewMsg(msg.Reply)
			resMsg.Data = []byte("")
			resMsg.Header.Set("status", strconv.Itoa(int(cs)))
			_ = t.nc.PublishMsg(resMsg)
		}()
	})
	return err
}

func (t *NATSTransport) OnEvent(handler func(eventCode string, fe *sdkdto.FileEvent)) {
	t.handler = handler
}

func (t *NATSTransport) Close() error {
	if t.sub != nil {
		return t.sub.Unsubscribe()
	}
	return nil
}

func (t *NATSTransport) registerAppEvent(appCode string, options []sdkdto.PreferOptions) error {
	api := utils.GetOSServiceURL() + "/app/event/register"
	response := apiclient.PostRequest[any](
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
