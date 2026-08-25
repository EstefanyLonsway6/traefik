// This is a placeholder for the logic required to fix the issue.
// In a real scenario, we would check if headers are already written.
// before attempting to write an error to the response body.

func (p *Proxy) handleStreamError(w http.ResponseWriter, err error) {
    if headersSent(w) {
        // If headers are sent, we cannot write an error message.
        // We must close the connection to signal truncation.
        if hijacker, ok := w.(http.Hijacker); ok {
            conn, _, err := hijacker.Hijack()
            if err == nil {
                conn.Close()
            }
        } else {
            // Fallback: close the connection if hijacking is not possible
            // but headers are already sent.
            w.Close()
        }
        return
    }
    // If headers are NOT sent, we can safely return a 502/500
    http.Error(w, err.Error(), http.StatusBadGateway)
}