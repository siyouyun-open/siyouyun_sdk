package sdkdto

type KV struct {
	Id        int64  `json:"id"`
	AppCode   string `json:"appCode"`
	Type      string `json:"type"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt int64  `json:"createdAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}
