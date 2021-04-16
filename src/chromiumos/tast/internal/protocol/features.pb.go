// Code generated by protoc-gen-go. DO NOT EDIT.
// source: features.proto

package protocol

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	api "go.chromium.org/chromiumos/config/go/api"
	device "go.chromium.org/chromiumos/infra/proto/go/device"
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

// Features represents a set of features available for tests.
type Features struct {
	// CheckDeps indicates whether to skip tests whose dependencies are not
	// satisfied by available features.
	CheckDeps            bool              `protobuf:"varint,5,opt,name=check_deps,json=checkDeps,proto3" json:"check_deps,omitempty"`
	Software             *SoftwareFeatures `protobuf:"bytes,1,opt,name=software,proto3" json:"software,omitempty"`
	Hardware             *HardwareFeatures `protobuf:"bytes,2,opt,name=hardware,proto3" json:"hardware,omitempty"`
	Vars                 map[string]string `protobuf:"bytes,3,rep,name=vars,proto3" json:"vars,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	MaybeMissingVars     string            `protobuf:"bytes,4,opt,name=maybe_missing_vars,json=maybeMissingVars,proto3" json:"maybe_missing_vars,omitempty"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *Features) Reset()         { *m = Features{} }
func (m *Features) String() string { return proto.CompactTextString(m) }
func (*Features) ProtoMessage()    {}
func (*Features) Descriptor() ([]byte, []int) {
	return fileDescriptor_2216f05915163cdf, []int{0}
}

func (m *Features) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Features.Unmarshal(m, b)
}
func (m *Features) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Features.Marshal(b, m, deterministic)
}
func (m *Features) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Features.Merge(m, src)
}
func (m *Features) XXX_Size() int {
	return xxx_messageInfo_Features.Size(m)
}
func (m *Features) XXX_DiscardUnknown() {
	xxx_messageInfo_Features.DiscardUnknown(m)
}

var xxx_messageInfo_Features proto.InternalMessageInfo

func (m *Features) GetCheckDeps() bool {
	if m != nil {
		return m.CheckDeps
	}
	return false
}

func (m *Features) GetSoftware() *SoftwareFeatures {
	if m != nil {
		return m.Software
	}
	return nil
}

func (m *Features) GetHardware() *HardwareFeatures {
	if m != nil {
		return m.Hardware
	}
	return nil
}

func (m *Features) GetVars() map[string]string {
	if m != nil {
		return m.Vars
	}
	return nil
}

func (m *Features) GetMaybeMissingVars() string {
	if m != nil {
		return m.MaybeMissingVars
	}
	return ""
}

// SoftwareFeatures represents a set of software features available for the
// image being tested.
type SoftwareFeatures struct {
	Available            []string `protobuf:"bytes,1,rep,name=available,proto3" json:"available,omitempty"`
	Unavailable          []string `protobuf:"bytes,2,rep,name=unavailable,proto3" json:"unavailable,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SoftwareFeatures) Reset()         { *m = SoftwareFeatures{} }
func (m *SoftwareFeatures) String() string { return proto.CompactTextString(m) }
func (*SoftwareFeatures) ProtoMessage()    {}
func (*SoftwareFeatures) Descriptor() ([]byte, []int) {
	return fileDescriptor_2216f05915163cdf, []int{1}
}

func (m *SoftwareFeatures) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SoftwareFeatures.Unmarshal(m, b)
}
func (m *SoftwareFeatures) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SoftwareFeatures.Marshal(b, m, deterministic)
}
func (m *SoftwareFeatures) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SoftwareFeatures.Merge(m, src)
}
func (m *SoftwareFeatures) XXX_Size() int {
	return xxx_messageInfo_SoftwareFeatures.Size(m)
}
func (m *SoftwareFeatures) XXX_DiscardUnknown() {
	xxx_messageInfo_SoftwareFeatures.DiscardUnknown(m)
}

var xxx_messageInfo_SoftwareFeatures proto.InternalMessageInfo

func (m *SoftwareFeatures) GetAvailable() []string {
	if m != nil {
		return m.Available
	}
	return nil
}

func (m *SoftwareFeatures) GetUnavailable() []string {
	if m != nil {
		return m.Unavailable
	}
	return nil
}

// HardwareFeatures represents a set of hardware features available for the
// device model being tested.
type HardwareFeatures struct {
	HardwareFeatures       *api.HardwareFeatures `protobuf:"bytes,1,opt,name=hardware_features,json=hardwareFeatures,proto3" json:"hardware_features,omitempty"`
	DeprecatedDeviceConfig *device.Config        `protobuf:"bytes,2,opt,name=deprecated_device_config,json=deprecatedDeviceConfig,proto3" json:"deprecated_device_config,omitempty"`
	XXX_NoUnkeyedLiteral   struct{}              `json:"-"`
	XXX_unrecognized       []byte                `json:"-"`
	XXX_sizecache          int32                 `json:"-"`
}

func (m *HardwareFeatures) Reset()         { *m = HardwareFeatures{} }
func (m *HardwareFeatures) String() string { return proto.CompactTextString(m) }
func (*HardwareFeatures) ProtoMessage()    {}
func (*HardwareFeatures) Descriptor() ([]byte, []int) {
	return fileDescriptor_2216f05915163cdf, []int{2}
}

func (m *HardwareFeatures) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_HardwareFeatures.Unmarshal(m, b)
}
func (m *HardwareFeatures) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_HardwareFeatures.Marshal(b, m, deterministic)
}
func (m *HardwareFeatures) XXX_Merge(src proto.Message) {
	xxx_messageInfo_HardwareFeatures.Merge(m, src)
}
func (m *HardwareFeatures) XXX_Size() int {
	return xxx_messageInfo_HardwareFeatures.Size(m)
}
func (m *HardwareFeatures) XXX_DiscardUnknown() {
	xxx_messageInfo_HardwareFeatures.DiscardUnknown(m)
}

var xxx_messageInfo_HardwareFeatures proto.InternalMessageInfo

func (m *HardwareFeatures) GetHardwareFeatures() *api.HardwareFeatures {
	if m != nil {
		return m.HardwareFeatures
	}
	return nil
}

func (m *HardwareFeatures) GetDeprecatedDeviceConfig() *device.Config {
	if m != nil {
		return m.DeprecatedDeviceConfig
	}
	return nil
}

func init() {
	proto.RegisterType((*Features)(nil), "tast.core.Features")
	proto.RegisterMapType((map[string]string)(nil), "tast.core.Features.VarsEntry")
	proto.RegisterType((*SoftwareFeatures)(nil), "tast.core.SoftwareFeatures")
	proto.RegisterType((*HardwareFeatures)(nil), "tast.core.HardwareFeatures")
}

func init() { proto.RegisterFile("features.proto", fileDescriptor_2216f05915163cdf) }

var fileDescriptor_2216f05915163cdf = []byte{
	// 400 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x52, 0x4f, 0x8b, 0xd4, 0x30,
	0x14, 0xa7, 0x33, 0xbb, 0x32, 0x79, 0x03, 0x4b, 0x8d, 0x22, 0xc3, 0xe8, 0x42, 0x1d, 0x05, 0x7b,
	0x90, 0x14, 0xd7, 0xc3, 0x8a, 0x47, 0x5d, 0x65, 0x2f, 0x5e, 0xa2, 0x78, 0xf0, 0x52, 0x32, 0x69,
	0x66, 0x1a, 0xb6, 0x6d, 0x4a, 0x92, 0x56, 0xfa, 0xb5, 0x04, 0xbf, 0xdf, 0xd2, 0xa4, 0x9d, 0x0e,
	0x33, 0xb7, 0xe4, 0xf7, 0x8f, 0xe4, 0xf7, 0x1e, 0x5c, 0xed, 0x04, 0xb3, 0x8d, 0x16, 0x86, 0xd4,
	0x5a, 0x59, 0x85, 0x91, 0x65, 0xc6, 0x12, 0xae, 0xb4, 0x58, 0xbf, 0xe5, 0xb9, 0x56, 0xa5, 0x6c,
	0x4a, 0x65, 0x12, 0xae, 0xaa, 0x9d, 0xdc, 0x27, 0xac, 0x96, 0x89, 0x55, 0xb5, 0x2a, 0xd4, 0xbe,
	0xf3, 0x86, 0xf5, 0xb3, 0x4c, 0xb4, 0x92, 0x8b, 0x41, 0xe1, 0xc1, 0xcd, 0xff, 0x19, 0x2c, 0xbe,
	0x0f, 0xc1, 0xf8, 0x1a, 0x80, 0xe7, 0x82, 0x3f, 0xa4, 0x99, 0xa8, 0xcd, 0xea, 0x32, 0x0a, 0xe2,
	0x05, 0x45, 0x0e, 0xb9, 0x13, 0xb5, 0xc1, 0xb7, 0xb0, 0x30, 0x6a, 0x67, 0xff, 0x32, 0x2d, 0x56,
	0x41, 0x14, 0xc4, 0xcb, 0x9b, 0x97, 0xe4, 0xf0, 0x08, 0xf2, 0x73, 0xa0, 0xc6, 0x34, 0x7a, 0x10,
	0xf7, 0xc6, 0x9c, 0xe9, 0xcc, 0x19, 0x67, 0x67, 0xc6, 0xfb, 0x81, 0x9a, 0x8c, 0xa3, 0x18, 0x7f,
	0x80, 0x8b, 0x96, 0x69, 0xb3, 0x9a, 0x47, 0xf3, 0x78, 0x79, 0x73, 0x7d, 0x64, 0x1a, 0xc5, 0xe4,
	0x37, 0xd3, 0xe6, 0x5b, 0x65, 0x75, 0x47, 0x9d, 0x14, 0xbf, 0x07, 0x5c, 0xb2, 0x6e, 0x2b, 0xd2,
	0x52, 0x1a, 0x23, 0xab, 0x7d, 0xea, 0x02, 0x2e, 0xa2, 0x20, 0x46, 0x34, 0x74, 0xcc, 0x0f, 0x4f,
	0xf4, 0xc6, 0xf5, 0x2d, 0xa0, 0x43, 0x00, 0x0e, 0x61, 0xfe, 0x20, 0x3a, 0xf7, 0x35, 0x44, 0xfb,
	0x23, 0x7e, 0x0e, 0x97, 0x2d, 0x2b, 0x1a, 0xff, 0x6a, 0x44, 0xfd, 0xe5, 0xf3, 0xec, 0x53, 0xb0,
	0xa1, 0x10, 0x9e, 0x7e, 0x18, 0xbf, 0x02, 0xc4, 0x5a, 0x26, 0x0b, 0xb6, 0x2d, 0xfa, 0x82, 0xe6,
	0x31, 0xa2, 0x13, 0x80, 0x23, 0x58, 0x36, 0xd5, 0xc4, 0xcf, 0x1c, 0x7f, 0x0c, 0x6d, 0xfe, 0x05,
	0x10, 0x9e, 0x96, 0x81, 0x7f, 0xc1, 0xd3, 0xb1, 0x8e, 0x74, 0xdc, 0x80, 0xa1, 0xfd, 0x77, 0x64,
	0x9a, 0x3b, 0x19, 0xa6, 0xca, 0x6a, 0x79, 0x5e, 0x68, 0x98, 0x9f, 0xa6, 0xde, 0xc3, 0x2a, 0x13,
	0xb5, 0x16, 0x9c, 0x59, 0x91, 0xa5, 0x7e, 0x31, 0x52, 0x1f, 0x31, 0x4c, 0xe8, 0x8a, 0x78, 0x94,
	0x7c, 0x75, 0x28, 0x7d, 0x31, 0xe9, 0xef, 0x1c, 0xe1, 0xf1, 0x2f, 0x6f, 0xfe, 0xbc, 0x3e, 0xda,
	0xbe, 0x7e, 0x40, 0x89, 0xac, 0xac, 0xd0, 0x15, 0x2b, 0x12, 0xb7, 0x62, 0x5c, 0x15, 0xdb, 0x27,
	0xee, 0xf4, 0xf1, 0x31, 0x00, 0x00, 0xff, 0xff, 0xb1, 0x38, 0x51, 0x11, 0xc4, 0x02, 0x00, 0x00,
}
