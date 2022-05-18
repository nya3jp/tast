// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.19.3
// source: loopback.proto

package protocol

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ExecRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Type:
	//	*ExecRequest_Init
	//	*ExecRequest_Stdin
	Type isExecRequest_Type `protobuf_oneof:"type"`
}

func (x *ExecRequest) Reset() {
	*x = ExecRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_loopback_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ExecRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExecRequest) ProtoMessage() {}

func (x *ExecRequest) ProtoReflect() protoreflect.Message {
	mi := &file_loopback_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExecRequest.ProtoReflect.Descriptor instead.
func (*ExecRequest) Descriptor() ([]byte, []int) {
	return file_loopback_proto_rawDescGZIP(), []int{0}
}

func (m *ExecRequest) GetType() isExecRequest_Type {
	if m != nil {
		return m.Type
	}
	return nil
}

func (x *ExecRequest) GetInit() *InitEvent {
	if x, ok := x.GetType().(*ExecRequest_Init); ok {
		return x.Init
	}
	return nil
}

func (x *ExecRequest) GetStdin() *PipeEvent {
	if x, ok := x.GetType().(*ExecRequest_Stdin); ok {
		return x.Stdin
	}
	return nil
}

type isExecRequest_Type interface {
	isExecRequest_Type()
}

type ExecRequest_Init struct {
	Init *InitEvent `protobuf:"bytes,1,opt,name=init,proto3,oneof"`
}

type ExecRequest_Stdin struct {
	Stdin *PipeEvent `protobuf:"bytes,2,opt,name=stdin,proto3,oneof"`
}

func (*ExecRequest_Init) isExecRequest_Type() {}

func (*ExecRequest_Stdin) isExecRequest_Type() {}

type ExecResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Type:
	//	*ExecResponse_Exit
	//	*ExecResponse_Stdout
	//	*ExecResponse_Stderr
	Type isExecResponse_Type `protobuf_oneof:"type"`
}

func (x *ExecResponse) Reset() {
	*x = ExecResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_loopback_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ExecResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExecResponse) ProtoMessage() {}

func (x *ExecResponse) ProtoReflect() protoreflect.Message {
	mi := &file_loopback_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExecResponse.ProtoReflect.Descriptor instead.
func (*ExecResponse) Descriptor() ([]byte, []int) {
	return file_loopback_proto_rawDescGZIP(), []int{1}
}

func (m *ExecResponse) GetType() isExecResponse_Type {
	if m != nil {
		return m.Type
	}
	return nil
}

func (x *ExecResponse) GetExit() *ExitEvent {
	if x, ok := x.GetType().(*ExecResponse_Exit); ok {
		return x.Exit
	}
	return nil
}

func (x *ExecResponse) GetStdout() *PipeEvent {
	if x, ok := x.GetType().(*ExecResponse_Stdout); ok {
		return x.Stdout
	}
	return nil
}

func (x *ExecResponse) GetStderr() *PipeEvent {
	if x, ok := x.GetType().(*ExecResponse_Stderr); ok {
		return x.Stderr
	}
	return nil
}

type isExecResponse_Type interface {
	isExecResponse_Type()
}

type ExecResponse_Exit struct {
	Exit *ExitEvent `protobuf:"bytes,1,opt,name=exit,proto3,oneof"`
}

type ExecResponse_Stdout struct {
	Stdout *PipeEvent `protobuf:"bytes,2,opt,name=stdout,proto3,oneof"`
}

type ExecResponse_Stderr struct {
	Stderr *PipeEvent `protobuf:"bytes,3,opt,name=stderr,proto3,oneof"`
}

func (*ExecResponse_Exit) isExecResponse_Type() {}

func (*ExecResponse_Stdout) isExecResponse_Type() {}

func (*ExecResponse_Stderr) isExecResponse_Type() {}

type InitEvent struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Args []string `protobuf:"bytes,1,rep,name=args,proto3" json:"args,omitempty"`
}

func (x *InitEvent) Reset() {
	*x = InitEvent{}
	if protoimpl.UnsafeEnabled {
		mi := &file_loopback_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InitEvent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InitEvent) ProtoMessage() {}

func (x *InitEvent) ProtoReflect() protoreflect.Message {
	mi := &file_loopback_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InitEvent.ProtoReflect.Descriptor instead.
func (*InitEvent) Descriptor() ([]byte, []int) {
	return file_loopback_proto_rawDescGZIP(), []int{2}
}

func (x *InitEvent) GetArgs() []string {
	if x != nil {
		return x.Args
	}
	return nil
}

type ExitEvent struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Code int32 `protobuf:"varint,1,opt,name=code,proto3" json:"code,omitempty"`
}

func (x *ExitEvent) Reset() {
	*x = ExitEvent{}
	if protoimpl.UnsafeEnabled {
		mi := &file_loopback_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ExitEvent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExitEvent) ProtoMessage() {}

func (x *ExitEvent) ProtoReflect() protoreflect.Message {
	mi := &file_loopback_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExitEvent.ProtoReflect.Descriptor instead.
func (*ExitEvent) Descriptor() ([]byte, []int) {
	return file_loopback_proto_rawDescGZIP(), []int{3}
}

func (x *ExitEvent) GetCode() int32 {
	if x != nil {
		return x.Code
	}
	return 0
}

type PipeEvent struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Data  []byte `protobuf:"bytes,1,opt,name=data,proto3" json:"data,omitempty"`
	Close bool   `protobuf:"varint,2,opt,name=close,proto3" json:"close,omitempty"`
}

func (x *PipeEvent) Reset() {
	*x = PipeEvent{}
	if protoimpl.UnsafeEnabled {
		mi := &file_loopback_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PipeEvent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PipeEvent) ProtoMessage() {}

func (x *PipeEvent) ProtoReflect() protoreflect.Message {
	mi := &file_loopback_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PipeEvent.ProtoReflect.Descriptor instead.
func (*PipeEvent) Descriptor() ([]byte, []int) {
	return file_loopback_proto_rawDescGZIP(), []int{4}
}

func (x *PipeEvent) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

func (x *PipeEvent) GetClose() bool {
	if x != nil {
		return x.Close
	}
	return false
}

var File_loopback_proto protoreflect.FileDescriptor

var file_loopback_proto_rawDesc = []byte{
	0x0a, 0x0e, 0x6c, 0x6f, 0x6f, 0x70, 0x62, 0x61, 0x63, 0x6b, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x09, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x22, 0x6f, 0x0a, 0x0b, 0x45,
	0x78, 0x65, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x2a, 0x0a, 0x04, 0x69, 0x6e,
	0x69, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x74, 0x61, 0x73, 0x74, 0x2e,
	0x63, 0x6f, 0x72, 0x65, 0x2e, 0x49, 0x6e, 0x69, 0x74, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x48, 0x00,
	0x52, 0x04, 0x69, 0x6e, 0x69, 0x74, 0x12, 0x2c, 0x0a, 0x05, 0x73, 0x74, 0x64, 0x69, 0x6e, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72,
	0x65, 0x2e, 0x50, 0x69, 0x70, 0x65, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x48, 0x00, 0x52, 0x05, 0x73,
	0x74, 0x64, 0x69, 0x6e, 0x42, 0x06, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x22, 0xa2, 0x01, 0x0a,
	0x0c, 0x45, 0x78, 0x65, 0x63, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x2a, 0x0a,
	0x04, 0x65, 0x78, 0x69, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x74, 0x61,
	0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x45, 0x78, 0x69, 0x74, 0x45, 0x76, 0x65, 0x6e,
	0x74, 0x48, 0x00, 0x52, 0x04, 0x65, 0x78, 0x69, 0x74, 0x12, 0x2e, 0x0a, 0x06, 0x73, 0x74, 0x64,
	0x6f, 0x75, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x74, 0x61, 0x73, 0x74,
	0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x50, 0x69, 0x70, 0x65, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x48,
	0x00, 0x52, 0x06, 0x73, 0x74, 0x64, 0x6f, 0x75, 0x74, 0x12, 0x2e, 0x0a, 0x06, 0x73, 0x74, 0x64,
	0x65, 0x72, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x74, 0x61, 0x73, 0x74,
	0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x50, 0x69, 0x70, 0x65, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x48,
	0x00, 0x52, 0x06, 0x73, 0x74, 0x64, 0x65, 0x72, 0x72, 0x42, 0x06, 0x0a, 0x04, 0x74, 0x79, 0x70,
	0x65, 0x22, 0x1f, 0x0a, 0x09, 0x49, 0x6e, 0x69, 0x74, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x12, 0x12,
	0x0a, 0x04, 0x61, 0x72, 0x67, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x04, 0x61, 0x72,
	0x67, 0x73, 0x22, 0x1f, 0x0a, 0x09, 0x45, 0x78, 0x69, 0x74, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x12,
	0x12, 0x0a, 0x04, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04, 0x63,
	0x6f, 0x64, 0x65, 0x22, 0x35, 0x0a, 0x09, 0x50, 0x69, 0x70, 0x65, 0x45, 0x76, 0x65, 0x6e, 0x74,
	0x12, 0x12, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04,
	0x64, 0x61, 0x74, 0x61, 0x12, 0x14, 0x0a, 0x05, 0x63, 0x6c, 0x6f, 0x73, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x08, 0x52, 0x05, 0x63, 0x6c, 0x6f, 0x73, 0x65, 0x32, 0x54, 0x0a, 0x13, 0x4c, 0x6f,
	0x6f, 0x70, 0x62, 0x61, 0x63, 0x6b, 0x45, 0x78, 0x65, 0x63, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63,
	0x65, 0x12, 0x3d, 0x0a, 0x04, 0x45, 0x78, 0x65, 0x63, 0x12, 0x16, 0x2e, 0x74, 0x61, 0x73, 0x74,
	0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x45, 0x78, 0x65, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x17, 0x2e, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x45, 0x78,
	0x65, 0x63, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x28, 0x01, 0x30, 0x01,
	0x42, 0x23, 0x5a, 0x21, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x6f, 0x73, 0x2f, 0x74,
	0x61, 0x73, 0x74, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_loopback_proto_rawDescOnce sync.Once
	file_loopback_proto_rawDescData = file_loopback_proto_rawDesc
)

func file_loopback_proto_rawDescGZIP() []byte {
	file_loopback_proto_rawDescOnce.Do(func() {
		file_loopback_proto_rawDescData = protoimpl.X.CompressGZIP(file_loopback_proto_rawDescData)
	})
	return file_loopback_proto_rawDescData
}

var file_loopback_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_loopback_proto_goTypes = []interface{}{
	(*ExecRequest)(nil),  // 0: tast.core.ExecRequest
	(*ExecResponse)(nil), // 1: tast.core.ExecResponse
	(*InitEvent)(nil),    // 2: tast.core.InitEvent
	(*ExitEvent)(nil),    // 3: tast.core.ExitEvent
	(*PipeEvent)(nil),    // 4: tast.core.PipeEvent
}
var file_loopback_proto_depIdxs = []int32{
	2, // 0: tast.core.ExecRequest.init:type_name -> tast.core.InitEvent
	4, // 1: tast.core.ExecRequest.stdin:type_name -> tast.core.PipeEvent
	3, // 2: tast.core.ExecResponse.exit:type_name -> tast.core.ExitEvent
	4, // 3: tast.core.ExecResponse.stdout:type_name -> tast.core.PipeEvent
	4, // 4: tast.core.ExecResponse.stderr:type_name -> tast.core.PipeEvent
	0, // 5: tast.core.LoopbackExecService.Exec:input_type -> tast.core.ExecRequest
	1, // 6: tast.core.LoopbackExecService.Exec:output_type -> tast.core.ExecResponse
	6, // [6:7] is the sub-list for method output_type
	5, // [5:6] is the sub-list for method input_type
	5, // [5:5] is the sub-list for extension type_name
	5, // [5:5] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_loopback_proto_init() }
func file_loopback_proto_init() {
	if File_loopback_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_loopback_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ExecRequest); i {
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
		file_loopback_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ExecResponse); i {
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
		file_loopback_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*InitEvent); i {
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
		file_loopback_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ExitEvent); i {
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
		file_loopback_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PipeEvent); i {
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
	file_loopback_proto_msgTypes[0].OneofWrappers = []interface{}{
		(*ExecRequest_Init)(nil),
		(*ExecRequest_Stdin)(nil),
	}
	file_loopback_proto_msgTypes[1].OneofWrappers = []interface{}{
		(*ExecResponse_Exit)(nil),
		(*ExecResponse_Stdout)(nil),
		(*ExecResponse_Stderr)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_loopback_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_loopback_proto_goTypes,
		DependencyIndexes: file_loopback_proto_depIdxs,
		MessageInfos:      file_loopback_proto_msgTypes,
	}.Build()
	File_loopback_proto = out.File
	file_loopback_proto_rawDesc = nil
	file_loopback_proto_goTypes = nil
	file_loopback_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// LoopbackExecServiceClient is the client API for LoopbackExecService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type LoopbackExecServiceClient interface {
	Exec(ctx context.Context, opts ...grpc.CallOption) (LoopbackExecService_ExecClient, error)
}

type loopbackExecServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewLoopbackExecServiceClient(cc grpc.ClientConnInterface) LoopbackExecServiceClient {
	return &loopbackExecServiceClient{cc}
}

func (c *loopbackExecServiceClient) Exec(ctx context.Context, opts ...grpc.CallOption) (LoopbackExecService_ExecClient, error) {
	stream, err := c.cc.NewStream(ctx, &_LoopbackExecService_serviceDesc.Streams[0], "/tast.core.LoopbackExecService/Exec", opts...)
	if err != nil {
		return nil, err
	}
	x := &loopbackExecServiceExecClient{stream}
	return x, nil
}

type LoopbackExecService_ExecClient interface {
	Send(*ExecRequest) error
	Recv() (*ExecResponse, error)
	grpc.ClientStream
}

type loopbackExecServiceExecClient struct {
	grpc.ClientStream
}

func (x *loopbackExecServiceExecClient) Send(m *ExecRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *loopbackExecServiceExecClient) Recv() (*ExecResponse, error) {
	m := new(ExecResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// LoopbackExecServiceServer is the server API for LoopbackExecService service.
type LoopbackExecServiceServer interface {
	Exec(LoopbackExecService_ExecServer) error
}

// UnimplementedLoopbackExecServiceServer can be embedded to have forward compatible implementations.
type UnimplementedLoopbackExecServiceServer struct {
}

func (*UnimplementedLoopbackExecServiceServer) Exec(LoopbackExecService_ExecServer) error {
	return status.Errorf(codes.Unimplemented, "method Exec not implemented")
}

func RegisterLoopbackExecServiceServer(s *grpc.Server, srv LoopbackExecServiceServer) {
	s.RegisterService(&_LoopbackExecService_serviceDesc, srv)
}

func _LoopbackExecService_Exec_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(LoopbackExecServiceServer).Exec(&loopbackExecServiceExecServer{stream})
}

type LoopbackExecService_ExecServer interface {
	Send(*ExecResponse) error
	Recv() (*ExecRequest, error)
	grpc.ServerStream
}

type loopbackExecServiceExecServer struct {
	grpc.ServerStream
}

func (x *loopbackExecServiceExecServer) Send(m *ExecResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *loopbackExecServiceExecServer) Recv() (*ExecRequest, error) {
	m := new(ExecRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var _LoopbackExecService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "tast.core.LoopbackExecService",
	HandlerType: (*LoopbackExecServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Exec",
			Handler:       _LoopbackExecService_Exec_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "loopback.proto",
}
