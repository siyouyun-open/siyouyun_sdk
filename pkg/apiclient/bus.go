package apiclient

import (
	"fmt"

	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

// busClient is the RUNC-mode transport that routes all calls through the bus.
// The actual call function is set by the ability package after BusTransport init.
type busClient struct {
	doFunc func(method, path string, query map[string]string, body any) ([]byte, error)
}

// SetBusFunc sets the bus call function. Called by transport.InitBus.
func SetBusFunc(fn func(method, path string, query map[string]string, body any) ([]byte, error)) {
	if bc, ok := instance.(*busClient); ok {
		bc.doFunc = fn
	}
}

func (c *busClient) Do(method, path string, ugn *utils.UserGroupNamespace, query map[string]string, body any) ([]byte, error) {
	if c.doFunc == nil {
		return nil, fmt.Errorf("bus client not initialized")
	}
	return c.doFunc(method, path, query, body)
}
