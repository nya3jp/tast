// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporting

import (
	"encoding/json"
	"io"
	"os"

	"chromiumos/tast/internal/run/resultsjson"
)

// StreamedResultsFilename is a file name to be used with StreamedWriter.
const StreamedResultsFilename = "streamed_results.jsonl"

// StreamedWriter writes a stream of JSON-marshaled jsonresults.Result objects
// to a file.
type StreamedWriter struct {
	f          *os.File
	enc        *json.Encoder
	lastOffset int64 // file offset of the start of the last-written result
}

// NewStreamedWriter creates and returns a new StreamedWriter for writing to
// a file at path.
// If the file already exists, new results are appended to it.
func NewStreamedWriter(path string) (*StreamedWriter, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	eof, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &StreamedWriter{f: f, enc: json.NewEncoder(f), lastOffset: eof}, nil
}

// Close closes the underlying file.
func (w *StreamedWriter) Close() {
	w.f.Close()
}

// Write writes the JSON-marshaled representation of res to the file.
// If update is true, the previous result that was written by this instance is overwritten.
// Concurrent calls are not supported (note that tests are run serially, and runners send
// control messages to the tast process serially as well).
func (w *StreamedWriter) Write(res *resultsjson.Result, update bool) error {
	var err error
	if update {
		// If we're replacing the last record, seek back to the beginning of it and leave the saved offset unmodified.
		if _, err = w.f.Seek(w.lastOffset, io.SeekStart); err != nil {
			return err
		}
		if err = w.f.Truncate(w.lastOffset); err != nil {
			return err
		}
	} else {
		// Otherwise, use Seek to record the current offset before we write.
		if w.lastOffset, err = w.f.Seek(0, io.SeekCurrent); err != nil {
			return err
		}
	}

	return w.enc.Encode(res)
}
