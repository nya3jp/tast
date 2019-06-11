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
	if cl, cs, ok := FromContext(context.Background()); ok || cl != nil || cs != nil {
		t.Errorf("FromContext(%v) = (%v, %v, %v); want (%v, %v, %v)", context.Background(), cl, cs, ok, nil, nil, false)
	}

	l := NewLog()
	ctx := NewContext(context.Background(), l)
	if cl, cs, ok := FromContext(ctx); !ok || cl != l || cs != l.Root {
		t.Errorf("FromContext(%v) = (%v, %v, %v); want (%v, %v, %v)", ctx, cl, cs, ok, l, &l.Root, true)
	}
}

func TestStartNil(t *testing.T) {
	// Start should be okay with receiving a context without a Log attached to it,
	// and Stage.End should be okay with a nil receiver.
	_, st := Start(context.Background(), "mystage")
	st.End()
}

func TestStartSeq(t *testing.T) {
	l := NewLog()
	ctx := NewContext(context.Background(), l)
	ctx1, st1 := Start(ctx, "stage1")
	_, st2 := Start(ctx1, "stage2")
	st2.End()
	st1.End()

	if len(l.Root.Children) != 1 {
		t.Errorf("Got %d stages; want 1", len(l.Root.Children))
	} else if l.Root.Children[0].Name != "stage1" {
		t.Errorf("Got stage %q; want %q", l.Root.Children[0].Name, "stage1")
	}

	if len(l.Root.Children[0].Children) != 1 {
		t.Errorf("Got %d stages; want 1", len(l.Root.Children[0].Children))
	} else if l.Root.Children[0].Children[0].Name != "stage2" {
		t.Errorf("Got stage %q; want %q", l.Root.Children[0].Children[0].Name, "stage2")
	}
}

func TestStartPar(t *testing.T) {
	l := NewLog()
	ctx := NewContext(context.Background(), l)
	_, st1 := Start(ctx, "stage1")
	_, st2 := Start(ctx, "stage2")
	st2.End()
	st1.End()

	if len(l.Root.Children) != 2 {
		t.Errorf("Got %d stages; want 2", len(l.Root.Children))
	} else {
		if l.Root.Children[0].Name != "stage1" {
			t.Errorf("Got stage %q; want %q", l.Root.Children[0].Name, "stage1")
		}
		if l.Root.Children[1].Name != "stage2" {
			t.Errorf("Got stage %q; want %q", l.Root.Children[1].Name, "stage2")
		}
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
	// Create a log with a stage and a second nested stage, but only end the first stage.
	clock := fakeClock{}
	lg := &Log{Root: &Stage{now: clock.now}}
	s0 := lg.StartTop("0")
	s0.StartChild("1")
	s0.End()

	// The effect should be the same as if we actually closed the nested stage.
	expClock := fakeClock{}
	expLog := &Log{Root: &Stage{now: expClock.now}}
	s0 = expLog.StartTop("0")
	s0.StartChild("1").End()
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
	l := &Log{Root: &Stage{now: clock.now}}

	s0 := l.StartTop(name0)
	s1 := s0.StartChild(name1)
	s1.StartChild(name2).End()
	s1.End()
	s0.StartChild(name3).End()
	s0.End()
	l.StartTop(name4).End()

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
func addInnerStages(s *Stage) {
	c := s.StartChild("0")
	c.StartChild("1").End()
	c.End()
	s.StartChild("2").End()
}

func TestImport(t *testing.T) {
	// Create an outer log with a single still-open stage.
	clock := fakeClock{}
	outerLog := &Log{Root: &Stage{now: clock.now}}
	st := outerLog.StartTop("out")

	// Create an inner log, import it, and close the outer stage.
	innerLog := &Log{Root: &Stage{now: clock.now}}
	addInnerStages(innerLog.Root)
	if err := st.Import(innerLog); err != nil {
		t.Fatal("Import() reported error: ", err)
	}
	st.End()

	// We expect to see the imported stages within the original stage.
	clock = fakeClock{}
	expLog := &Log{Root: &Stage{now: clock.now}}
	st = expLog.StartTop("out")
	addInnerStages(st)
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
	outerLog := &Log{Root: &Stage{now: clock.now}}
	st := outerLog.StartTop("out")
	st.End()

	// Create an inner log. Importing it should fail since st has ended.
	innerLog := &Log{Root: &Stage{now: clock.now}}
	addInnerStages(innerLog.Root)
	if err := st.Import(innerLog); err == nil {
		t.Error("Import() unexpectedly succeeded without an open stage")
	}
}
