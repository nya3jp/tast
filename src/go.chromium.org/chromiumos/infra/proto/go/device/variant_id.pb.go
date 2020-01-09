// Code generated by protoc-gen-go. DO NOT EDIT.
// source: device/variant_id.proto

package device

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// Globally unique identifier.
type VariantId struct {
	// Required. Source: 'mosys platform sku', aka Device-SKU.
	Value string `protobuf:"bytes,1,opt,name=value" json:"value,omitempty"`
}

func (m *VariantId) Reset()                    { *m = VariantId{} }
func (m *VariantId) String() string            { return proto.CompactTextString(m) }
func (*VariantId) ProtoMessage()               {}
func (*VariantId) Descriptor() ([]byte, []int) { return fileDescriptor5, []int{0} }

func (m *VariantId) GetValue() string {
	if m != nil {
		return m.Value
	}
	return ""
}

func init() {
	proto.RegisterType((*VariantId)(nil), "device.VariantId")
}

func init() { proto.RegisterFile("device/variant_id.proto", fileDescriptor5) }

var fileDescriptor5 = []byte{
	// 123 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x12, 0x4f, 0x49, 0x2d, 0xcb,
	0x4c, 0x4e, 0xd5, 0x2f, 0x4b, 0x2c, 0xca, 0x4c, 0xcc, 0x2b, 0x89, 0xcf, 0x4c, 0xd1, 0x2b, 0x28,
	0xca, 0x2f, 0xc9, 0x17, 0x62, 0x83, 0x48, 0x28, 0x29, 0x72, 0x71, 0x86, 0x41, 0xe4, 0x3c, 0x53,
	0x84, 0x44, 0xb8, 0x58, 0xcb, 0x12, 0x73, 0x4a, 0x53, 0x25, 0x18, 0x15, 0x18, 0x35, 0x38, 0x83,
	0x20, 0x1c, 0x27, 0xa3, 0x28, 0x83, 0xf4, 0x7c, 0xbd, 0xe4, 0x8c, 0xa2, 0xfc, 0xdc, 0xcc, 0xd2,
	0x5c, 0xbd, 0xfc, 0xa2, 0x74, 0x7d, 0x18, 0x27, 0xbf, 0x58, 0x3f, 0x33, 0x2f, 0xad, 0x28, 0x51,
	0x1f, 0x6c, 0xa8, 0x7e, 0x7a, 0xbe, 0x3e, 0xc4, 0xd8, 0x24, 0x36, 0xb0, 0x80, 0x31, 0x20, 0x00,
	0x00, 0xff, 0xff, 0x21, 0xfa, 0x8d, 0xf0, 0x80, 0x00, 0x00, 0x00,
}
