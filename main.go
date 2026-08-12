package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

// trackingResponseWriter wraps http.ResponseWriter to track whether headers
// have been written. This is essential for distinguishing between streaming
// errors (headers already flushed) and pre-response errors.
type trackingResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (t *trackingResponseWriter) WriteHeader(statusCode int) {
	t.wroteHeader = true
	t.ResponseWriter.WriteHeader(statusCode)
}

func (t *trackingResponseWriter) Write(b []byte) (int, error) {
	t.wroteHeader = true
	return t.ResponseWriter.Write(b)
}

// Flush implements http.Flusher so streaming protocols (SSE, chunked) work.
func (t *trackingResponseWriter) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ProxyHandler handles proxying an upstream response to the client.
// It properly distinguishes between errors that occur before headers are
// flushed (return 502 Bad Gateway) and errors that occur during streaming
// (abruptly close the connection without appending error text).
func ProxyHandler(w http.ResponseWriter, r *http.Request, upstream io.Reader) {
	tw := &trackingResponseWriter{ResponseWriter: w}

	_, err := io.Copy(tw, upstream)
	if err != nil {
		if tw.wroteHeader {
			// Headers already sent — we are mid-stream.
			// Do NOT append error text to the body; it would corrupt
			// the stream for clients (EventSource, SSE parsers, etc.).
			// Instead, abruptly close the connection to signal truncation.
			log.Printf("Upstream stream error (headers already sent): %v — closing connection", err)
			closeClientConnection(w)
			return
		}
		// Headers NOT yet sent — safe to return a proper error status.
		log.Printf("Upstream error before response: %v — returning 502", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		return
	}
}

// closeClientConnection attempts to hijack the underlying TCP connection
// and close it abruptly, signaling to the client that the stream was
// truncated. This is the correct behavior for mid-stream upstream failures
// in streaming protocols (SSE, chunked transfer).
func closeClientConnection(w http.ResponseWriter) {
	if hj, ok := w.(http.Hijacker); ok {
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			log.Printf("Failed to hijack connection: %v", err)
			return
		}
		// Discard any buffered data and close the raw connection.
		bufrw.Flush()
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetLinger(0)
		}
		conn.Close()
	}
}

func main() {
	fmt.Println("Proxy logic ready for integration.")
}
