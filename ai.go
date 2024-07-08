package siyouyunsdk

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"io"
)

type AI struct {
	*gateway.AIHelperApi
}

func (fs *FS) NewAIHelperApi() *AI {
	return &AI{
		AIHelperApi: gateway.NewAIHelperApi(fs.UGN),
	}
}

func (ai *AI) TextClip(text string) ([]float32, error) {
	resp, err := ai.AIInvoke(gateway.TextFeature, &gateway.AIPayload{
		Text: text,
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := resp.([]float32); !ok {
		return nil, errors.New("结果格式错误")
	} else {
		return resp, nil
	}
}

func (ai *AI) ImageClipFromFile(path string) ([]float32, error) {
	resp, err := ai.AIInvoke(gateway.ImageFeature, &gateway.AIPayload{
		ImgPath: path,
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := resp.([]float32); !ok {
		return nil, errors.New("结果格式错误")
	} else {
		return resp, nil
	}
}

func (ai *AI) ImageClipFromReader(r io.Reader) ([]float32, error) {
	resp, err := ai.AIInvoke(gateway.ImageFeature, &gateway.AIPayload{
		ImgReader: r,
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := resp.([]float32); !ok {
		return nil, errors.New("结果格式错误")
	} else {
		return resp, nil
	}
}

func (ai *AI) ClassifyImageFromFile(path string) (*gateway.ClassificationResponse, error) {
	resp, err := ai.AIInvoke(gateway.ClassifyImage, &gateway.AIPayload{
		ImgPath: path,
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := resp.(gateway.ClassificationResponse); !ok {
		return nil, errors.New("结果格式错误")
	} else {
		return &resp, nil
	}
}

func (ai *AI) ClassifyImageFromReader(r io.Reader) (*gateway.ClassificationResponse, error) {
	resp, err := ai.AIInvoke(gateway.ClassifyImage, &gateway.AIPayload{
		ImgReader: r,
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := resp.(gateway.ClassificationResponse); !ok {
		return nil, errors.New("结果格式错误")
	} else {
		return &resp, nil
	}
}

func (ai *AI) RecognizeFacesFromFile(path string) (*gateway.FaceRecognitionResponse, error) {
	resp, err := ai.AIInvoke(gateway.RecognizeFaces, &gateway.AIPayload{
		ImgPath: path,
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := resp.(gateway.FaceRecognitionResponse); !ok {
		return nil, errors.New("结果格式错误")
	} else {
		return &resp, nil
	}
}

func (ai *AI) RecognizeFacesFromReader(r io.Reader) (*gateway.FaceRecognitionResponse, error) {
	resp, err := ai.AIInvoke(gateway.RecognizeFaces, &gateway.AIPayload{
		ImgReader: r,
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := resp.(gateway.FaceRecognitionResponse); !ok {
		return nil, errors.New("结果格式错误")
	} else {
		return &resp, nil
	}
}
