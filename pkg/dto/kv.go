package sdkdto

type KV struct {
	Id         int64  `json:"id"`
	AppCode    string `json:"appCode"`
	Type       string `json:"type"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	CreateTime int64  `json:"createTime"`
	UpdateTime int64  `json:"updateTime"`
}
