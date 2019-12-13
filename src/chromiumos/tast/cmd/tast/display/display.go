// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package display provides advanced methods of logging to a terminal.
package display

import (
	"strings"
)

// Display divides the screen into multiple independently-scrollable logging areas.
//
// Output is separated into three areas:
//
//	+------------+
//	| Persistent |
//	| ...        |
//	+------------+
//	| Status     |
//	+------------+
//	| Verbose    |
//	| ...        |
//	+------------+
//
// The persistent area contains important messages that should remain onscreen
// for as long as possible. New messages are logged at the bottom of the area,
// which grows upward until it reaches the top of the screen.
//
// The status line is used to display the current progress and can be updated
// frequently.
//
// The verbose area contains transient messages. It's assumed that only the last
// few verbose messages are still relevent. New messages are logged at the bottom
// of the area, which grows downward, but only to a fixed maximum height.
// If the area reaches the bottom of the screen before hitting its maximum height,
// all areas are pushed upward.
//
// TODO(derat): Detect screen resize (with SIGWINCH?) and reinitialize everything.
type Display struct {
	tm             Term
	screenRows     int
	screenCols     int
	statusRow      int // status row (1-indexed)
	numVerboseRows int // current number of verbose lines
	maxVerboseRows int // maximum number of verbose lines to display
}

// New returns a new Display backed by tm. The verbose area can grow to
// maxVerboseRows rows before scrolling.
func New(tm Term, maxVerboseRows int) (*Display, error) {
	if err := tm.check(); err != nil {
		return nil, err
	}

	d := &Display{
		tm:             tm,
		numVerboseRows: 0,
		maxVerboseRows: maxVerboseRows,
	}

	th := d.tm.newHandle()
	d.screenRows, d.screenCols, _ = th.getSize()
	d.statusRow, _, _ = th.getCursorPos()
	th.cursorVisible(false)
	th.lineWrap(false)
	return d, th.close()
}

// Close closes d. The status row and verbose area are cleared,
// and the cursor is left at the beginning of the status row's line.
func (d *Display) Close() error {
	th := d.tm.newHandle()
	th.setScrollRegion(-1, -1)
	th.clearAttr()
	th.lineWrap(true)
	th.setCursorPos(d.statusRow, 1)
	th.clearToBottom()
	th.cursorVisible(true)
	return th.close()
}

// scrollForStatusAndVerbose vertically scrolls the whole screen to accomodate the status row
// and verbose area.
func (d *Display) scrollForStatusAndVerbose(th termHandle) {
	if d.statusRow+d.numVerboseRows <= d.screenRows {
		return
	}
	nr := d.statusRow + d.numVerboseRows - d.screenRows
	th.scroll(nr)
	d.statusRow -= nr
}

// scrollRegion vertically scrolls the region between 1-indexed rows start and end.
// The scroll region is reset at completion.
func (d *Display) scrollRegion(th termHandle, start, end, rows int) {
	th.setScrollRegion(start, end)
	th.scroll(rows)
	th.setScrollRegion(-1, -1)
}

// SetStatus sets the status line to s.
func (d *Display) SetStatus(s string) error {
	// If we got multiple lines, just use the last non-empty one.
	s = strings.TrimRight(s, "\n")
	if lines := strings.Split(s, "\n"); len(lines) > 1 {
		s = lines[len(lines)-1]
	}

	// Pad the string to the end of the screen since we're writing it with reverse video.
	if len(s) < d.screenCols {
		s = s + strings.Repeat(" ", d.screenCols-len(s))
	}

	th := d.tm.newHandle()
	d.scrollForStatusAndVerbose(th)
	th.setCursorPos(d.statusRow, 1)
	th.clearLine()
	th.reverse()

	// With line-wrap disabled, writing beyond the end of the line repeatedly overwrites
	// the final column, so truncate to avoid this.
	if len(s) > d.screenCols {
		s = s[:d.screenCols]
	}
	th.writeString(s)
	th.clearAttr()
	return th.close()
}

// callForEachLine invokes f for each line in s. If f returns an error, it's immediately
// returned to the caller.
func callForEachLine(s string, f func(string) error) error {
	var err error
	for _, ln := range strings.Split(s, "\n") {
		if err = f(ln); err != nil {
			return err
		}
	}
	return nil
}

// AddPersistent adds s to the bottom of the persistent area.
// Long lines are truncated, and trailing empty lines are dropped.
func (d *Display) AddPersistent(s string) error {
	s = strings.TrimRight(s, "\n")
	if strings.Contains(s, "\n") {
		return callForEachLine(s, d.AddPersistent)
	}

	pr := 0
	th := d.tm.newHandle()
	if d.statusRow+d.numVerboseRows == d.screenRows {
		// If the status and verbose area are already flush with the bottom of the screen,
		// we need to shift the existing persistent data up one line.
		pr = d.statusRow - 1
		d.scrollRegion(th, 1, pr, 1)
	} else {
		// Otherwise, we need to shift the status and verbose area down one line.
		pr = d.statusRow
		d.scrollRegion(th, d.statusRow, d.screenRows, -1)
		d.statusRow++
	}

	th.setCursorPos(pr, 1)
	th.clearLine()
	// TODO(derat): Wrap long lines instead of truncating.
	// It probably makes sense to let the caller supply an indent width for this
	// so that the wrapped lines can be indented beyond the logging prefix.
	if len(s) > d.screenCols {
		s = s[:d.screenCols]
	}
	th.writeString(s)
	return th.close()
}

// AddVerbose adds s to the bottom of the persistent area.
// Long lines are truncated, and trailing empty lines are dropped.
func (d *Display) AddVerbose(s string) error {
	s = strings.TrimRight(s, "\n")
	if strings.Contains(s, "\n") {
		return callForEachLine(s, d.AddVerbose)
	}

	th := d.tm.newHandle()
	if d.numVerboseRows < d.maxVerboseRows {
		d.numVerboseRows++
		d.scrollForStatusAndVerbose(th)
	} else {
		d.scrollRegion(th, d.statusRow+1, d.statusRow+d.numVerboseRows, 1)
	}

	th.setCursorPos(d.statusRow+d.numVerboseRows, 1)
	th.clearLine()
	// TODO(derat): Wrap long lines here too.
	if len(s) > d.screenCols {
		s = s[:d.screenCols]
	}
	th.writeString(s)
	return th.close()
}
