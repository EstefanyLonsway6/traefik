package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// startTruncatingUpstream starts a raw TCP server that answers any request with
// a chunked "200 OK", sends two chunks and then abruptly closes the connection
// without writing the terminating "0\r\n\r\n" chunk.
func startTruncatingUpstream(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Drain the request head.
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil || strings.TrimSpace(line) == "" {
				break
			}
		}

		fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
		fmt.Fprint(conn, "7\r\nchunk-1\r\n")
		fmt.Fprint(conn, "7\r\nchunk-2\r\n")
	}()

	return ln.Addr().String()
}

// openUpstreamStream performs a request against the raw upstream and returns its
// still-open response body.
func openUpstreamStream(t *testing.T, addr string) io.ReadCloser {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial upstream: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: upstream\r\n\r\n"); err != nil {
		t.Fatalf("write upstream request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read upstream response: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp.Body
}

func TestProxyHandlerDoesNotAppendErrorToStreamedBody(t *testing.T) {
	upstreamBody := openUpstreamStream(t, startTruncatingUpstream(t))

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ProxyHandler(w, r, upstreamBody)
	}))
	defer proxy.Close()

	resp, err := http.Get(proxy.URL)
	if err != nil {
		t.Fatalf("GET proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, readErr := io.ReadAll(resp.Body)
	got := string(body)

	if readErr == nil {
		t.Error("reading the proxied body succeeded, want a truncation error signalling the aborted connection")
	}
	if !strings.Contains(got, "chunk-1") || !strings.Contains(got, "chunk-2") {
		t.Errorf("client did not receive both upstream chunks, body = %q", got)
	}
	if strings.Contains(got, "Internal Server Error") {
		t.Errorf("error text was appended to the streamed body: %q", got)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

func TestProxyHandlerReturnsBadGatewayBeforeHeadersAreWritten(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ProxyHandler(w, r, failingReader{})
	}))
	defer proxy.Close()

	resp, err := http.Get(proxy.URL)
	if err != nil {
		t.Fatalf("GET proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read proxy body: %v", err)
	}
	if !strings.Contains(string(body), http.StatusText(http.StatusBadGateway)) {
		t.Errorf("missing error payload, body = %q", body)
	}
}

func TestProxyHandlerForwardsSuccessfulStream(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ProxyHandler(w, r, strings.NewReader("data: hello\n\n"))
	}))
	defer proxy.Close()

	resp, err := http.Get(proxy.URL)
	if err != nil {
		t.Fatalf("GET proxy: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read proxy body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if string(body) != "data: hello\n\n" {
		t.Errorf("body = %q, want %q", body, "data: hello\n\n")
	}
}
