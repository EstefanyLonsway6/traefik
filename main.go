package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
)

// streamWriter wraps an http.ResponseWriter and records whether any part of the
// response (status line, headers or body) has already been handed to the
// client. Once that happened the status code can no longer be changed, so an
// upstream failure must not be turned into an error payload: writing one would
// corrupt the stream that the client is already parsing.
type streamWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (s *streamWriter) WriteHeader(statusCode int) {
	if s.wroteHeader {
		return
	}
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(statusCode)
}

func (s *streamWriter) Write(p []byte) (int, error) {
	s.wroteHeader = true
	return s.ResponseWriter.Write(p)
}

// Flush commits the status line and headers even when not a single body byte
// has been written yet, which is the usual case for SSE and chunked streams.
func (s *streamWriter) Flush() {
	s.wroteHeader = true
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ProxyHandler streams the upstream response to the client.
func ProxyHandler(w http.ResponseWriter, r *http.Request, upstream io.Reader) {
	sw := &streamWriter{ResponseWriter: w}

	err := streamCopy(sw, upstream)
	if err == nil {
		return
	}

	log.Printf("fastproxy: upstream error while proxying %s: %v", r.URL, err)

	if !sw.wroteHeader {
		// Nothing reached the client yet, so a regular error response is still
		// possible and the status code is meaningful.
		http.Error(sw, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	// The headers (and possibly part of the body) are already on the wire.
	// Appending an error message here would corrupt the payload, so terminate
	// the response instead and let the client detect the truncation.
	abortResponse(sw)
}

// streamCopy copies src to dst, flushing after every chunk so that streaming
// responses are forwarded as they arrive instead of being buffered by the
// proxy. Data read before an error is always forwarded to the client.
func streamCopy(dst *streamWriter, src io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			dst.Flush()
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

// abortResponse terminates an in-flight response without appending anything to
// the body.
//
// For HTTP/1.1 the underlying connection is hijacked and closed. Closing it
// without writing the terminating zero-length chunk is exactly what signals a
// truncated body to the client; already flushed bytes are preserved because
// Hijack flushes the pending chunked writer first. For HTTP/2, which cannot be
// hijacked, panicking with http.ErrAbortHandler makes net/http reset the stream
// (RST_STREAM) without logging a stack trace.
func abortResponse(w http.ResponseWriter) {
	if hijacker, ok := w.(http.Hijacker); ok {
		if conn, brw, err := hijacker.Hijack(); err == nil {
			if brw != nil {
				_ = brw.Flush()
			}
			_ = conn.Close()
			return
		}
	}

	panic(http.ErrAbortHandler)
}

func main() {
	fmt.Println("Proxy logic ready for integration.")
}
