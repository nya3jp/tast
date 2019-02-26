// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package control

import (
	"bytes"
	"encoding/json"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

func TestWriteAndRead(t *gotesting.T) {
	msgs := []interface{}{
		&RunStart{time.Unix(1, 0), 5},
		&RunLog{time.Unix(2, 0), "run message"},
		&TestStart{time.Unix(3, 0), testing.Test{
			Name: "pkg.MyTest",
			Desc: "test description",
			Attr: []string{"attr1", "attr2"},
		}},
		&TestLog{time.Unix(4, 0), "here's a log message"},
		&TestError{time.Unix(5, 0), testing.Error{Reason: "whoops", File: "file.go", Line: 20, Stack: "stack"}},
		&TestEnd{time.Unix(6, 0), "pkg.MyTest", []string{"dep"}, &timing.Log{}, nil},
		&RunEnd{time.Unix(7, 0), "/tmp/out"},
		&RunError{time.Unix(8, 0), testing.Error{Reason: "whoops again", File: "file2.go", Line: 30, Stack: "stack 2"}, 1},
	}

	b := bytes.Buffer{}
	mw := NewMessageWriter(&b)
	for _, msg := range msgs {
		if err := mw.WriteMessage(msg); err != nil {
			t.Errorf("WriteMessage() failed for %v: %v", msg, err)
		}
	}

	act := make([]interface{}, 0)
	mr := NewMessageReader(&b)
	for mr.More() {
		if msg, err := mr.ReadMessage(); err != nil {
			t.Errorf("ReadMessage() failed on %d: %v", len(act), err)
		} else {
			act = append(act, msg)
		}
	}
	if !cmp.Equal(act, msgs, cmpopts.IgnoreUnexported(timing.Log{})) {
		aj, _ := json.Marshal(act)
		ej, _ := json.Marshal(msgs)
		t.Errorf("Read messages %v; want %v", string(aj), string(ej))
	}
}
