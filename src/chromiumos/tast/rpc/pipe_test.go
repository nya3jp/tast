// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"
)

func TestPipeListener(t *testing.T) {
	const (
		readStr  = "read"
		writeStr = "write"
	)

	r := bytes.NewBufferString(readStr)
	w := &bytes.Buffer{}

	lis := NewPipeListener(r, w)
	defer lis.Close()

	func() {
		conn, err := lis.Accept()
		if err != nil {
			t.Fatal("Accept failed: ", err)
		}
		defer conn.Close()

		if b, err := ioutil.ReadAll(conn); err != nil {
			t.Error("Read failed: ", err)
		} else if s := string(b); s != readStr {
			t.Errorf("Read returned %q; want %q", s, readStr)
		}

		if _, err := conn.Write([]byte(writeStr)); err != nil {
			t.Error("Write failed: ", err)
		}
		if s := w.String(); s != writeStr {
			t.Errorf("Write wrote %q; want %q", s, writeStr)
		}
	}()

	if _, err := lis.Accept(); err != io.EOF {
		t.Errorf("Accept failed: %v; want %v", err, io.EOF)
	}
}
