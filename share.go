package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockproto"
)

type shareOptions struct {
	SocketPath string
	Domain     string
	Prefix     string
	NoPrefix   bool
	QR         bool
	All        bool
	JSON       bool
	Names      []string
	Writer     io.Writer
}

// shareItem is one shareable route, registered or URL-only.
type shareItem struct {
	Host       string `json:"host"`
	URL        string `json:"url"`
	Registered bool   `json:"registered"`
	Upstream   string `json:"upstream,omitempty"`
	Source     string `json:"source,omitempty"`
}

func runShare(opts shareOptions) error {
	w := writerOr(opts.Writer)

	sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux share: %v", err)}
	}

	// Best-effort daemon lookup for domain/scheme/port and the route table.
	var (
		domain   string
		scheme   = "https"
		port     int
		entries  []sockproto.Entry
		daemonOK bool
	)
	if conn, derr := net.Dial("unix", sockPath); derr == nil {
		defer conn.Close()
		enc := sockproto.NewEncoder(conn)
		dec := sockproto.NewDecoder(conn)
		if d, https, p, ierr := lookupDaemonInfoClient(enc, dec); ierr == nil {
			domain, port, daemonOK = d, p, true
			if !https {
				scheme = "http"
			}
		}
		if enc.Encode(&sockproto.Message{Op: sockproto.OpList}) == nil {
			if resp, lerr := dec.Decode(); lerr == nil && resp.Ok {
				entries = resp.Entries
			}
		}
	}

	var items []shareItem
	if opts.All {
		if !daemonOK {
			return exitError{code: 1, text: "hostmux share --all: daemon not reachable; start it with `hostmux start`"}
		}
		for _, e := range entries {
			for _, h := range e.Hosts {
				items = append(items, shareItem{
					Host:       h,
					URL:        formatPublicURL(h, scheme, port),
					Registered: true,
					Upstream:   e.Upstream,
					Source:     e.Source,
				})
			}
		}
	} else {
		if err := validateExplicitNames(opts.Names); err != nil {
			return exitError{code: 2, text: fmt.Sprintf("hostmux share: %v", err)}
		}
		names, nerr := resolveRequestedNames(opts.Names)
		if nerr != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux share: %v", nerr)}
		}
		domainForExpand := opts.Domain
		if domainForExpand == "" {
			domainForExpand = domain
		}
		hosts, herr := resolveRequestedHosts(names, hostResolveOptions{
			Domain:   domainForExpand,
			Prefix:   opts.Prefix,
			NoPrefix: opts.NoPrefix,
		})
		if herr != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux share: %v", herr)}
		}
		byHost := indexEntriesByHost(entries)
		for _, h := range hosts {
			it := shareItem{Host: h, URL: formatPublicURL(h, scheme, port)}
			if e, ok := byHost[h]; ok {
				it.Registered = true
				it.Upstream = e.Upstream
				it.Source = e.Source
			}
			items = append(items, it)
		}
		if hostnames.HasBare(hosts) && !daemonOK && opts.Domain == "" {
			fmt.Fprintln(os.Stderr, "hostmux share: daemon unreachable and no --domain; some names may be unexpanded")
		}
	}

	if opts.JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	for i, it := range items {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, it.Host)
		fmt.Fprintf(w, "  url:      %s\n", it.URL)
		if it.Registered {
			fmt.Fprintf(w, "  upstream: %s (%s)\n", it.Upstream, it.Source)
		} else {
			fmt.Fprintln(w, "  upstream: not currently registered")
		}
		if opts.QR {
			qr, qerr := renderQR(it.URL)
			if qerr != nil {
				fmt.Fprintf(os.Stderr, "hostmux share: qr: %v\n", qerr)
			} else {
				fmt.Fprint(w, qr)
			}
		}
	}
	return nil
}

func indexEntriesByHost(entries []sockproto.Entry) map[string]sockproto.Entry {
	m := make(map[string]sockproto.Entry)
	for _, e := range entries {
		for _, h := range e.Hosts {
			m[h] = e
		}
	}
	return m
}

// renderQR returns a compact terminal QR code for the given content.
func renderQR(content string) (string, error) {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	return q.ToSmallString(false), nil
}

// isTerminal reports whether f is an interactive terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
