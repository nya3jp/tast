// Code generated by protoc-gen-go. DO NOT EDIT.
// source: handshake.proto

package protocol

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

// HandshakeRequest contains parameters needed to initialize a gRPC server.
// The message is sent in a raw format since gRPC connection is not ready before
// handshake.
type HandshakeRequest struct {
	// Whether to initialize user-defined gRPC services.
	NeedUserServices     bool              `protobuf:"varint,1,opt,name=need_user_services,json=needUserServices,proto3" json:"need_user_services,omitempty"`
	EntityInitParams     *EntityInitParams `protobuf:"bytes,2,opt,name=entity_init_params,json=entityInitParams,proto3" json:"entity_init_params,omitempty"`
	RunnerInitParams     *RunnerInitParams `protobuf:"bytes,3,opt,name=runner_init_params,json=runnerInitParams,proto3" json:"runner_init_params,omitempty"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *HandshakeRequest) Reset()         { *m = HandshakeRequest{} }
func (m *HandshakeRequest) String() string { return proto.CompactTextString(m) }
func (*HandshakeRequest) ProtoMessage()    {}
func (*HandshakeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_a77305914d5d202f, []int{0}
}

func (m *HandshakeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_HandshakeRequest.Unmarshal(m, b)
}
func (m *HandshakeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_HandshakeRequest.Marshal(b, m, deterministic)
}
func (m *HandshakeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_HandshakeRequest.Merge(m, src)
}
func (m *HandshakeRequest) XXX_Size() int {
	return xxx_messageInfo_HandshakeRequest.Size(m)
}
func (m *HandshakeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_HandshakeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_HandshakeRequest proto.InternalMessageInfo

func (m *HandshakeRequest) GetNeedUserServices() bool {
	if m != nil {
		return m.NeedUserServices
	}
	return false
}

func (m *HandshakeRequest) GetEntityInitParams() *EntityInitParams {
	if m != nil {
		return m.EntityInitParams
	}
	return nil
}

func (m *HandshakeRequest) GetRunnerInitParams() *RunnerInitParams {
	if m != nil {
		return m.RunnerInitParams
	}
	return nil
}

// EntityInitParams contains parameters needed to initialize user-defined
// entities.
type EntityInitParams struct {
	// Runtime variables.
	Vars                 map[string]string `protobuf:"bytes,1,rep,name=vars,proto3" json:"vars,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *EntityInitParams) Reset()         { *m = EntityInitParams{} }
func (m *EntityInitParams) String() string { return proto.CompactTextString(m) }
func (*EntityInitParams) ProtoMessage()    {}
func (*EntityInitParams) Descriptor() ([]byte, []int) {
	return fileDescriptor_a77305914d5d202f, []int{1}
}

func (m *EntityInitParams) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_EntityInitParams.Unmarshal(m, b)
}
func (m *EntityInitParams) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_EntityInitParams.Marshal(b, m, deterministic)
}
func (m *EntityInitParams) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EntityInitParams.Merge(m, src)
}
func (m *EntityInitParams) XXX_Size() int {
	return xxx_messageInfo_EntityInitParams.Size(m)
}
func (m *EntityInitParams) XXX_DiscardUnknown() {
	xxx_messageInfo_EntityInitParams.DiscardUnknown(m)
}

var xxx_messageInfo_EntityInitParams proto.InternalMessageInfo

func (m *EntityInitParams) GetVars() map[string]string {
	if m != nil {
		return m.Vars
	}
	return nil
}

// HandshakeResponse is a response to an HandshakeRequest message.
// The message is sent in a raw format since gRPC connection is not ready before
// handshake.
type HandshakeResponse struct {
	// Set if an error occurred.
	Error                *HandshakeError `protobuf:"bytes,1,opt,name=error,proto3" json:"error,omitempty"`
	XXX_NoUnkeyedLiteral struct{}        `json:"-"`
	XXX_unrecognized     []byte          `json:"-"`
	XXX_sizecache        int32           `json:"-"`
}

func (m *HandshakeResponse) Reset()         { *m = HandshakeResponse{} }
func (m *HandshakeResponse) String() string { return proto.CompactTextString(m) }
func (*HandshakeResponse) ProtoMessage()    {}
func (*HandshakeResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_a77305914d5d202f, []int{2}
}

func (m *HandshakeResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_HandshakeResponse.Unmarshal(m, b)
}
func (m *HandshakeResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_HandshakeResponse.Marshal(b, m, deterministic)
}
func (m *HandshakeResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_HandshakeResponse.Merge(m, src)
}
func (m *HandshakeResponse) XXX_Size() int {
	return xxx_messageInfo_HandshakeResponse.Size(m)
}
func (m *HandshakeResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_HandshakeResponse.DiscardUnknown(m)
}

var xxx_messageInfo_HandshakeResponse proto.InternalMessageInfo

func (m *HandshakeResponse) GetError() *HandshakeError {
	if m != nil {
		return m.Error
	}
	return nil
}

// HandshakeError describes a failed handshake result.
type HandshakeError struct {
	Reason               string   `protobuf:"bytes,1,opt,name=reason,proto3" json:"reason,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *HandshakeError) Reset()         { *m = HandshakeError{} }
func (m *HandshakeError) String() string { return proto.CompactTextString(m) }
func (*HandshakeError) ProtoMessage()    {}
func (*HandshakeError) Descriptor() ([]byte, []int) {
	return fileDescriptor_a77305914d5d202f, []int{3}
}

func (m *HandshakeError) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_HandshakeError.Unmarshal(m, b)
}
func (m *HandshakeError) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_HandshakeError.Marshal(b, m, deterministic)
}
func (m *HandshakeError) XXX_Merge(src proto.Message) {
	xxx_messageInfo_HandshakeError.Merge(m, src)
}
func (m *HandshakeError) XXX_Size() int {
	return xxx_messageInfo_HandshakeError.Size(m)
}
func (m *HandshakeError) XXX_DiscardUnknown() {
	xxx_messageInfo_HandshakeError.DiscardUnknown(m)
}

var xxx_messageInfo_HandshakeError proto.InternalMessageInfo

func (m *HandshakeError) GetReason() string {
	if m != nil {
		return m.Reason
	}
	return ""
}

// RunnerInitParams contains information needed to initialize test runners.
type RunnerInitParams struct {
	// A file path glob that matches test bundle executables.
	// Example: "/usr/local/libexec/tast/bundles/local/*"
	BundleGlob           string   `protobuf:"bytes,1,opt,name=bundle_glob,json=bundleGlob,proto3" json:"bundle_glob,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *RunnerInitParams) Reset()         { *m = RunnerInitParams{} }
func (m *RunnerInitParams) String() string { return proto.CompactTextString(m) }
func (*RunnerInitParams) ProtoMessage()    {}
func (*RunnerInitParams) Descriptor() ([]byte, []int) {
	return fileDescriptor_a77305914d5d202f, []int{4}
}

func (m *RunnerInitParams) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RunnerInitParams.Unmarshal(m, b)
}
func (m *RunnerInitParams) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RunnerInitParams.Marshal(b, m, deterministic)
}
func (m *RunnerInitParams) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RunnerInitParams.Merge(m, src)
}
func (m *RunnerInitParams) XXX_Size() int {
	return xxx_messageInfo_RunnerInitParams.Size(m)
}
func (m *RunnerInitParams) XXX_DiscardUnknown() {
	xxx_messageInfo_RunnerInitParams.DiscardUnknown(m)
}

var xxx_messageInfo_RunnerInitParams proto.InternalMessageInfo

func (m *RunnerInitParams) GetBundleGlob() string {
	if m != nil {
		return m.BundleGlob
	}
	return ""
}

func init() {
	proto.RegisterType((*HandshakeRequest)(nil), "tast.core.HandshakeRequest")
	proto.RegisterType((*EntityInitParams)(nil), "tast.core.EntityInitParams")
	proto.RegisterMapType((map[string]string)(nil), "tast.core.EntityInitParams.VarsEntry")
	proto.RegisterType((*HandshakeResponse)(nil), "tast.core.HandshakeResponse")
	proto.RegisterType((*HandshakeError)(nil), "tast.core.HandshakeError")
	proto.RegisterType((*RunnerInitParams)(nil), "tast.core.RunnerInitParams")
}

func init() { proto.RegisterFile("handshake.proto", fileDescriptor_a77305914d5d202f) }

var fileDescriptor_a77305914d5d202f = []byte{
	// 352 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x52, 0xcf, 0x4b, 0xf3, 0x40,
	0x10, 0x25, 0xed, 0xd7, 0xf2, 0x65, 0x02, 0xdf, 0x17, 0x17, 0x91, 0xaa, 0x07, 0x6b, 0x44, 0xc8,
	0x41, 0x12, 0x68, 0x0f, 0xfe, 0x38, 0x8a, 0x45, 0x7b, 0x93, 0x15, 0x3d, 0x78, 0x09, 0x9b, 0x74,
	0xb0, 0xa1, 0xe9, 0x6e, 0x9d, 0xdd, 0x14, 0xfa, 0x0f, 0xf8, 0x27, 0xfa, 0xf7, 0x48, 0xb6, 0xb1,
	0xb4, 0x11, 0xbc, 0xed, 0xbc, 0x79, 0xf3, 0xe6, 0xcd, 0x63, 0xe1, 0xff, 0x54, 0xc8, 0x89, 0x9e,
	0x8a, 0x19, 0x46, 0x0b, 0x52, 0x46, 0x31, 0xd7, 0x08, 0x6d, 0xa2, 0x4c, 0x11, 0x06, 0x9f, 0x0e,
	0xf8, 0x0f, 0xdf, 0x6d, 0x8e, 0xef, 0x25, 0x6a, 0xc3, 0x2e, 0x80, 0x49, 0xc4, 0x49, 0x52, 0x6a,
	0xa4, 0x44, 0x23, 0x2d, 0xf3, 0x0c, 0x75, 0xcf, 0xe9, 0x3b, 0xe1, 0x5f, 0xee, 0x57, 0x9d, 0x67,
	0x8d, 0xf4, 0x54, 0xe3, 0x6c, 0x0c, 0x0c, 0xa5, 0xc9, 0xcd, 0x2a, 0xc9, 0x65, 0x6e, 0x92, 0x85,
	0x20, 0x31, 0xd7, 0xbd, 0x56, 0xdf, 0x09, 0xbd, 0xc1, 0x71, 0xb4, 0x59, 0x15, 0x8d, 0x2c, 0x69,
	0x2c, 0x73, 0xf3, 0x68, 0x29, 0xdc, 0xc7, 0x06, 0x52, 0x49, 0x51, 0x29, 0x25, 0xd2, 0x8e, 0x54,
	0xfb, 0x87, 0x14, 0xb7, 0xa4, 0x6d, 0x29, 0x6a, 0x20, 0xc1, 0x87, 0x03, 0x7e, 0x73, 0x23, 0xbb,
	0x86, 0x3f, 0x4b, 0x41, 0xd5, 0x29, 0xed, 0xd0, 0x1b, 0x9c, 0xff, 0x62, 0x2e, 0x7a, 0x11, 0xa4,
	0x47, 0xd2, 0xd0, 0x8a, 0xdb, 0x91, 0xa3, 0x4b, 0x70, 0x37, 0x10, 0xf3, 0xa1, 0x3d, 0xc3, 0x95,
	0x4d, 0xc4, 0xe5, 0xd5, 0x93, 0xed, 0x43, 0x67, 0x29, 0x8a, 0x12, 0xed, 0xdd, 0x2e, 0x5f, 0x17,
	0x37, 0xad, 0x2b, 0x27, 0xb8, 0x83, 0xbd, 0xad, 0x80, 0xf5, 0x42, 0x49, 0x8d, 0x2c, 0x86, 0x0e,
	0x12, 0x29, 0xb2, 0x12, 0xde, 0xe0, 0x70, 0xcb, 0xc9, 0x86, 0x3c, 0xaa, 0x08, 0x7c, 0xcd, 0x0b,
	0x42, 0xf8, 0xb7, 0xdb, 0x60, 0x07, 0xd0, 0x25, 0x14, 0x5a, 0xc9, 0xda, 0x46, 0x5d, 0x05, 0x43,
	0xf0, 0x9b, 0xf1, 0xb0, 0x13, 0xf0, 0xd2, 0x52, 0x4e, 0x0a, 0x4c, 0xde, 0x0a, 0x95, 0xd6, 0x03,
	0xb0, 0x86, 0xee, 0x0b, 0x95, 0xde, 0x9e, 0xbd, 0x9e, 0x66, 0x53, 0x52, 0xf3, 0xbc, 0x9c, 0x2b,
	0x1d, 0x57, 0x66, 0xe2, 0x5c, 0x1a, 0x24, 0x29, 0x8a, 0xd8, 0xfe, 0x99, 0x4c, 0x15, 0x69, 0xd7,
	0xbe, 0x86, 0x5f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x32, 0x40, 0xb7, 0x00, 0x50, 0x02, 0x00, 0x00,
}
