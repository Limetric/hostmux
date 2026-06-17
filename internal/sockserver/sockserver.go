// Package sockserver implements the hostmux daemon's Unix socket listener.
// Clients (hostmux run, hostmux list) connect, exchange newline-delimited
// JSON messages defined in internal/sockproto, and receive routing-table
// mutations or queries. Each connection's registrations are scoped to the
// lifetime of the TCP connection: an EOF or error triggers automatic
// removal via router.RemoveBySource, so a SIGKILL'd registrar produces no
// stale state.
package sockserver

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Limetric/hostmux/internal/router"
	"github.com/Limetric/hostmux/internal/sockproto"
)

// Options configures a Server on construction.
type Options struct {
	// OnShutdown, if non-nil, is invoked in its own goroutine after the
	// server replies to an OpShutdown message. The daemon wires this to
	// its main-loop context cancel, triggering the same teardown path as
	// a SIGTERM.
	OnShutdown func()

	// Domain returns the daemon's configured base domain, if any.
	Domain func() string

	// PlainHTTP is true when the daemon's public edge uses only plain HTTP
	// (no TLS listener). When false, clients should use https:// for public
	// URLs such as HOSTMUX_URL.
	PlainHTTP bool

	// PublicPort is the effective public TCP port the daemon's edge listens
	// on. It is reported via OpInfo so clients can build accurate URLs.
	// Zero means "unspecified"; clients should fall back to scheme default.
	PublicPort int
}

// Server is the daemon-side Unix socket server. Each connection owns the
// hostnames it registered; on disconnect the daemon removes them.
type Server struct {
	router     *router.Router
	onShutdown func()
	domain     func() string
	plainHTTP  bool
	publicPort int

	mu     sync.Mutex
	ln     net.Listener
	closed bool
	connID atomic.Uint64
}

// New returns a Server bound to the given router and options.
func New(r *router.Router, opts Options) *Server {
	return &Server{
		router:     r,
		onShutdown: opts.OnShutdown,
		domain:     opts.Domain,
		plainHTTP:  opts.PlainHTTP,
		publicPort: opts.PublicPort,
	}
}

// Listen creates the Unix socket at path. Removes any stale socket file first.
func (s *Server) Listen(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sockserver: cleanup stale socket: %w", err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("sockserver: listen %s: %w", path, err)
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	return nil
}

// Serve accepts connections until Close is called.
func (s *Server) Serve() {
	for {
		s.mu.Lock()
		ln := s.ln
		s.mu.Unlock()
		if ln == nil {
			return
		}
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			continue
		}
		go s.serveConn(conn)
	}
}

// Close stops the listener and unblocks Serve.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.ln != nil {
		err := s.ln.Close()
		s.ln = nil
		return err
	}
	return nil
}

// ManualSourcePrefix namespaces manually-exposed routes so they can never
// collide with or masquerade as connection-scoped ("socket:N") or config
// ("config") sources. The client supplies only the bare name.
const ManualSourcePrefix = "manual:"

// ManualSource returns the router source name for a manually-exposed route.
func ManualSource(name string) string { return ManualSourcePrefix + name }

// validManualName reports whether a manual-route name is a safe token: a
// non-empty string of letters, digits, dash, dot, or underscore with no
// colon (which would let a client forge an arbitrary source).
func validManualName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '.' || r == '_':
		default:
			return false
		}
	}
	return true
}

// handleExpose registers a connection-independent route owned by a
// caller-named source. The route persists until OpUnexpose or daemon
// restart, unlike OpRegister routes which are torn down on disconnect.
func (s *Server) handleExpose(msg *sockproto.Message) error {
	if !validManualName(msg.Source) {
		return fmt.Errorf("invalid route name %q: use letters, digits, '-', '.', '_'", msg.Source)
	}
	return s.router.AddEntry(router.Entry{
		Source:   ManualSource(msg.Source),
		Hosts:    msg.Hosts,
		Upstream: msg.Upstream,
		Labels:   msg.Labels,
		PID:      msg.PID,
		Command:  msg.Command,
		Cwd:      msg.Cwd,
	})
}

// handleUnexpose removes a manually-exposed route by name.
func (s *Server) handleUnexpose(msg *sockproto.Message) error {
	if !validManualName(msg.Source) {
		return fmt.Errorf("invalid route name %q", msg.Source)
	}
	if !s.router.RemoveSource(ManualSource(msg.Source)) {
		return fmt.Errorf("no exposed route named %q", msg.Source)
	}
	return nil
}

func (s *Server) serveConn(c net.Conn) {
	id := s.connID.Add(1)
	source := fmt.Sprintf("socket:%d", id)
	defer func() {
		s.router.RemoveBySource(source)
		c.Close()
	}()
	dec := sockproto.NewDecoder(c)
	enc := sockproto.NewEncoder(c)
	for {
		msg, err := dec.Decode()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			_ = enc.Encode(&sockproto.Message{Ok: false, Error: err.Error()})
			return
		}
		switch msg.Op {
		case sockproto.OpRegister:
			if err := s.router.AddEntry(router.Entry{
				Source:   source,
				Hosts:    msg.Hosts,
				Upstream: msg.Upstream,
				Labels:   msg.Labels,
				PID:      msg.PID,
				Command:  msg.Command,
				Cwd:      msg.Cwd,
			}); err != nil {
				_ = enc.Encode(&sockproto.Message{Ok: false, Error: err.Error()})
				continue
			}
			_ = enc.Encode(&sockproto.Message{Ok: true})
		case sockproto.OpExpose:
			if err := s.handleExpose(msg); err != nil {
				_ = enc.Encode(&sockproto.Message{Ok: false, Error: err.Error()})
				continue
			}
			_ = enc.Encode(&sockproto.Message{Ok: true})
		case sockproto.OpUnexpose:
			if err := s.handleUnexpose(msg); err != nil {
				_ = enc.Encode(&sockproto.Message{Ok: false, Error: err.Error()})
				continue
			}
			_ = enc.Encode(&sockproto.Message{Ok: true})
		case sockproto.OpList:
			snap := s.router.Snapshot()
			out := make([]sockproto.Entry, 0, len(snap))
			for _, e := range snap {
				wire := sockproto.Entry{
					Source:   e.Source,
					Hosts:    e.Hosts,
					Upstream: e.Upstream,
					Labels:   e.Labels,
					PID:      e.PID,
					Command:  e.Command,
					Cwd:      e.Cwd,
				}
				if !e.RegisteredAt.IsZero() {
					wire.RegisteredAt = e.RegisteredAt.Unix()
				}
				out = append(out, wire)
			}
			_ = enc.Encode(&sockproto.Message{Ok: true, Entries: out})
		case sockproto.OpInfo:
			domain := ""
			if s.domain != nil {
				domain = s.domain()
			}
			publicHTTPS := !s.plainHTTP
			_ = enc.Encode(&sockproto.Message{
				Ok:          true,
				Domain:      domain,
				PublicHTTPS: &publicHTTPS,
				PublicPort:  s.publicPort,
			})
		case sockproto.OpBye:
			_ = enc.Encode(&sockproto.Message{Ok: true})
			return
		case sockproto.OpShutdown:
			_ = enc.Encode(&sockproto.Message{Ok: true})
			// Fire the callback in its own goroutine so this handler
			// returns before the daemon starts tearing down the listener.
			// Otherwise the sockserver Close could block waiting for this
			// very handler to finish.
			if s.onShutdown != nil {
				go s.onShutdown()
			}
			return
		default:
			_ = enc.Encode(&sockproto.Message{Ok: false, Error: fmt.Sprintf("unknown op %q", msg.Op)})
		}
	}
}
