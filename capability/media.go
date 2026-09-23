package capability

import (
	"context"
	"encoding/json"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Media is the media metadata capability (the daemon proxies to OS's ffmpeg service).
type Media struct{ client *bus.Client }

// NewMedia creates a Media wrapper.
func NewMedia(client *bus.Client) *Media { return &Media{client: client} }

// Stream / Format / FFProbeInfo match the structure of the upstream ffprobe result.
type Stream struct {
	Index         int    `json:"index"`
	CodecName     string `json:"codec_name"`
	CodecLongName string `json:"codec_long_name"`
	CodecType     string `json:"codec_type"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	SampleRate    string `json:"sample_rate"`
	Channels      int    `json:"channels"`
	Duration      string `json:"duration"`
	BitRate       string `json:"bit_rate"`
}

type Format struct {
	NbStreams      int    `json:"nb_streams"`
	FormatName     string `json:"format_name"`
	FormatLongName string `json:"format_long_name"`
	StartTime      string `json:"start_time"`
	Duration       string `json:"duration"`
	Size           string `json:"size"`
	BitRate        string `json:"bit_rate"`
	ProbeScore     int    `json:"probe_score"`
}

type FFProbeInfo struct {
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}

// FFmpegInfo reads media file info (via daemon → OS ffmpeg).
//
// scope is the user scope: containers may leave it empty (the daemon derives it from the
// connection), native must provide it.
func (m *Media) FFmpegInfo(ctx context.Context, scope Scope, ufi string) (FFProbeInfo, error) {
	payload := scope.payload()
	payload["ufi"] = ufi
	var info FFProbeInfo
	err := callEnvelope(ctx, m.client, &bus.Request{Template: protocol.TemplateMediaFFmpegInfo, Payload: payload}, &info)
	return info, err
}

// Device is the device info capability.
type Device struct{ client *bus.Client }

// NewDevice creates a Device wrapper.
func NewDevice(client *bus.Client) *Device { return &Device{client: client} }

// ServiceEndpoint / ServiceEndpointGroups / ServiceEndpoints match the structure returned by OS.
type ServiceEndpoint struct {
	Endpoint string `json:"endpoint"`
	Hostname string `json:"hostname,omitempty"`
}

type ServiceEndpointGroups struct {
	Registry   map[string]ServiceEndpoint `json:"registry"`
	Share      ServiceEndpoint            `json:"share"`
	TI         ServiceEndpoint            `json:"ti"`
	Docker     ServiceEndpoint            `json:"docker"`
	AppVersion ServiceEndpoint            `json:"appversion"`
	CloudAuth  ServiceEndpoint            `json:"cloudAuth"`
}

type ServiceEndpoints struct {
	Endpoints ServiceEndpointGroups `json:"endpoints"`
	Refresh   struct {
		IntervalSeconds int `json:"intervalSeconds"`
	} `json:"refresh"`
}

// ServiceEndpoints queries the endpoints of the services on the device. It is unrelated to the user,
// so it needs no scope.
func (d *Device) ServiceEndpoints(ctx context.Context) (ServiceEndpoints, error) {
	var endpoints ServiceEndpoints
	err := callEnvelope(ctx, d.client, &bus.Request{
		Template: protocol.TemplateDeviceServiceEndpoints,
		Payload:  json.RawMessage(`{}`),
	}, &endpoints)
	return endpoints, err
}
