package gateway

import (
	"errors"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"io"
)

type AIMethod string

const (
	TextFeature    AIMethod = "TextFeature"
	ImageFeature            = "ImageFeatureFromFile"
	ClassifyImage           = "ClassifyImageFromFile"
	RecognizeFaces          = "RecognizeFacesFromFile"
)

type AIPayload struct {
	// text
	Text string `json:"text"`
	// img
	ImgPath   string    `json:"imgPath"`
	ImgReader io.Reader `json:"imgReader"`
}

type AIHelperApi struct {
	Host string
	*utils.UserGroupNamespace
}

func NewAIHelperApi(ugn *utils.UserGroupNamespace) *AIHelperApi {
	return &AIHelperApi{
		Host:               CoreServiceURL + "/ai",
		UserGroupNamespace: ugn,
	}
}

// ClassificationResponse 图片分类
type ClassificationResponse struct {
	ClassId   int32   `json:"classId,omitempty"`   // 类别ID
	ClassName string  `json:"className,omitempty"` // 类别名称
	Prop      float32 `json:"prop,omitempty"`      // 可能性
}

// FaceRecognitionResponseFace 人脸识别
type FaceRecognitionResponseFace struct {
	Left    int32     `json:"left,omitempty"`
	Top     int32     `json:"top,omitempty"`
	Right   int32     `json:"right,omitempty"`
	Bottom  int32     `json:"bottom,omitempty"`
	Feature []float32 `json:"feature,omitempty"` // 人脸特征向量
}

type FaceRecognitionResponse struct {
	Faces []*FaceRecognitionResponseFace `json:"faces,omitempty"` // 可能检测到多个人脸
}

func (kv *AIHelperApi) AIInvoke(method AIMethod, payload *AIPayload) (any, error) {
	api := kv.Host + "/invoke"
	response := restclient.PostRequest[any](
		kv.UserGroupNamespace,
		api,
		map[string]string{
			"method": string(method),
			"text":   payload.Text,
			"path":   payload.ImgPath,
		},
		payload.ImgReader,
	)
	if response.Code != sdkconst.Success {
		return nil, errors.New(response.Msg)
	}
	return response.Data, nil
}
