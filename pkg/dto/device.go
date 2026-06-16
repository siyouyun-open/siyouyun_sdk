package sdkdto

// ServiceEndpointsConfig service endpoints configuration
type ServiceEndpointsConfig struct {
	Endpoints ServiceEndpointGroups  `json:"endpoints"`
	Refresh   ServiceEndpointRefresh `json:"refresh"`
}

// ServiceEndpointGroups grouped service endpoints
type ServiceEndpointGroups struct {
	Registry   map[string]ServiceEndpoint `json:"registry"`
	Share      ServiceEndpoint            `json:"share"`
	TI         ServiceEndpoint            `json:"ti"`
	Docker     ServiceEndpoint            `json:"docker"`
	AppVersion ServiceEndpoint            `json:"appversion"`
	CloudAuth  ServiceEndpoint            `json:"cloudAuth"`
}

// ServiceEndpoint single service endpoint entry
type ServiceEndpoint struct {
	Endpoint string `json:"endpoint"`
	Hostname string `json:"hostname,omitempty"`
}

// ServiceEndpointRefresh refresh interval config
type ServiceEndpointRefresh struct {
	IntervalSeconds int `json:"intervalSeconds"`
}
