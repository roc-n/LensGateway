package test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRequestLogging(t *testing.T) {
	url, stop, buf, err := setupForLoggingTest()
	if err != nil {
		t.Fatalf("failed to start gateway: %v", err)
	}
	defer stop()

	fmt.Println(url)
	resp, err := http.Get(url + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Fatalf("missing X-Request-ID header")
	}

	// allow async logger to flush
	time.Sleep(50 * time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, `"http_path":"/"`) {
		t.Fatalf("log did not contain path; got: %s", out)
	}
	if !strings.Contains(out, reqID) {
		t.Fatalf("log did not include request id %s; got: %s", reqID, out)
	}
}
