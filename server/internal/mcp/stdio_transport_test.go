package mcp

import (
	"context"
	"io"
	"os"
	"testing"
)

// A stdio boot must not close the process streams. The SDK's StdioTransport
// closes os.Stdin when the connection ends, and a test process boots the server
// more than once, so the second boot reads a closed descriptor: the DEK guard
// tests failed that way on CI with "read /dev/stdin: file already closed" while
// the code under test was still fine.
func TestStdioTransportLeavesTheProcessStreamsOpen(t *testing.T) {
	conn, err := stdioTransport().Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Stat reports ErrClosed on a closed *os.File, and costs no read.
	for name, f := range map[string]*os.File{"stdin": os.Stdin, "stdout": os.Stdout} {
		if _, err := f.Stat(); err != nil {
			t.Errorf("%s must stay open after the transport closes: %v", name, err)
		}
	}
}

// The writer half still has to work after the transport hands the connection
// its streams: a no-op Close must not make the writer unwritable.
func TestStdioStreamWrappersForwardReadsAndWrites(t *testing.T) {
	var buf writeBuf
	w := openWriter{&buf}
	if _, err := w.Write([]byte("frame")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != "frame" {
		t.Fatalf("wrote %q, want %q", buf.String(), "frame")
	}
	r := openReader{&readBuf{}}
	if _, err := io.ReadAll(r); err != nil {
		t.Fatalf("read: %v", err)
	}
}

type writeBuf struct{ b []byte }

func (w *writeBuf) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *writeBuf) String() string              { return string(w.b) }

type readBuf struct{ done bool }

func (r *readBuf) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, "frame"), nil
}
