package file

import (
	"errors"
	"strings"
	"testing"
)

type failingLocalWriter struct {
	writeErr error
	closeErr error
	closed   bool
}

func (w *failingLocalWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(p), nil
}

func (w *failingLocalWriter) Close() error {
	w.closed = true
	return w.closeErr
}

func TestLocalWritePreservesCopyAndCloseErrors(t *testing.T) {
	writeErr := errors.New("disk write failed")
	closeErr := errors.New("delayed close failed")
	for _, failedWrite := range []bool{false, true} {
		writer := &failingLocalWriter{closeErr: closeErr}
		if failedWrite {
			writer.writeErr = writeErr
		}
		err := copyAndCloseLocalFile(writer, strings.NewReader("data"))
		if !writer.closed || !errors.Is(err, closeErr) {
			t.Fatalf("close failure must be reported: closed=%v error=%v", writer.closed, err)
		}
		if failedWrite && !errors.Is(err, writeErr) {
			t.Fatalf("copy failure must be retained: %v", err)
		}
	}
}
