package test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Captures stdout for the duration of a function call.
func withStdoutCapture(fn func() error) (map[string]any, error) {
	// Before calling the function, redirect stdout.
	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", pipeErr)
	}
	os.Stdout = w

	// Execute the function and capture its error.
	err := fn()

	// After the function call, restore stdout.
	os.Stdout = oldStdout
	if err != nil {
		r.Close()
		w.Close()
		return nil, err
	}

	vals := make(map[string]any)
	vals["r"] = r
	vals["w"] = w

	return vals, nil
}

func setupForLoggingTest() (url string, stop func(), logs *bytes.Buffer, err error) {
	gateSrv, backendSrv, vals, err := setupGatewayCore(withStdoutCapture)
	r, w := vals["r"].(*os.File), vals["w"].(*os.File)

	// Start goroutine to copy from pipe reader into a buffer.
	buf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(buf, r)
		close(done)
	}()

	stopFunc := func() {
		gateSrv.Close()
		backendSrv.Close()
		// close writer to unblock reader
		w.Close()
		// wait a short time for copy to finish
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
	}
	return gateSrv.URL, stopFunc, buf, err
}

func TestRequestLogging(t *testing.T) {
	url, stop, logs, err := setupForLoggingTest()
	if err != nil {
		t.Fatalf("failed to start gateway: %v", err)
	}
	defer stop()

	resp, err := http.Get(url + "/api/users")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// checking response
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Fatalf("missing X-Request-ID header")
	}

	// allow async logger to flush
	time.Sleep(50 * time.Millisecond)

	// checking logs
	out := logs.String()
	if !strings.Contains(out, `"http_path":"/api/users"`) {
		t.Fatalf("log did not contain path; got: %s", out)
	}
	if !strings.Contains(out, reqID) {
		t.Fatalf("log did not include request id %s; got: %s", reqID, out)
	}
}
