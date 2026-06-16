package ability

import (
	"errors"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// Device device ability
type Device struct {
	gatewayAddr string
}

// NewDevice create device ability
func NewDevice() *Device {
	return &Device{
		gatewayAddr: utils.GetOSServiceURL() + "/device",
	}
}

func (d *Device) Name() string {
	return "Device"
}

func (d *Device) IsReady() bool {
	return isOSServiceReady()
}

func (d *Device) Close() {
}

// GetServiceEndpoints query service endpoints for mini-program
func (d *Device) GetServiceEndpoints() (*sdkdto.ServiceEndpointsConfig, error) {
	api := d.gatewayAddr + "/service-endpoints"
	response := restclient.GetRequest[sdkdto.ServiceEndpointsConfig](
		nil,
		api,
		nil,
	)
	if response.Code != sdkconst.Success {
		return nil, errors.New(response.Msg)
	}
	return response.Data, nil
}
