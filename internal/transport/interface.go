package transport

import sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"

// EventTransport abstracts the event transport layer.
// Host mode uses NATSTransport, RUNC mode uses BusTransport.
type EventTransport interface {
	Subscribe(appCode string, opts map[string]sdkdto.PreferOptions) error
	OnEvent(handler func(eventCode string, fe *sdkdto.FileEvent))
	Close() error
}
