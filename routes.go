package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockproto"
)

type routesOptions struct {
	SocketPath string
	Writer     io.Writer
}

func runRoutes(opts routesOptions) error {
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
	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SOURCE\tHOSTS\tUPSTREAM")
	for _, e := range resp.Entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Source, strings.Join(e.Hosts, ","), e.Upstream)
	}
	return tw.Flush()
}
