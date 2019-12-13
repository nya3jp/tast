// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

// Term is used to control a terminal.
type Term interface {
	// check verifies that the current terminal provides required functionality.
	check() error
	// newHandle returns a handle that can be used to perform a series of operations.
	newHandle() termHandle
}

// termHandle is used to perform a series of operations on a terminal.
// When the handle is closed, the terminal's original state is restored.
// Most functions return the handle itself to simplify chaining commands.
type termHandle interface {
	// close closes the handle (e.g. taking the terminal out of raw mode).
	// It must be called on completion of the operations.
	// It returns the first error encountered, if any.
	close() error
	// writeString writes text to the terminal starting at the current cursor position.
	writeString(s string) termHandle
	// reverse configures the terminal to write inverted text.
	reverse() termHandle
	// clearAttr clears all currently-set display attributes. This only refers to reverse.
	clearAttr() termHandle
	// cursorVisible shows or hides the cursor.
	cursorVisible(vis bool) termHandle
	// lineWrap controls if lines are automatically wrapped or not.
	lineWrap(wrap bool) termHandle
	// clearLine clears the entire current line.
	clearLine() termHandle
	// clearToBottom clears from the current row to the bottom row.
	clearToBottom() termHandle
	// scroll scrolls the current scroll region. Positive values push the existing
	// text upwards.
	scroll(lines int) termHandle
	// setScrollRegion sets the region of the screen to vertically scroll.
	// start and end are 1-indexed rows. A negative start or end resets the region
	// so the whole screen will be scrolled.
	setScrollRegion(start, end int) termHandle
	// setCursorPos moves the cursor to the requested position. row and col are 1-indexed.
	setCursorPos(row, col int) termHandle
	// saveCursorPos saves the current cursor position for a later call to restoreCursorPos.
	saveCursorPos() termHandle
	// restoreCursorPos restores the cursor position from an earlier call to saveCursorPos.
	restoreCursorPos() termHandle
	// getCursorPos gets the current 1-indexed cursor coordinates from the terminal.
	getCursorPos() (row, col int, err error)
	// getSize returns the dimensions of the terminal window.
	getSize() (rows, cols int, err error)
}

// VT100Term controls a VT100 terminal.
// TODO(derat): Extract escape sequences from the system terminfo database instead of hardcoding them.
type VT100Term struct{}

func (t *VT100Term) check() error {
	if !terminal.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("stdin isn't a terminal")
	}
	if os.Getenv("TMUX") != "" {
		return errors.New("tmux doesn't support VT100 scrolling sequences")
	}
	return nil
}

func (t *VT100Term) newHandle() termHandle {
	c := &vt100TermHandle{}
	c.origState, c.rawErr = terminal.MakeRaw(int(os.Stdin.Fd()))
	return c
}

type vt100TermHandle struct {
	origState *terminal.State // state before going into raw mode
	rawErr    error           // error seen when going into raw mode
	opErr     error           // first error seen from some other operation
}

func (th *vt100TermHandle) close() error {
	if th.rawErr != nil {
		return th.rawErr
	}
	resErr := terminal.Restore(int(os.Stdin.Fd()), th.origState)
	if th.opErr != nil {
		return th.opErr
	}
	return resErr
}

func (th *vt100TermHandle) writeString(s string) termHandle {
	if th.opErr == nil {
		_, th.opErr = os.Stdout.WriteString(s)
	}
	return th
}

func (th *vt100TermHandle) writeEscSeq(s string) {
	// Some occasionally-conflicting resources for escape codes:
	// https://en.wikipedia.org/wiki/ANSI_escape_code
	// http://www.termsys.demon.co.uk/vtansi.htm
	// http://ascii-table.com/ansi-escape-sequences-vt-100.php
	if th.opErr == nil {
		_, th.opErr = os.Stdout.WriteString("\033[" + s)
	}
}

func (th *vt100TermHandle) reverse() termHandle {
	th.writeEscSeq("7m")
	return th
}

func (th *vt100TermHandle) clearAttr() termHandle {
	th.writeEscSeq("0m")
	return th
}

func (th *vt100TermHandle) cursorVisible(vis bool) termHandle {
	if vis {
		th.writeEscSeq("?25h")
	} else {
		th.writeEscSeq("?25l")
	}
	return th
}

func (th *vt100TermHandle) lineWrap(wrap bool) termHandle {
	if wrap {
		th.writeEscSeq("?7h")
	} else {
		th.writeEscSeq("?7l")
	}
	return th
}

func (th *vt100TermHandle) clearLine() termHandle {
	th.writeEscSeq("2K")
	return th
}

func (th *vt100TermHandle) clearToBottom() termHandle {
	th.writeEscSeq("J")
	return th
}

func (th *vt100TermHandle) scroll(lines int) termHandle {
	// TODO(derat): Find an alternative for tmux.
	if lines > 0 {
		th.writeEscSeq(fmt.Sprintf("%dS", lines))
	} else if lines < 0 {
		th.writeEscSeq(fmt.Sprintf("%dT", -lines))
	}
	return th
}

func (th *vt100TermHandle) setScrollRegion(start, end int) termHandle {
	if start < 0 || end < 0 {
		th.writeEscSeq("r")
	} else {
		th.writeEscSeq(fmt.Sprintf("%d;%dr", start, end))
	}
	return th
}

func (th *vt100TermHandle) setCursorPos(row, col int) termHandle {
	th.writeEscSeq(fmt.Sprintf("%d;%df", row, col))
	return th
}

func (th *vt100TermHandle) saveCursorPos() termHandle {
	th.writeEscSeq("s")
	return th
}

func (th *vt100TermHandle) restoreCursorPos() termHandle {
	th.writeEscSeq("u")
	return th
}

func (th *vt100TermHandle) readCursorPos() (row, col int, err error) {
	b, err := bufio.NewReader(os.Stdin).ReadBytes('R')
	if err != nil {
		return 0, 0, err
	}
	if len(b) < 6 || b[0] != '\033' || b[1] != '[' {
		return 0, 0, fmt.Errorf("didn't get expected escape code or data (read %v)", b)
	}

	p := strings.Split(string(b[2:len(b)-1]), ";")
	if len(p) != 2 {
		return 0, 0, fmt.Errorf("didn't get expected ROW;COLUMN data (read %v)", b)
	}

	d := make([]int, 2)
	for i := 0; i < 2; i++ {
		v, err := strconv.ParseInt(p[i], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse cursor coordinate from %q", p[i])
		}
		d[i] = int(v)
	}
	return d[0], d[1], nil
}

func (th *vt100TermHandle) getCursorPos() (row, col int, err error) {
	// When the terminal sees this, it should write "<ESC>[{ROW};{COLUMN}R".
	if th.writeEscSeq("6n"); th.opErr != nil {
		return 0, 0, err
	}
	row, col, th.opErr = th.readCursorPos()
	return row, col, th.opErr
}

func (th *vt100TermHandle) getSize() (rows, cols int, err error) {
	th.saveCursorPos()
	defer th.restoreCursorPos()

	// Try to warp way out to the bottom right and see where we end up.
	th.setCursorPos(999, 999)
	return th.getCursorPos()
}
