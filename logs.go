package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Limetric/hostmux/internal/daemon"
)

type logsOptions struct {
	Follow bool
	Lines  int // last N lines; 0 means the whole file
	Writer io.Writer
	// pathOverride lets tests point at a temp file instead of ~/.hostmux.
	pathOverride string
	// pollInterval controls follow polling; 0 uses a sane default.
	pollInterval time.Duration
	// stop, when non-nil and closed, ends a --follow loop (used by tests).
	stop <-chan struct{}
}

func runLogs(opts logsOptions) error {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	path := opts.pathOverride
	if path == "" {
		p, err := daemon.LogPath()
		if err != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux logs: %v", err)}
		}
		path = p
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return exitError{code: 1, text: fmt.Sprintf("hostmux logs: no daemon log yet at %s (start the daemon first)", path)}
		}
		return exitError{code: 1, text: fmt.Sprintf("hostmux logs: %v", err)}
	}
	defer f.Close()

	// Snapshot the size BEFORE reading the initial content, and make that the
	// exact boundary where --follow resumes. Anything appended after this
	// point is picked up by the follow loop rather than being skipped in the
	// gap between the initial read and a Seek-to-end.
	fi, err := f.Stat()
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux logs: %v", err)}
	}
	offset := fi.Size()

	if opts.Lines > 0 {
		if err := writeLastLines(w, io.LimitReader(f, offset), opts.Lines); err != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux logs: %v", err)}
		}
	} else {
		if _, err := io.CopyN(w, f, offset); err != nil && !errors.Is(err, io.EOF) {
			return exitError{code: 1, text: fmt.Sprintf("hostmux logs: %v", err)}
		}
	}
	if !opts.Follow {
		return nil
	}

	interval := opts.pollInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	buf := make([]byte, 32*1024)
	for {
		// Honor cancellation even when the log grows fast enough that every
		// read returns data (otherwise the stop channel could be starved).
		select {
		case <-opts.stop:
			return nil
		default:
		}
		n, rerr := f.ReadAt(buf, offset)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return exitError{code: 1, text: fmt.Sprintf("hostmux logs: %v", werr)}
			}
			offset += int64(n)
		}
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return exitError{code: 1, text: fmt.Sprintf("hostmux logs: %v", rerr)}
		}
		if n > 0 {
			// More may be buffered; keep draining before we sleep.
			continue
		}
		select {
		case <-opts.stop:
			return nil
		case <-time.After(interval):
		}
	}
}

// writeLastLines writes the last n lines read from r to w.
func writeLastLines(w io.Writer, r io.Reader, n int) error {
	// Simple, memory-bounded enough for a dev-tool log: keep a ring of the
	// last n lines.
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ring := make([]string, 0, n)
	for sc.Scan() {
		if len(ring) == n {
			ring = ring[1:]
		}
		ring = append(ring, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for _, line := range ring {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
