package ability

import (
	"errors"

	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
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
	response := apiclient.GetRequest[sdkdto.ServiceEndpointsConfig](
		nil,
		api,
		nil,
	)
	if response.Code != sdkconst.Success {
		return nil, errors.New(response.Msg)
	}
	return response.Data, nil
}
