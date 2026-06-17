package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockproto"
)

type routesOptions struct {
	SocketPath string
	JSON       bool
	Wide       bool
	Writer     io.Writer
	// now is injectable for deterministic age formatting in tests.
	now func() time.Time
}

// routeJSON is the stable JSON shape emitted by `hostmux routes --json`.
type routeJSON struct {
	Source       string            `json:"source"`
	Hosts        []string          `json:"hosts"`
	Upstream     string            `json:"upstream"`
	Labels       map[string]string `json:"labels,omitempty"`
	PID          int               `json:"pid,omitempty"`
	Command      string            `json:"command,omitempty"`
	Cwd          string            `json:"cwd,omitempty"`
	RegisteredAt int64             `json:"registered_at,omitempty"`
	AgeSeconds   int64             `json:"age_seconds,omitempty"`
}

func runRoutes(opts routesOptions) error {
	now := opts.now
	if now == nil {
		now = time.Now
	}
	sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux routes: %v\n", err)
		return exitError{code: 1}
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux routes: dial %s: %v\n", sockPath, err)
		return exitError{code: 1}
	}
	defer conn.Close()

	enc := sockproto.NewEncoder(conn)
	dec := sockproto.NewDecoder(conn)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpList}); err != nil {
		fmt.Fprintf(os.Stderr, "hostmux routes: %v\n", err)
		return exitError{code: 1}
	}
	resp, err := dec.Decode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux routes: %v\n", err)
		return exitError{code: 1}
	}
	if !resp.Ok {
		fmt.Fprintf(os.Stderr, "hostmux routes: %s\n", resp.Error)
		return exitError{code: 1}
	}

	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}

	if opts.JSON {
		return writeRoutesJSON(writer, resp.Entries, now())
	}
	return writeRoutesTable(writer, resp.Entries, opts.Wide, now())
}

func writeRoutesJSON(w io.Writer, entries []sockproto.Entry, now time.Time) error {
	out := make([]routeJSON, 0, len(entries))
	for _, e := range entries {
		rj := routeJSON{
			Source:       e.Source,
			Hosts:        e.Hosts,
			Upstream:     e.Upstream,
			Labels:       e.Labels,
			PID:          e.PID,
			Command:      e.Command,
			Cwd:          e.Cwd,
			RegisteredAt: e.RegisteredAt,
		}
		if e.RegisteredAt > 0 {
			if age := now.Unix() - e.RegisteredAt; age >= 0 {
				rj.AgeSeconds = age
			}
		}
		out = append(out, rj)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeRoutesTable(w io.Writer, entries []sockproto.Entry, wide bool, now time.Time) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if wide {
		fmt.Fprintln(tw, "SOURCE\tHOSTS\tUPSTREAM\tAGE\tPID\tLABELS\tCOMMAND")
	} else {
		fmt.Fprintln(tw, "SOURCE\tHOSTS\tUPSTREAM\tAGE")
	}
	for _, e := range entries {
		age := formatAge(e.RegisteredAt, now)
		if wide {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				e.Source,
				strings.Join(e.Hosts, ","),
				e.Upstream,
				age,
				pidString(e.PID),
				dashIfEmpty(formatLabels(e.Labels)),
				dashIfEmpty(e.Command),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				e.Source,
				strings.Join(e.Hosts, ","),
				e.Upstream,
				age,
			)
		}
	}
	return tw.Flush()
}

func pidString(pid int) string {
	if pid <= 0 {
		return "-"
	}
	return strconv.Itoa(pid)
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// formatAge renders the age of a route given its registered-at unix time.
// Returns "-" when the time is unknown (older daemon or unstamped entry).
func formatAge(registeredAt int64, now time.Time) string {
	if registeredAt <= 0 {
		return "-"
	}
	d := now.Sub(time.Unix(registeredAt, 0))
	return humanizeDuration(d)
}

// humanizeDuration renders a short, coarse age like "3s", "5m", "2h", "4d".
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}
