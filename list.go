package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockproto"
)

func cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	socketFlag := fs.String("socket", "", "override Unix socket path")
	fs.Parse(args)

	sockPath, err := sockpath.Resolve(sockpath.Options{Flag: *socketFlag})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux list: %v\n", err)
		return 1
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux list: dial %s: %v\n", sockPath, err)
		return 1
	}
	defer conn.Close()
	enc := sockproto.NewEncoder(conn)
	dec := sockproto.NewDecoder(conn)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpList}); err != nil {
		fmt.Fprintf(os.Stderr, "hostmux list: %v\n", err)
		return 1
	}
	resp, err := dec.Decode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux list: %v\n", err)
		return 1
	}
	if !resp.Ok {
		fmt.Fprintf(os.Stderr, "hostmux list: %s\n", resp.Error)
		return 1
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SOURCE\tHOSTS\tUPSTREAM")
	for _, e := range resp.Entries {
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.Source, strings.Join(e.Hosts, ","), e.Upstream)
	}
	w.Flush()
	return 0
}
