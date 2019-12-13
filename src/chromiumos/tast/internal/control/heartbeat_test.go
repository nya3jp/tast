// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package control

import (
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestHeartbeatWriter(t *testing.T) {
	// Use os.Pipe instead of io.Pipe since os.Pipe has internal buffer which is
	// essential to catch possible WriteMessage races.
	// Note that io.Pipe combined with bufio.Writer has different characteristics;
	// we need to call Flush explicitly to write to the underlying writer, which
	// is inconvenient in this test case.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed: ", err)
	}
	defer r.Close()

	mr := NewMessageReader(r)

	func() {
		defer w.Close()

		mw := NewMessageWriter(w)
		hbw := NewHeartbeatWriter(mw, time.Nanosecond)
		// Don't defer hbw.Close() here; it deadlocks if the buffer is full.
		// Leaking a goroutine is better than being unable to report errors.

		// Read at least 3 heartbeat messages.
		for i := 0; i < 3; i++ {
			msg, err := mr.ReadMessage()
			if err != nil {
				t.Fatal("ReadMessage failed: ", err)
			}
			if _, ok := msg.(*Heartbeat); !ok {
				t.Fatalf("ReadMessage returned %T; want *control.Heartbeat", msg)
			}
		}

		go func() {
			hbw.Stop()
			mw.WriteMessage(&RunEnd{})
		}()

		for {
			msg, err := mr.ReadMessage()
			if err != nil {
				t.Fatal("ReadMessage failed: ", err)
			}
			if _, ok := msg.(*RunEnd); ok {
				break
			} else if _, ok := msg.(*Heartbeat); !ok {
				t.Fatalf("ReadMessage returned %T; want *control.Heartbeat", msg)
			}
		}

		// Sleep for a moment to allow the background goroutine to write a message
		// if it is still alive (which is unexpected).
		time.Sleep(10 * time.Millisecond)
	}()

	// Heartbeat messages must not appear after RunEnd.
	if msg, err := mr.ReadMessage(); err == nil {
		t.Fatalf("Heartbeat sent after Close: %v", msg)
	}
}

func TestHeartbeatWriterZeroInterval(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()

	mw := NewMessageWriter(w)
	// With zero interval, HeartbeatWriter should not write messages.
	hbw := NewHeartbeatWriter(mw, 0)

	go func() {
		// Sleep for a moment to allow the background goroutine to write a message
		// if it is ever the case (which is unexpected).
		time.Sleep(10 * time.Millisecond)
		hbw.Stop()
		w.Close()
	}()

	d, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal("ReadAll failed: ", err)
	}
	if len(d) > 0 {
		t.Errorf("Heartbeat messages written: %q", d)
	}
}

func TestHeartbeatWriterMultipleStop(t *testing.T) {
	mw := NewMessageWriter(ioutil.Discard)
	hbw := NewHeartbeatWriter(mw, time.Second)

	// It is safe to call Stop multiple times.
	hbw.Stop()
	hbw.Stop()
	hbw.Stop()
}
