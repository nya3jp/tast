package timing

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// fakeClock can be used to simulate the package of time in tests.
type fakeClock struct{ sec int64 }

// now returns a time based on c.sec and increments it to simulate a second passing.
func (c *fakeClock) now() time.Time {
	t := time.Unix(c.sec, 0)
	c.sec++
	return t
}

func TestContext(t *testing.T) {
	if cl, ok := FromContext(context.Background()); ok || cl != nil {
		t.Errorf("FromContext(%v) = (%v, %v); want (%v, %v)", context.Background(), ok, cl, false, nil)
	}

	l := &Log{}
	ctx := NewContext(context.Background(), l)
	if cl, ok := FromContext(ctx); !ok || cl != l {
		t.Errorf("FromContext(%v) = (%v, %v); want (%v, %v)", ctx, ok, cl, true, l)
	}
}

// writeLog returns a buffer containing JSON data written by lg.Write.
func writeLog(t *testing.T, lg *Log) *bytes.Buffer {
	b := &bytes.Buffer{}
	if err := lg.Write(b); err != nil {
		t.Fatal("Write() failed: ", err)
	}
	return b
}

func Empty(t *testing.T) {
	l := &Log{}
	if !l.Empty() {
		t.Error("Empty() initially returned true")
	}

	s := l.Start("stage")
	if l.Empty() {
		t.Error("Empty() returned true with open stage")
	}

	s.End()
	if l.Empty() {
		t.Error("Empty() returned true with closed stage")
	}
}

func TestStage_End(t *testing.T) {
	// Create a log with a stage and a second nested stage, but only end the first stage.
	clock := fakeClock{}
	lg := &Log{fakeNow: clock.now}
	s0 := lg.Start("0")
	lg.Start("1")
	s0.End()

	// The effoct should be the same as if we actually closed the nested stage.
	expClock := fakeClock{}
	expLog := &Log{fakeNow: expClock.now}
	s0 = expLog.Start("0")
	expLog.Start("1").End()
	s0.End()

	actBuf := writeLog(t, lg)
	expBuf := writeLog(t, expLog)
	if actBuf.String() != expBuf.String() {
		t.Errorf("Got %q; want %v", actBuf.String(), expBuf.String())
	}
}

func TestWrite(t *testing.T) {
	const (
		name0 = "stage0"
		name1 = "stage1"
		name2 = "stage2"
		name3 = "stage3"
		name4 = "stage4"
	)

	clock := fakeClock{}
	l := &Log{fakeNow: clock.now}

	s0 := l.Start(name0)
	s1 := l.Start(name1)
	l.Start(name2).End()
	s1.End()
	l.Start(name3).End()
	s0.End()
	l.Start(name4).End()

	// Check the expected indenting as well.
	act := writeLog(t, l).String()
	exp := strings.TrimLeft(`
[[7.000, "stage0", [
         [3.000, "stage1", [
                 [1.000, "stage2"]]],
         [1.000, "stage3"]]],
 [1.000, "stage4"]]
`, "\n")
	if act != exp {
		t.Errorf("Write() = %q; want %q", act, exp)
	}
}

// addInnerStages adds two timing stages to lg, with an extra stage embedded in the first one.
func addInnerStages(lg *Log) {
	st := lg.Start("0")
	lg.Start("1").End()
	st.End()
	lg.Start("2").End()
}

func TestImport(t *testing.T) {
	// Create an outer log with a single still-open stage.
	clock := fakeClock{}
	outerLog := &Log{fakeNow: clock.now}
	st := outerLog.Start("out")

	// Create an inner log, import it, and close the outer stage.
	innerLog := &Log{fakeNow: clock.now}
	addInnerStages(innerLog)
	if err := outerLog.Import(innerLog); err != nil {
		t.Fatal("Import() reported error: ", err)
	}
	st.End()

	// We expect to see the imported stages within the original stage.
	clock = fakeClock{}
	expLog := &Log{fakeNow: clock.now}
	st = expLog.Start("out")
	addInnerStages(expLog)
	st.End()

	actBuf := writeLog(t, outerLog)
	expBuf := writeLog(t, expLog)
	if actBuf.String() != expBuf.String() {
		t.Errorf("Got %q; want %v", actBuf.String(), expBuf.String())
	}
}

func TestImportOuterClosed(t *testing.T) {
	// Create an outer log with a single closed stage.
	clock := fakeClock{}
	outerLog := &Log{fakeNow: clock.now}
	outerLog.Start("out").End()

	// Create an inner log. Importing it should fail since the outer log doesn't have an open stage.
	innerLog := &Log{fakeNow: clock.now}
	addInnerStages(innerLog)
	if err := outerLog.Import(innerLog); err == nil {
		t.Error("Import() unexpectedly succeeded without an open stage")
	}
}
