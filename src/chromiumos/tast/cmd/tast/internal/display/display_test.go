// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeTerm is an in-memory implementation of the terminal interface used for testing.
type fakeTerm struct {
	numRows, numCols       int
	curRow, curCol         int // 0-indexed
	savedRow, savedCol     int // 0-indexed
	scrollStart, scrollEnd int // 0-indexed rows

	data []byte // currently-displayed data, with NULs for empty cells

	openHandles int // number of unclosed fakeTermHandle objects

	reverse       bool
	cursorVisible bool
	lineWrap      bool
}

func newFakeTerm(rows, cols int, contents string) *fakeTerm {
	t := &fakeTerm{
		numRows:       rows,
		numCols:       cols,
		scrollStart:   0,
		scrollEnd:     rows - 1,
		data:          make([]byte, rows*cols),
		cursorVisible: true,
		lineWrap:      true,
	}
	for i, s := range strings.Split(strings.TrimRight(contents, "\n"), "\n") {
		copy(t.getRow(i), []byte(s))
	}
	return t
}

func (t *fakeTerm) check() error { return nil }

func (t *fakeTerm) newHandle() termHandle {
	t.openHandles++
	return &fakeTermHandle{tm: t}
}

// getRow returns a slice into t.data representing the slice at 0-indexed row.
func (t *fakeTerm) getRow(row int) []byte {
	return t.data[row*t.numCols : (row+1)*t.numCols]
}

// clearRow clears 0-indexed row in t.data.
func (t *fakeTerm) clearRow(row int) {
	copy(t.getRow(row), []byte(strings.Repeat("\x00", t.numCols)))
}

// String returns a string representation of the terminal's contents
// with each row on a separate line.
func (t *fakeTerm) String() string {
	var s string
	for i := 0; i < t.numRows; i++ {
		s += string(bytes.TrimRight(t.getRow(i), "\x00")) + "\n"
	}
	return s
}

type fakeTermHandle struct {
	tm  *fakeTerm
	err error
}

func (th *fakeTermHandle) close() error {
	th.tm.openHandles--
	return nil
}

func (th *fakeTermHandle) writeString(s string) termHandle {
	// TODO(derat): Improve the fake implementation if more features are needed.
	if th.tm.lineWrap {
		th.err = errors.New("line wrap unsupported")
		return th
	}
	if strings.ContainsAny(s, "\n\t") {
		th.err = errors.New("tabs and newlines unsupported")
		return th
	}

	for len(s) > 0 {
		r := th.tm.getRow(th.tm.curRow)

		// Pad the beginning of the row with spaces if it doesn't already
		// extend to the column where we'll start writing.
		if th.tm.curCol > 0 && r[th.tm.curCol-1] == 0 {
			copy(r[0:th.tm.curCol], bytes.Repeat([]byte{' '}, th.tm.curCol))
		}

		pre := th.tm.curCol - len(r)
		if pre > 0 {
			copy(r[len(r):th.tm.curCol], strings.Repeat(" ", pre))
		}

		numCopied := copy(r[th.tm.curCol:th.tm.numCols], []byte(s))
		s = s[numCopied:]

		th.tm.curCol += numCopied
		if th.tm.curCol >= th.tm.numCols {
			th.tm.curCol = th.tm.numCols - 1
		}
	}
	return th
}

func (th *fakeTermHandle) reverse() termHandle {
	th.tm.reverse = true
	return th
}

func (th *fakeTermHandle) clearAttr() termHandle {
	th.tm.reverse = false
	return th
}

func (th *fakeTermHandle) cursorVisible(vis bool) termHandle {
	th.tm.cursorVisible = vis
	return th
}

func (th *fakeTermHandle) lineWrap(wrap bool) termHandle {
	th.tm.lineWrap = wrap
	return th
}

func (th *fakeTermHandle) clearLine() termHandle {
	th.tm.clearRow(th.tm.curRow)
	return th
}

func (th *fakeTermHandle) clearToBottom() termHandle {
	for i := th.tm.curRow; i < th.tm.numRows; i++ {
		th.tm.clearRow(i)
	}
	return th
}

func (th *fakeTermHandle) scroll(lines int) termHandle {
	var start, end, inc int
	if lines > 0 {
		start = th.tm.scrollStart
		end = th.tm.scrollEnd + 1
		inc = 1
	} else {
		start = th.tm.scrollEnd
		end = th.tm.scrollStart - 1
		inc = -1
	}

	for dst := start; dst != end; dst += inc {
		src := dst + lines
		if src < th.tm.scrollStart || src > th.tm.scrollEnd {
			th.tm.clearRow(dst)
		} else {
			copy(th.tm.getRow(dst), th.tm.getRow(src))
		}
	}
	return th
}

func (th *fakeTermHandle) setScrollRegion(start, end int) termHandle {
	if start < 0 || end < 0 {
		th.tm.scrollStart, th.tm.scrollEnd = 0, th.tm.numRows-1
	} else {
		th.tm.scrollStart, th.tm.scrollEnd = start-1, end-1
	}
	return th
}

func (th *fakeTermHandle) setCursorPos(row, col int) termHandle {
	th.tm.curRow, th.tm.curCol = row-1, col-1
	return th
}

func (th *fakeTermHandle) saveCursorPos() termHandle {
	th.tm.savedRow, th.tm.savedCol = th.tm.curRow, th.tm.curRow
	return th
}

func (th *fakeTermHandle) restoreCursorPos() termHandle {
	th.tm.curRow, th.tm.curRow = th.tm.savedRow, th.tm.savedCol
	return th
}

func (th *fakeTermHandle) getCursorPos() (row, col int, err error) {
	return th.tm.curRow + 1, th.tm.curCol + 1, nil
}

func (th *fakeTermHandle) getSize() (rows, cols int, err error) {
	return th.tm.numRows, th.tm.numCols, nil
}

// testWriteString writes s to tm at (row, col) and compares tm's full data to exp.
// Errors are reported via t.
func testWriteString(t *testing.T, tm *fakeTerm, row, col int, s, exp string) {
	if row > 0 && col > 0 {
		if err := tm.newHandle().setCursorPos(row, col).close(); err != nil {
			t.Errorf("setCursorPos(%d, %d) failed: %v", row, col, err)
		}
	}
	if err := tm.newHandle().writeString(s).close(); err != nil {
		t.Errorf("writeString(%q) failed: %v", s, err)
	}
	if act := tm.String(); act != exp {
		t.Errorf("writeString(%q) produced %q; want %q", s, act, exp)
	}
}

// testScroll scrolls tm by lines and compares its full data to exp.
// Errors are reported via t.
func testScroll(t *testing.T, tm *fakeTerm, lines int, exp string) {
	if err := tm.newHandle().scroll(lines).close(); err != nil {
		t.Errorf("scroll(%d) failed: %v", lines, err)
	}
	if act := tm.String(); act != exp {
		t.Errorf("scroll(%d) produced %q; want %q", lines, act, exp)
	}
}

func TestFakeTermWriteString(t *testing.T) {
	tm := newFakeTerm(3, 5, "")
	tm.lineWrap = false

	testWriteString(t, tm, 0, 0, "012", "012\n\n\n")
	testWriteString(t, tm, 0, 0, "345", "01235\n\n\n")
	testWriteString(t, tm, 2, 3, "abcde", "01235\n  abe\n\n")
	testWriteString(t, tm, 1, 4, "xyz", "012xz\n  abe\n\n")
	testWriteString(t, tm, 3, 1, "12345", "012xz\n  abe\n12345\n")
}

func TestFakeTermScroll(t *testing.T) {
	tm := newFakeTerm(3, 2, "12\nab\ncd\n")
	tm.lineWrap = false

	testScroll(t, tm, 0, "12\nab\ncd\n")
	testScroll(t, tm, 2, "cd\n\n\n")
	testScroll(t, tm, -1, "\ncd\n\n")
	testScroll(t, tm, -1, "\n\ncd\n")
	testScroll(t, tm, -1, "\n\n\n")
	testScroll(t, tm, 1, "\n\n\n")
}

func TestFakeTermScrollRegion(t *testing.T) {
	tm := newFakeTerm(3, 2, "12\nab\ncd\n")
	tm.lineWrap = false

	if err := tm.newHandle().setScrollRegion(1, 2).close(); err != nil {
		t.Error("setScrollRegion(1, 2) failed: ", err)
	}
	testScroll(t, tm, 1, "ab\n\ncd\n")
	testScroll(t, tm, -1, "\nab\ncd\n")
	testScroll(t, tm, -1, "\n\ncd\n")

	if err := tm.newHandle().setScrollRegion(-1, -1).close(); err != nil {
		t.Error("setScrollRegion(-1, -1) failed: ", err)
	}
	testScroll(t, tm, 2, "cd\n\n\n")
}

func initDisplayTest(t *testing.T, rows, cols, maxVerboseRows int) (*fakeTerm, *Display) {
	tm := newFakeTerm(rows, cols, "")
	dpy, err := New(tm, maxVerboseRows)
	if err != nil {
		t.Fatal("New() failed: ", err)
	}
	return tm, dpy
}

func TestSetStatus(t *testing.T) {
	tm, dpy := initDisplayTest(t, 3, 12, 2)
	defer dpy.Close()

	dpy.SetStatus("status 1")
	if act, exp := tm.String(), "status 1    \n\n\n"; act != exp {
		t.Errorf("Initial SetStatus() produced %q; want %q", act, exp)
	}
	dpy.SetStatus("2nd status")
	if act, exp := tm.String(), "2nd status  \n\n\n"; act != exp {
		t.Errorf("Second SetStatus() produced %q; want %q", act, exp)
	}
	dpy.SetStatus("this is a long status line")
	if act, exp := tm.String(), "this is a lo\n\n\n"; act != exp {
		t.Errorf("Long SetStatus() produced %q; want %q", act, exp)
	}
	dpy.SetStatus("")
	if act, exp := tm.String(), "            \n\n\n"; act != exp {
		t.Errorf("Empty SetStatus() produced %q; want %q", act, exp)
	}
}

func TestAddPersistent(t *testing.T) {
	tm, dpy := initDisplayTest(t, 3, 12, 2)
	defer dpy.Close()

	dpy.SetStatus("status")
	dpy.AddPersistent("persist 1")
	if act, exp := tm.String(), "persist 1\nstatus      \n\n"; act != exp {
		t.Errorf("First AddPersistent() produced %q; want %q", act, exp)
	}
	dpy.AddPersistent("persist 2")
	if act, exp := tm.String(), "persist 1\npersist 2\nstatus      \n"; act != exp {
		t.Errorf("Second AddPersistent() produced %q; want %q", act, exp)
	}
	// Adding a third persistent line should push the first persistent line offscreen and
	// leave the status on the bottom row.
	dpy.AddPersistent("this is a long persistent line")
	if act, exp := tm.String(), "persist 2\nthis is a lo\nstatus      \n"; act != exp {
		t.Errorf("Third AddPersistent() produced %q; want %q", act, exp)
	}
	// SetStatus should now update the bottom line.
	dpy.SetStatus("new status")
	if act, exp := tm.String(), "persist 2\nthis is a lo\nnew status  \n"; act != exp {
		t.Errorf("Second SetStatus() produced %q; want %q", act, exp)
	}
}

func TestAddVerbose(t *testing.T) {
	tm, dpy := initDisplayTest(t, 5, 12, 2)
	defer dpy.Close()

	dpy.SetStatus("status")
	dpy.AddVerbose("verbose 1")
	dpy.AddVerbose("verbose 2")
	if act, exp := tm.String(), "status      \nverbose 1\nverbose 2\n\n\n"; act != exp {
		t.Errorf("First two AddVerbose()s produced %q; want %q", act, exp)
	}
	// Adding a third verbose line should scroll the existing lines up.
	dpy.AddVerbose("this is a long verbose line")
	if act, exp := tm.String(), "status      \nverbose 2\nthis is a lo\n\n\n"; act != exp {
		t.Errorf("Third AddVerbose() produced %q; want %q", act, exp)
	}
	// SetStatus should still update the top line.
	dpy.SetStatus("new status")
	if act, exp := tm.String(), "new status  \nverbose 2\nthis is a lo\n\n\n"; act != exp {
		t.Errorf("Second SetStatus() produced %q; want %q", act, exp)
	}
}

func TestMultilineMessages(t *testing.T) {
	tm, dpy := initDisplayTest(t, 5, 12, 2)
	defer dpy.Close()

	dpy.SetStatus("status 1\nstatus 2\n\n")
	if act, exp := tm.String(), "status 2    \n\n\n\n\n"; act != exp {
		t.Errorf("SetStatus() with newline produced %q; want %q", act, exp)
	}

	dpy.AddPersistent("persist 1\npersist 2\n\n")
	if act, exp := tm.String(), "persist 1\npersist 2\nstatus 2    \n\n\n"; act != exp {
		t.Errorf("AddPersistent() with newlines produced %q; want %q", act, exp)
	}

	dpy.AddVerbose("verbose 1\nverbose 2\nverbose 3\nverbose 4\nverbose 5\n\n\n")
	if act, exp := tm.String(), "persist 1\npersist 2\nstatus 2    \nverbose 4\nverbose 5\n"; act != exp {
		t.Errorf("AddVerbose() with newlines produced %q; want %q", act, exp)
	}
}

func TestStartNearBottom(t *testing.T) {
	tm := newFakeTerm(5, 10, "")
	tm.newHandle().setCursorPos(3, 1).close()
	dpy, err := New(tm, 2)
	if err != nil {
		t.Fatal("New() failed: ", err)
	}
	defer dpy.Close()

	dpy.SetStatus("status")
	dpy.AddPersistent("persist 1")
	if act, exp := tm.String(), "\n\npersist 1\nstatus    \n\n"; act != exp {
		t.Errorf("First AddPersistent() produced %q; want %q", act, exp)
	}
	dpy.AddPersistent("persist 2")
	if act, exp := tm.String(), "\n\npersist 1\npersist 2\nstatus    \n"; act != exp {
		t.Errorf("Second AddPersistent() produced %q; want %q", act, exp)
	}
	dpy.AddPersistent("persist 3")
	if act, exp := tm.String(), "\npersist 1\npersist 2\npersist 3\nstatus    \n"; act != exp {
		t.Errorf("Third AddPersistent() produced %q; want %q", act, exp)
	}
	// When a verbose line is added while the status is on the bottom row, everything should be pushed up.
	dpy.AddVerbose("verbose 1")
	if act, exp := tm.String(), "persist 1\npersist 2\npersist 3\nstatus    \nverbose 1\n"; act != exp {
		t.Errorf("First AddVerbose() produced %q; want %q", act, exp)
	}
	// A second verbose line should push everything up another line.
	dpy.AddVerbose("verbose 2")
	if act, exp := tm.String(), "persist 2\npersist 3\nstatus    \nverbose 1\nverbose 2\n"; act != exp {
		t.Errorf("Second AddVerbose() produced %q; want %q", act, exp)
	}
	// Only two verbose lines are supposed to displayed, so a third verbose message should scroll the verbose area.
	dpy.AddVerbose("verbose 3")
	if act, exp := tm.String(), "persist 2\npersist 3\nstatus    \nverbose 2\nverbose 3\n"; act != exp {
		t.Errorf("Third AddVerbose() produced %q; want %q", act, exp)
	}
	// The persistent area should scroll when another persistent line is added.
	dpy.AddPersistent("persist 4")
	if act, exp := tm.String(), "persist 3\npersist 4\nstatus    \nverbose 2\nverbose 3\n"; act != exp {
		t.Errorf("Fourth AddPersistent() produced %q; want %q", act, exp)
	}
}

func TestClose(t *testing.T) {
	tm, dpy := initDisplayTest(t, 5, 6, 2)

	dpy.SetStatus("status")
	dpy.AddPersistent("hist 1")
	dpy.AddPersistent("hist 2")
	dpy.AddVerbose("vrb 1")
	dpy.AddVerbose("vrb 2")
	if act, exp := tm.String(), "hist 1\nhist 2\nstatus\nvrb 1\nvrb 2\n"; act != exp {
		t.Errorf("Initial state is %q; want %q", act, exp)
	}

	dpy.Close()
	// The verbose and status lines should be cleared.
	if act, exp := tm.String(), "hist 1\nhist 2\n\n\n\n"; act != exp {
		t.Errorf("Close produced %q; want %q", act, exp)
	}
	// The cursor should be moved to the beginning of the status line.
	if tm.curRow != 2 || tm.curCol != 0 {
		t.Errorf("Cursor is at (%d, %d); want (2, 0)", tm.curRow, tm.curCol)
	}
	if tm.scrollStart != 0 || tm.scrollEnd != 4 {
		t.Errorf("Term scroll region not reset (start=%v, end=%v)", tm.scrollStart, tm.scrollEnd)
	}
	if tm.openHandles != 0 {
		t.Errorf("Term left in raw mode (%d open handle(s))", tm.openHandles)
	}
	if tm.reverse || !tm.cursorVisible || !tm.lineWrap {
		t.Errorf("Term not returned to valid state (reverse=%v, visible=%v, wrap=%v)",
			tm.reverse, tm.cursorVisible, tm.lineWrap)
	}
}
