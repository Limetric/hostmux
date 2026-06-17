package router

import (
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"
)

// Entry is a snapshot view of one registration in the routing table.
// It is used by Snapshot and ReplaceSource. The Source field is informational
// for Snapshot output and ignored by ReplaceSource (which uses its source
// argument).
type Entry struct {
	Source   string
	Hosts    []string
	Upstream string

	// Optional route metadata. Carried through Snapshot for richer route
	// visibility (hostmux routes). RegisteredAt is stamped by the router on
	// insert and ignored when supplied as input.
	Labels       map[string]string
	PID          int
	Command      string
	Cwd          string
	RegisteredAt time.Time
}

// Router holds the host → upstream mapping. All mutating operations are
// atomic and safe under concurrent access.
type Router struct {
	mu       sync.RWMutex
	byHost   map[string]*entry
	bySource map[string][]*entry
	now      func() time.Time
}

type entry struct {
	source       string
	hosts        []string
	upstream     string
	labels       map[string]string
	pid          int
	command      string
	cwd          string
	registeredAt time.Time
}

// New returns a fresh empty router.
func New() *Router {
	return &Router{
		byHost:   make(map[string]*entry),
		bySource: make(map[string][]*entry),
		now:      time.Now,
	}
}

// Count returns the total number of registered hostnames across all sources.
func (r *Router) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byHost)
}

// Lookup returns the upstream URL for a host, or false if not registered.
func (r *Router) Lookup(host string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byHost[host]
	if !ok {
		return "", false
	}
	return e.upstream, true
}

// Add registers all hosts under the given source pointing at upstream.
// All-or-nothing: if any host is currently owned by a different source, no
// host is registered and an error is returned naming the colliding host.
// Hosts already owned by the same source are silently refreshed to the new
// upstream.
func (r *Router) Add(source string, hosts []string, upstream string) error {
	return r.AddEntry(Entry{Source: source, Hosts: hosts, Upstream: upstream})
}

// AddEntry is Add with optional route metadata (labels, PID, command, cwd).
// RegisteredAt on the input is ignored; the router stamps it at insert time.
func (r *Router) AddEntry(in Entry) error {
	if in.Source == "" {
		return fmt.Errorf("router: source must be non-empty")
	}
	if len(in.Hosts) == 0 {
		return fmt.Errorf("router: hosts must be non-empty")
	}
	if in.Upstream == "" {
		return fmt.Errorf("router: upstream must be non-empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range in.Hosts {
		if existing, ok := r.byHost[h]; ok && existing.source != in.Source {
			return fmt.Errorf("router: host %q already registered by %q", h, existing.source)
		}
	}
	// Drop any existing same-source entries that contain these hosts so we
	// can rebuild with the new upstream cleanly.
	for _, h := range in.Hosts {
		if existing, ok := r.byHost[h]; ok && existing.source == in.Source {
			r.removeEntryLocked(existing)
		}
	}
	e := &entry{
		source:       in.Source,
		hosts:        append([]string(nil), in.Hosts...),
		upstream:     in.Upstream,
		labels:       copyLabels(in.Labels),
		pid:          in.PID,
		command:      in.Command,
		cwd:          in.Cwd,
		registeredAt: r.now(),
	}
	for _, h := range in.Hosts {
		r.byHost[h] = e
	}
	r.bySource[in.Source] = append(r.bySource[in.Source], e)
	return nil
}

func copyLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// RemoveBySource removes every entry owned by the given source.
func (r *Router) RemoveBySource(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeSourceLocked(source)
}

// RemoveSource removes every entry owned by source and reports whether the
// source had any entries to begin with. Used by manual unexpose so the CLI
// can distinguish "removed" from "nothing was registered under that name".
func (r *Router) RemoveSource(source string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, existed := r.bySource[source]
	r.removeSourceLocked(source)
	return existed
}

func (r *Router) removeSourceLocked(source string) {
	for _, e := range r.bySource[source] {
		for _, h := range e.hosts {
			if r.byHost[h] == e {
				delete(r.byHost, h)
			}
		}
	}
	delete(r.bySource, source)
}

// ReplaceSource atomically replaces every entry owned by source with the
// supplied set. Validates that the new set has no internal duplicates and
// no collision with any other source. On error the live state is untouched.
func (r *Router) ReplaceSource(source string, newEntries []Entry) error {
	if source == "" {
		return fmt.Errorf("router: source must be non-empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[string]bool)
	for _, ne := range newEntries {
		if len(ne.Hosts) == 0 {
			return fmt.Errorf("router: entry with empty hosts")
		}
		if ne.Upstream == "" {
			return fmt.Errorf("router: entry with empty upstream")
		}
		for _, h := range ne.Hosts {
			if seen[h] {
				return fmt.Errorf("router: duplicate host %q in new entries", h)
			}
			seen[h] = true
		}
	}
	for h := range seen {
		if existing, ok := r.byHost[h]; ok && existing.source != source {
			return fmt.Errorf("router: host %q already registered by %q", h, existing.source)
		}
	}

	// Commit: drop old entries for this source, install new.
	for _, e := range r.bySource[source] {
		for _, h := range e.hosts {
			if r.byHost[h] == e {
				delete(r.byHost, h)
			}
		}
	}
	delete(r.bySource, source)
	now := r.now()
	for _, ne := range newEntries {
		e := &entry{
			source:       source,
			hosts:        append([]string(nil), ne.Hosts...),
			upstream:     ne.Upstream,
			labels:       copyLabels(ne.Labels),
			pid:          ne.PID,
			command:      ne.Command,
			cwd:          ne.Cwd,
			registeredAt: now,
		}
		for _, h := range ne.Hosts {
			r.byHost[h] = e
		}
		r.bySource[source] = append(r.bySource[source], e)
	}
	return nil
}

// Snapshot returns a copy of all entries, sorted by source then first host.
func (r *Router) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Entry
	for source, entries := range r.bySource {
		for _, e := range entries {
			out = append(out, Entry{
				Source:       source,
				Hosts:        append([]string(nil), e.hosts...),
				Upstream:     e.upstream,
				Labels:       copyLabels(e.labels),
				PID:          e.pid,
				Command:      e.command,
				Cwd:          e.cwd,
				RegisteredAt: e.registeredAt,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if len(out[i].Hosts) > 0 && len(out[j].Hosts) > 0 {
			return out[i].Hosts[0] < out[j].Hosts[0]
		}
		return false
	})
	return out
}

// removeEntryLocked drops a single entry. Caller must hold the write lock.
func (r *Router) removeEntryLocked(e *entry) {
	for _, h := range e.hosts {
		if r.byHost[h] == e {
			delete(r.byHost, h)
		}
	}
	src := r.bySource[e.source]
	for i, x := range src {
		if x == e {
			r.bySource[e.source] = slices.Delete(src, i, i+1)
			break
		}
	}
}
