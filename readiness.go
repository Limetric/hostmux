package main

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// readinessResult reports why waitForReady returned.
type readinessResult int

const (
	readyOK readinessResult = iota
	readyTimeout
	readyChildExited
)

// defaultWaitTimeout is how long `hostmux run --wait` polls for upstream
// readiness before giving up (and announcing URLs anyway).
const defaultWaitTimeout = 30 * time.Second

// normalizeWaitPath ensures an HTTP wait path has a leading slash. An empty
// path means "TCP connect only".
func normalizeWaitPath(path string) string {
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

// readyCheck reports whether the upstream at host:port is accepting requests.
// With an empty path it only checks that a TCP connection can be established;
// with a path it issues an HTTP GET and treats a non-error response (status
// < 400) as ready.
func readyCheck(host string, port int, path string) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	if path == "" {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + path)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 400
}

// waitForReady polls readyCheck until the upstream is ready, the timeout
// elapses, or childDone is closed (the child exited before becoming ready).
// pollInterval controls the cadence between checks.
func waitForReady(host string, port int, path string, timeout, pollInterval time.Duration, childDone <-chan struct{}, now func() time.Time) readinessResult {
	if now == nil {
		now = time.Now
	}
	deadline := now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		// Don't poll after the child has already exited.
		select {
		case <-childDone:
			return readyChildExited
		default:
		}
		if readyCheck(host, port, path) {
			return readyOK
		}
		select {
		case <-childDone:
			return readyChildExited
		case <-ticker.C:
			if now().After(deadline) {
				return readyTimeout
			}
		}
	}
}
