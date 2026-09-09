// Existing imports and code...

func (p *FastProxy) serveHTTP(w http.ResponseWriter, req *http.Request) {
    // ... existing setup code ...

    var headersWritten bool
    recorder := &responseRecorder{ResponseWriter: w} // Custom recorder to track headers

    upstream, err := p.getUpstream(req)
    if err != nil {
        if !headersWritten {
            w.WriteHeader(http.StatusBadGateway)
            w.Write([]byte("Internal Server Error"))
        }
        return
    }
    defer upstream.Close()

    // ... existing proxy logic ...

    go func() {
        _, err := io.Copy(recorder, upstream)
        if err != nil {
            // Handle upstream read error
            if !recorder.headersWritten {
                // Safe to write error response
                w.WriteHeader(http.StatusBadGateway)
                w.Write([]byte("Internal Server Error"))
            } else {
                // Headers already flushed: close connection
                p.closeClientConnection(w)
            }
        }
        // ... existing cleanup ...
    }()

    // ... rest of handler ...
}

// Helper to close client connection when possible
func (p *FastProxy) closeClientConnection(w http.ResponseWriter) {
    if hijacker, ok := w.(http.Hijacker); ok {
        conn, _, err := hijacker.Hijack()
        if err == nil {
            conn.Close()
        }
    }
}

// responseRecorder wraps ResponseWriter to track header writes
type responseRecorder struct {
    http.ResponseWriter
    headersWritten bool
}

func (r *responseRecorder) WriteHeader(code int) {
    if !r.headersWritten {
        r.headersWritten = true
        r.ResponseWriter.WriteHeader(code)
    }
}

func (r *responseRecorder) Write(data []byte) (int, error) {
    if !r.headersWritten {
        r.headersWritten = true
        r.ResponseWriter.WriteHeader(http.StatusOK)
    }
    return r.ResponseWriter.Write(data)
}
