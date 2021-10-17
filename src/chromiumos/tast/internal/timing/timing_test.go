// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package timing

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/protocol"
)

// fakeClock can be used to simulate the package of time in tests.
type fakeClock struct{ sec int64 }

// install installs the fake clock as the function used to get the current time
// in this package.
func (c *fakeClock) install() {
	now = c.now
}

// uninstall uninstalls the fake clock.
func (c *fakeClock) uninstall() {
	now = time.Now
}

// reset resets the fake timer to the initial state.
func (c *fakeClock) reset() {
	c.sec = 0
}

// now returns a time based on c.sec and increments it to simulate a second passing.
func (c *fakeClock) now() time.Time {
	t := time.Unix(c.sec, 0)
	c.sec++
	return t
}

// writePretty returns a buffer containing JSON data written by lg.WritePretty.
func writePretty(t *testing.T, lg *Log) *bytes.Buffer {
	b := &bytes.Buffer{}
	if err := lg.WritePretty(b); err != nil {
		t.Fatal("WritePretty() failed: ", err)
	}
	return b
}

func TestEmpty(t *testing.T) {
	l := NewLog()
	if !l.Empty() {
		t.Error("Empty() initially returned true")
	}

	s := l.StartTop("stage")
	if l.Empty() {
		t.Error("Empty() returned true with open stage")
	}

	s.End()
	if l.Empty() {
		t.Error("Empty() returned true with closed stage")
	}
}

func TestStage_End(t *testing.T) {
	var fc fakeClock
	fc.install()
	defer fc.uninstall()

	// Create a log with a stage and a second nested stage, but only end the first stage.
	lg := NewLog()
	s0 := lg.StartTop("0")
	s0.StartChild("1")
	s0.End()

	// The effect should be the same as if we actually closed the nested stage.
	fc.reset()
	expLog := NewLog()
	s0 = expLog.StartTop("0")
	s0.StartChild("1").End()
	s0.End()

	actBuf := writePretty(t, lg)
	expBuf := writePretty(t, expLog)
	if actBuf.String() != expBuf.String() {
		t.Errorf("Got %v; want %v", actBuf.String(), expBuf.String())
	}
}

func TestWritePretty(t *testing.T) {
	const (
		name0 = "stage0"
		name1 = "stage1"
		name2 = "stage2"
		name3 = "stage3"
		name4 = "stage4"
	)

	var fc fakeClock
	fc.install()
	defer fc.uninstall()

	l := NewLog()
	s0 := l.StartTop(name0)
	s1 := s0.StartChild(name1)
	s1.StartChild(name2).End()
	s1.End()
	s0.StartChild(name3).End()
	s0.End()
	l.StartTop(name4).End()

	// Check the expected indenting as well.
	act := writePretty(t, l).String()
	exp := strings.TrimLeft(`
[[7.000, "stage0", [
         [3.000, "stage1", [
                 [1.000, "stage2"]]],
         [1.000, "stage3"]]],
 [1.000, "stage4"]]
`, "\n")
	if act != exp {
		t.Errorf("WritePretty() = %q; want %q", act, exp)
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	var fc fakeClock
	fc.install()
	defer fc.uninstall()

	// Create a log.
	log := NewLog()
	st := log.StartTop("0")
	st.StartChild("1").End()
	st.StartChild("2").End()
	st.End()

	// Marshal and unmarshal the log.
	b, err := json.Marshal(log)
	if err != nil {
		t.Fatal("Marshal failed: ", err)
	}
	var newLog Log
	if err := json.Unmarshal(b, &newLog); err != nil {
		t.Fatal("Unmarshal failed: ", err)
	}

	// log and newLog must be idential.
	if diff := cmp.Diff(&newLog, log, cmpopts.IgnoreUnexported(Stage{})); diff != "" {
		t.Fatal("Log changed after marshal and unmarshal (-got +want)\n", diff)
	}
}

// addInnerStages adds two timing stages to lg, with an extra stage embedded in the first one.
func addInnerStages(s *Stage) {
	c := s.StartChild("0")
	c.StartChild("1").End()
	c.End()
	s.StartChild("2").End()
}

func TestImport(t *testing.T) {
	var fc fakeClock
	fc.install()
	defer fc.uninstall()

	// Create an outer log with a single still-open stage.
	outerLog := NewLog()
	st := outerLog.StartTop("out")

	// Create an inner log, import it, and close the outer stage.
	innerLog := NewLog()
	addInnerStages(innerLog.Root)
	if err := st.Import(innerLog); err != nil {
		t.Fatal("Import() reported error: ", err)
	}
	st.End()

	// We expect to see the imported stages within the original stage.
	fc.reset()
	expLog := NewLog()
	st = expLog.StartTop("out")
	addInnerStages(st)
	st.End()

	actBuf := writePretty(t, outerLog)
	expBuf := writePretty(t, expLog)
	if actBuf.String() != expBuf.String() {
		t.Errorf("Got %v; want %v", actBuf.String(), expBuf.String())
	}
}

func TestImportOuterClosed(t *testing.T) {
	var fc fakeClock
	fc.install()
	defer fc.uninstall()

	// Create an outer log with a single closed stage.
	outerLog := NewLog()
	st := outerLog.StartTop("out")
	st.End()

	// Create an inner log. Importing it should fail since st has ended.
	innerLog := NewLog()
	addInnerStages(innerLog.Root)
	if err := st.Import(innerLog); err == nil {
		t.Error("Import() unexpectedly succeeded without an open stage")
	}
}

func TestImportMarshaledLog(t *testing.T) {
	var fc fakeClock
	fc.install()
	defer fc.uninstall()

	// Create an inner log with a single still-open stage.
	innerLog := NewLog()
	innerLog.StartTop("in")

	// Marshal and unmarshal the inner log.
	b, err := json.Marshal(innerLog)
	if err != nil {
		t.Fatal("Marshal failed: ", err)
	}
	var newLog Log
	if err := json.Unmarshal(b, &newLog); err != nil {
		t.Fatal("Unmarshal failed: ", err)
	}

	// Create an outer log and import the unmarshaled log.
	outerLog := NewLog()
	st := outerLog.StartTop("out")
	if err := st.Import(&newLog); err != nil {
		t.Fatal("Import() reported error: ", err)
	}

	// Finish the stage in the outer log. This will close the incomplete
	// stage in the inner log. This used to cause panic (crbug.com/981708).
	st.End()
}

func TestProto(t *testing.T) {
	var fc fakeClock
	fc.install()
	defer fc.uninstall()

	log := NewLog()
	st := log.StartTop("0")
	st.StartChild("1").End()
	st.StartChild("2").End()
	st.End()

	got, err := log.Proto()
	if err != nil {
		t.Fatalf("Proto failed: %v", err)
	}

	want := &protocol.TimingLog{
		Root: &protocol.TimingStage{
			Children: []*protocol.TimingStage{{
				Name:      "0",
				StartTime: &timestamp.Timestamp{Seconds: 0},
				EndTime:   &timestamp.Timestamp{Seconds: 5},
				Children: []*protocol.TimingStage{
					{
						Name:      "1",
						StartTime: &timestamp.Timestamp{Seconds: 1},
						EndTime:   &timestamp.Timestamp{Seconds: 2},
					},
					{
						Name:      "2",
						StartTime: &timestamp.Timestamp{Seconds: 3},
						EndTime:   &timestamp.Timestamp{Seconds: 4},
					},
				},
			}},
		},
	}
	if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
		t.Errorf("Proto mismatch (-got +want):\n%s", diff)
	}
}

func TestLogFromProto(t *testing.T) {
	log := &protocol.TimingLog{
		Root: &protocol.TimingStage{
			Children: []*protocol.TimingStage{{
				Name:      "0",
				StartTime: &timestamp.Timestamp{Seconds: 0},
				EndTime:   &timestamp.Timestamp{Seconds: 5},
				Children: []*protocol.TimingStage{
					{
						Name:      "1",
						StartTime: &timestamp.Timestamp{Seconds: 1},
						EndTime:   &timestamp.Timestamp{Seconds: 2},
					},
					{
						Name:      "2",
						StartTime: &timestamp.Timestamp{Seconds: 3},
						EndTime:   &timestamp.Timestamp{Seconds: 4},
					},
				},
			}},
		},
	}
	got, err := LogFromProto(log)
	if err != nil {
		t.Fatalf("LogFromProto failed: %v", err)
	}

	want := &Log{
		Root: &Stage{
			Children: []*Stage{{
				Name:      "0",
				StartTime: time.Unix(0, 0),
				EndTime:   time.Unix(5, 0),
				Children: []*Stage{
					{
						Name:      "1",
						StartTime: time.Unix(1, 0),
						EndTime:   time.Unix(2, 0),
					},
					{
						Name:      "2",
						StartTime: time.Unix(3, 0),
						EndTime:   time.Unix(4, 0),
					},
				},
			}},
		},
	}
	if diff := cmp.Diff(got, want, cmpopts.IgnoreUnexported(Stage{})); diff != "" {
		t.Fatal("Log changed after marshal and unmarshal (-got +want)\n", diff)
	}
}
