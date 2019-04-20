// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sync"
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
		&Heartbeat{Time: time.Unix(9, 0)},
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

func TestConcurrentWrites(t *gotesting.T) {
	// Use os.Pipe instead of io.Pipe to allow concurrent writes with buffering.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed: ", err)
	}
	defer r.Close()

	// This channel is closed when the reader goroutine exits.
	done := make(chan struct{})

	func() {
		defer w.Close()

		mw := NewMessageWriter(w)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		const n = 10

		var wg sync.WaitGroup
		wg.Add(n)

		// Start n writer goroutines to write to mw concurrently.
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						mw.WriteMessage(&Heartbeat{Time: time.Now()})
					}
				}
			}()
		}

		// Start a reader goroutine to read messages and check consistency.
		go func() {
			defer close(done)

			mr := NewMessageReader(r)
			for mr.More() {
				if _, err := mr.ReadMessage(); err != nil {
					t.Error("Corrupted message found: ", err)
					break
				}
			}
		}()

		// Wait for ctx to expire and writer goroutines to finish.
		wg.Wait()
	}()

	// The write end of the pipe has been closed. Wait for the reader goroutine to finish.
	<-done
}
