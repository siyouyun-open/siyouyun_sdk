// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.33.0
// 	protoc        v5.26.1
// source: ai_service.proto

package sdkprotos

import (
	reflect "reflect"
	sync "sync"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// 消息类型定义
type FaceRecognitionRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"` // 图片路径
}

func (x *FaceRecognitionRequest) Reset() {
	*x = FaceRecognitionRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ai_service_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FaceRecognitionRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FaceRecognitionRequest) ProtoMessage() {}

func (x *FaceRecognitionRequest) ProtoReflect() protoreflect.Message {
	mi := &file_ai_service_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FaceRecognitionRequest.ProtoReflect.Descriptor instead.
func (*FaceRecognitionRequest) Descriptor() ([]byte, []int) {
	return file_ai_service_proto_rawDescGZIP(), []int{0}
}

func (x *FaceRecognitionRequest) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

type FaceRecognitionResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Faces []*FaceRecognitionResponse_Face `protobuf:"bytes,1,rep,name=faces,proto3" json:"faces,omitempty"` // 可能检测到多个人脸
}

func (x *FaceRecognitionResponse) Reset() {
	*x = FaceRecognitionResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ai_service_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FaceRecognitionResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FaceRecognitionResponse) ProtoMessage() {}

func (x *FaceRecognitionResponse) ProtoReflect() protoreflect.Message {
	mi := &file_ai_service_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FaceRecognitionResponse.ProtoReflect.Descriptor instead.
func (*FaceRecognitionResponse) Descriptor() ([]byte, []int) {
	return file_ai_service_proto_rawDescGZIP(), []int{1}
}

func (x *FaceRecognitionResponse) GetFaces() []*FaceRecognitionResponse_Face {
	if x != nil {
		return x.Faces
	}
	return nil
}

type TextClipRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Text string `protobuf:"bytes,1,opt,name=text,proto3" json:"text,omitempty"` // 文本内容
}

func (x *TextClipRequest) Reset() {
	*x = TextClipRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ai_service_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TextClipRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TextClipRequest) ProtoMessage() {}

func (x *TextClipRequest) ProtoReflect() protoreflect.Message {
	mi := &file_ai_service_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TextClipRequest.ProtoReflect.Descriptor instead.
func (*TextClipRequest) Descriptor() ([]byte, []int) {
	return file_ai_service_proto_rawDescGZIP(), []int{2}
}

func (x *TextClipRequest) GetText() string {
	if x != nil {
		return x.Text
	}
	return ""
}

type VectorResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Feature []float32 `protobuf:"fixed32,1,rep,packed,name=feature,proto3" json:"feature,omitempty"` // 特征向量
}

func (x *VectorResponse) Reset() {
	*x = VectorResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ai_service_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *VectorResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*VectorResponse) ProtoMessage() {}

func (x *VectorResponse) ProtoReflect() protoreflect.Message {
	mi := &file_ai_service_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use VectorResponse.ProtoReflect.Descriptor instead.
func (*VectorResponse) Descriptor() ([]byte, []int) {
	return file_ai_service_proto_rawDescGZIP(), []int{3}
}

func (x *VectorResponse) GetFeature() []float32 {
	if x != nil {
		return x.Feature
	}
	return nil
}

type ImageClipRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"` // 图片路径
}

func (x *ImageClipRequest) Reset() {
	*x = ImageClipRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ai_service_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ImageClipRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ImageClipRequest) ProtoMessage() {}

func (x *ImageClipRequest) ProtoReflect() protoreflect.Message {
	mi := &file_ai_service_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ImageClipRequest.ProtoReflect.Descriptor instead.
func (*ImageClipRequest) Descriptor() ([]byte, []int) {
	return file_ai_service_proto_rawDescGZIP(), []int{4}
}

func (x *ImageClipRequest) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

type FaceRecognitionResponse_Face struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Left    int32     `protobuf:"varint,1,opt,name=left,proto3" json:"left,omitempty"`
	Top     int32     `protobuf:"varint,2,opt,name=top,proto3" json:"top,omitempty"`
	Right   int32     `protobuf:"varint,3,opt,name=right,proto3" json:"right,omitempty"`
	Bottom  int32     `protobuf:"varint,4,opt,name=bottom,proto3" json:"bottom,omitempty"`
	Feature []float32 `protobuf:"fixed32,5,rep,packed,name=feature,proto3" json:"feature,omitempty"` // 人脸特征向量
}

func (x *FaceRecognitionResponse_Face) Reset() {
	*x = FaceRecognitionResponse_Face{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ai_service_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FaceRecognitionResponse_Face) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FaceRecognitionResponse_Face) ProtoMessage() {}

func (x *FaceRecognitionResponse_Face) ProtoReflect() protoreflect.Message {
	mi := &file_ai_service_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FaceRecognitionResponse_Face.ProtoReflect.Descriptor instead.
func (*FaceRecognitionResponse_Face) Descriptor() ([]byte, []int) {
	return file_ai_service_proto_rawDescGZIP(), []int{1, 0}
}

func (x *FaceRecognitionResponse_Face) GetLeft() int32 {
	if x != nil {
		return x.Left
	}
	return 0
}

func (x *FaceRecognitionResponse_Face) GetTop() int32 {
	if x != nil {
		return x.Top
	}
	return 0
}

func (x *FaceRecognitionResponse_Face) GetRight() int32 {
	if x != nil {
		return x.Right
	}
	return 0
}

func (x *FaceRecognitionResponse_Face) GetBottom() int32 {
	if x != nil {
		return x.Bottom
	}
	return 0
}

func (x *FaceRecognitionResponse_Face) GetFeature() []float32 {
	if x != nil {
		return x.Feature
	}
	return nil
}

var File_ai_service_proto protoreflect.FileDescriptor

var file_ai_service_proto_rawDesc = []byte{
	0x0a, 0x10, 0x61, 0x69, 0x5f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x12, 0x02, 0x61, 0x69, 0x22, 0x2c, 0x0a, 0x16, 0x46, 0x61, 0x63, 0x65, 0x52, 0x65,
	0x63, 0x6f, 0x67, 0x6e, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x70, 0x61, 0x74, 0x68, 0x22, 0xc7, 0x01, 0x0a, 0x17, 0x46, 0x61, 0x63, 0x65, 0x52, 0x65, 0x63,
	0x6f, 0x67, 0x6e, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x36, 0x0a, 0x05, 0x66, 0x61, 0x63, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x20, 0x2e, 0x61, 0x69, 0x2e, 0x46, 0x61, 0x63, 0x65, 0x52, 0x65, 0x63, 0x6f, 0x67, 0x6e, 0x69,
	0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x46, 0x61, 0x63,
	0x65, 0x52, 0x05, 0x66, 0x61, 0x63, 0x65, 0x73, 0x1a, 0x74, 0x0a, 0x04, 0x46, 0x61, 0x63, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x6c, 0x65, 0x66, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04,
	0x6c, 0x65, 0x66, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x74, 0x6f, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x03, 0x74, 0x6f, 0x70, 0x12, 0x14, 0x0a, 0x05, 0x72, 0x69, 0x67, 0x68, 0x74, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x05, 0x72, 0x69, 0x67, 0x68, 0x74, 0x12, 0x16, 0x0a, 0x06,
	0x62, 0x6f, 0x74, 0x74, 0x6f, 0x6d, 0x18, 0x04, 0x20, 0x01, 0x28, 0x05, 0x52, 0x06, 0x62, 0x6f,
	0x74, 0x74, 0x6f, 0x6d, 0x12, 0x18, 0x0a, 0x07, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x18,
	0x05, 0x20, 0x03, 0x28, 0x02, 0x52, 0x07, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x22, 0x25,
	0x0a, 0x0f, 0x54, 0x65, 0x78, 0x74, 0x43, 0x6c, 0x69, 0x70, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x65, 0x78, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x04, 0x74, 0x65, 0x78, 0x74, 0x22, 0x2a, 0x0a, 0x0e, 0x56, 0x65, 0x63, 0x74, 0x6f, 0x72, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x66, 0x65, 0x61, 0x74, 0x75,
	0x72, 0x65, 0x18, 0x01, 0x20, 0x03, 0x28, 0x02, 0x52, 0x07, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72,
	0x65, 0x22, 0x26, 0x0a, 0x10, 0x49, 0x6d, 0x61, 0x67, 0x65, 0x43, 0x6c, 0x69, 0x70, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74, 0x68, 0x32, 0xc8, 0x01, 0x0a, 0x09, 0x41, 0x49,
	0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x4b, 0x0a, 0x0e, 0x52, 0x65, 0x63, 0x6f, 0x67,
	0x6e, 0x69, 0x7a, 0x65, 0x46, 0x61, 0x63, 0x65, 0x73, 0x12, 0x1a, 0x2e, 0x61, 0x69, 0x2e, 0x46,
	0x61, 0x63, 0x65, 0x52, 0x65, 0x63, 0x6f, 0x67, 0x6e, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1b, 0x2e, 0x61, 0x69, 0x2e, 0x46, 0x61, 0x63, 0x65, 0x52,
	0x65, 0x63, 0x6f, 0x67, 0x6e, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x00, 0x12, 0x35, 0x0a, 0x08, 0x54, 0x65, 0x78, 0x74, 0x43, 0x6c, 0x69, 0x70,
	0x12, 0x13, 0x2e, 0x61, 0x69, 0x2e, 0x54, 0x65, 0x78, 0x74, 0x43, 0x6c, 0x69, 0x70, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x61, 0x69, 0x2e, 0x56, 0x65, 0x63, 0x74, 0x6f,
	0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x37, 0x0a, 0x09, 0x49,
	0x6d, 0x61, 0x67, 0x65, 0x43, 0x6c, 0x69, 0x70, 0x12, 0x14, 0x2e, 0x61, 0x69, 0x2e, 0x49, 0x6d,
	0x61, 0x67, 0x65, 0x43, 0x6c, 0x69, 0x70, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12,
	0x2e, 0x61, 0x69, 0x2e, 0x56, 0x65, 0x63, 0x74, 0x6f, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x00, 0x42, 0x0e, 0x5a, 0x0c, 0x2e, 0x2e, 0x2f, 0x73, 0x64, 0x6b, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x73, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_ai_service_proto_rawDescOnce sync.Once
	file_ai_service_proto_rawDescData = file_ai_service_proto_rawDesc
)

func file_ai_service_proto_rawDescGZIP() []byte {
	file_ai_service_proto_rawDescOnce.Do(func() {
		file_ai_service_proto_rawDescData = protoimpl.X.CompressGZIP(file_ai_service_proto_rawDescData)
	})
	return file_ai_service_proto_rawDescData
}

var file_ai_service_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_ai_service_proto_goTypes = []interface{}{
	(*FaceRecognitionRequest)(nil),       // 0: ai.FaceRecognitionRequest
	(*FaceRecognitionResponse)(nil),      // 1: ai.FaceRecognitionResponse
	(*TextClipRequest)(nil),              // 2: ai.TextClipRequest
	(*VectorResponse)(nil),               // 3: ai.VectorResponse
	(*ImageClipRequest)(nil),             // 4: ai.ImageClipRequest
	(*FaceRecognitionResponse_Face)(nil), // 5: ai.FaceRecognitionResponse.Face
}
var file_ai_service_proto_depIdxs = []int32{
	5, // 0: ai.FaceRecognitionResponse.faces:type_name -> ai.FaceRecognitionResponse.Face
	0, // 1: ai.AIService.RecognizeFaces:input_type -> ai.FaceRecognitionRequest
	2, // 2: ai.AIService.TextClip:input_type -> ai.TextClipRequest
	4, // 3: ai.AIService.ImageClip:input_type -> ai.ImageClipRequest
	1, // 4: ai.AIService.RecognizeFaces:output_type -> ai.FaceRecognitionResponse
	3, // 5: ai.AIService.TextClip:output_type -> ai.VectorResponse
	3, // 6: ai.AIService.ImageClip:output_type -> ai.VectorResponse
	4, // [4:7] is the sub-list for method output_type
	1, // [1:4] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_ai_service_proto_init() }
func file_ai_service_proto_init() {
	if File_ai_service_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_ai_service_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FaceRecognitionRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_ai_service_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FaceRecognitionResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_ai_service_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TextClipRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_ai_service_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*VectorResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_ai_service_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ImageClipRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_ai_service_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FaceRecognitionResponse_Face); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_ai_service_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_ai_service_proto_goTypes,
		DependencyIndexes: file_ai_service_proto_depIdxs,
		MessageInfos:      file_ai_service_proto_msgTypes,
	}.Build()
	File_ai_service_proto = out.File
	file_ai_service_proto_rawDesc = nil
	file_ai_service_proto_goTypes = nil
	file_ai_service_proto_depIdxs = nil
}
