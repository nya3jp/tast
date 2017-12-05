package timing

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

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

func TestWrite(t *testing.T) {
	const (
		name0 = "stage0"
		name1 = "stage1"
		name2 = "stage2"
		name3 = "stage3"
		name4 = "stage4"
	)

	// Simulate a second passing between each call to get the time.
	var sec int64
	now := func() time.Time {
		t := time.Unix(sec, 0)
		sec++
		return t
	}
	l := &Log{fakeNow: now}

	s0 := l.Start(name0)
	s1 := l.Start(name1)
	l.Start(name2).End()
	s1.End()
	l.Start(name3).End()
	s0.End()
	l.Start(name4).End()

	b := bytes.Buffer{}
	if err := l.Write(&b); err != nil {
		t.Fatal("Write failed: ", err)
	}
	var act interface{}
	if err := json.Unmarshal(b.Bytes(), &act); err != nil {
		t.Fatal(err)
	}

	var exp interface{}
	if err := json.Unmarshal([]byte(
		`[
			[7.000, "stage0", [
				[3.000, "stage1", [
					[1.000, "stage2"]
				]],
				[1.000, "stage3"]]],
			[1.000, "stage4"]
		]`), &exp); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(act, exp) {
		t.Errorf("Write() = %q; want %q", act, exp)
	}
}
