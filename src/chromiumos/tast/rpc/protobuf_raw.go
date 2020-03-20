// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"encoding/binary"
	"io"
	"time"

	proto "github.com/golang/protobuf/proto"

	"chromiumos/tast/errors"
)

const (
	// lengthSize is the number of bytes the message length takes before protobuf raw data.
	lengthSize uint32 = 4
	// ioWaitTime is the timeout value waiting for raw protobuf messages on the stream.
	ioWaitTime = 10 * time.Second
)

// SendRawMessage sends raw message to the I/O data stream.
func SendRawMessage(w io.Writer, msg proto.Message) error {
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

// ReceiveRawMessage receives raw message for the I/O data stream.
// It times out if no message is received.
func ReceiveRawMessage(r io.Reader, msg proto.Message) error {
	// Decode data length.
	lengthData, err := receiveNBytes(lengthSize, r)
	if err != nil {
		return err
	}
	length := binary.LittleEndian.Uint32(lengthData)
	// Read message data.
	raw, err := receiveNBytes(length, r)
	if err != nil {
		return err
	}
	return proto.Unmarshal(raw, msg)
}

func receiveNBytes(n uint32, r io.Reader) ([]byte, error) {
	ch := make(chan bool)

	var data []byte
	var readErr error
	if n == 0 {
		return data, nil
	}
	go func() {
		var received uint32
		for {
			expectedSize := n - received
			d := make([]byte, expectedSize)

			m, err := r.Read(d)
			if err != nil {
				// Record the error and exit.
				readErr = err
				break
			}

			data = append(data, d...)

			um := uint32(m)
			if expectedSize == um {
				// All data has been read.
				break
			}
			received += um
		}
		ch <- true
	}()

	select {
	case <-ch:
		return data, readErr
	case <-time.After(ioWaitTime):
		return data, errors.New("timed out waiting for protobuf messages")
	}
}
