package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

// ProxyHandler simulates the proxy logic where the issue occurs.
func ProxyHandler(w http.ResponseWriter, r *http.Request, upstream io.Reader) {
	// In a real scenario, we would track if headers were written.
	// For this implementation, we simulate the copy loop.
	_, err := io.Copy(w, upstream)
	if err != nil {
		// Check if headers were already sent (simplified check)
		if !w.Written() {
			// If headers were not sent, we can write an error response
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		} else {
			// If headers were sent, log the error but do not write to the response body
			log.Printf("Upstream error: %v", err)
		}
		return
	}
}

func main() {
	fmt.Println("Proxy logic ready for integration.")
}