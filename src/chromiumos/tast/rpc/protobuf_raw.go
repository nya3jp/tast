// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"encoding/binary"
	"io"

	"github.com/golang/protobuf/proto"
)

const (
	// lengthSize is the number of bytes the message length takes before protobuf raw data.
	lengthSize uint32 = 4
)

// sendRawMessage sends raw protobuf message to the I/O data stream.
func sendRawMessage(w io.Writer, msg proto.Message) error {
	raw, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	// Add data length to the message head.
	lenData := make([]byte, lengthSize)
	binary.LittleEndian.PutUint32(lenData, uint32(len(raw)))
	d := append(lenData, raw...)

	if _, err := w.Write(d); err != nil {
		return err
	}

	return nil
}

// receiveRawMessage receives raw protobuf message from the I/O data stream.
func receiveRawMessage(r io.Reader, msg proto.Message) error {
	// Decode data length.
	lengthData := make([]byte, lengthSize)
	if _, err := io.ReadFull(r, lengthData); err != nil {
		return err
	}
	length := binary.LittleEndian.Uint32(lengthData)
	// Read message data.
	raw := make([]byte, length)
	if _, err := io.ReadFull(r, raw); err != nil {
		return err
	}
	return proto.Unmarshal(raw, msg)
}
