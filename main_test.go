package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorReader simulates an upstream that sends data then disconnects mid-stream.
type errorReader struct {
	data  []byte
	pos   int
	err   error
	errAt int // byte position where error triggers; -1 = immediate on first read
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.errAt == -1 {
		return 0, r.err
	}
	if r.errAt >= 0 && r.pos > r.errAt {
		return 0, r.err
	}
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	remaining := len(r.data) - r.pos
	if r.errAt >= 0 && r.pos+remaining > r.errAt+1 {
		remaining = r.errAt + 1 - r.pos
	}
	n := copy(p, r.data[r.pos:r.pos+remaining])
	r.pos += n
	// Check if we've reached or passed the error point after this read
	if r.errAt >= 0 && r.pos > r.errAt {
		return n, r.err
	}
	return n, nil
}

func TestProxyHandler_StreamingErrorNoErrorTextAppended(t *testing.T) {
	chunks := strings.Repeat("data: chunk\n\n", 5)
	reader := &errorReader{
		data:  []byte(chunks),
		err:   errors.New("connection reset by peer"),
		errAt: len(chunks) / 2,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)

	ProxyHandler(rec, req, reader)

	body := rec.Body.String()

	if strings.Contains(body, "Internal Server Error") {
		t.Errorf("Streaming error appended 'Internal Server Error' to body: %q", body)
	}

	if !strings.Contains(body, "data: chunk") {
		t.Error("Expected partial data to be written before error")
	}
}

func TestProxyHandler_PreHeadersErrorReturns502(t *testing.T) {
	reader := &errorReader{
		data:  []byte{},
		err:   errors.New("upstream unreachable"),
		errAt: -1,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api", nil)

	ProxyHandler(rec, req, reader)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("Expected 502 Bad Gateway for pre-headers error, got %d", rec.Code)
	}
}

func TestProxyHandler_SuccessfulStream(t *testing.T) {
	data := "data: hello\n\ndata: world\n\n"
	reader := strings.NewReader(data)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)

	ProxyHandler(rec, req, reader)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rec.Code)
	}
	if rec.Body.String() != data {
		t.Errorf("Expected body %q, got %q", data, rec.Body.String())
	}
}

func TestProxyHandler_ImmediateErrorReturns502(t *testing.T) {
	reader := &errorReader{
		data:  []byte{},
		err:   fmt.Errorf("dial tcp: connection refused"),
		errAt: -1,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	ProxyHandler(rec, req, reader)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("Expected 502 for immediate upstream failure, got %d", rec.Code)
	}
}
