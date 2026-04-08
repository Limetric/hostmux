package router

import (
	"fmt"
	"slices"
	"sort"
	"sync"
)

// Entry is a snapshot view of one registration in the routing table.
// It is used by Snapshot and ReplaceSource. The Source field is informational
// for Snapshot output and ignored by ReplaceSource (which uses its source
// argument).
type Entry struct {
	Source   string
	Hosts    []string
	Upstream string
}

// Router holds the host → upstream mapping. All mutating operations are
// atomic and safe under concurrent access.
type Router struct {
	mu       sync.RWMutex
	byHost   map[string]*entry
	bySource map[string][]*entry
}

type entry struct {
	source   string
	hosts    []string
	upstream string
}

// New returns a fresh empty router.
func New() *Router {
	return &Router{
		byHost:   make(map[string]*entry),
		bySource: make(map[string][]*entry),
	}
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
	if source == "" {
		return fmt.Errorf("router: source must be non-empty")
	}
	if len(hosts) == 0 {
		return fmt.Errorf("router: hosts must be non-empty")
	}
	if upstream == "" {
		return fmt.Errorf("router: upstream must be non-empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range hosts {
		if existing, ok := r.byHost[h]; ok && existing.source != source {
			return fmt.Errorf("router: host %q already registered by %q", h, existing.source)
		}
	}
	// Drop any existing same-source entries that contain these hosts so we
	// can rebuild with the new upstream cleanly.
	for _, h := range hosts {
		if existing, ok := r.byHost[h]; ok && existing.source == source {
			r.removeEntryLocked(existing)
		}
	}
	e := &entry{source: source, hosts: append([]string(nil), hosts...), upstream: upstream}
	for _, h := range hosts {
		r.byHost[h] = e
	}
	r.bySource[source] = append(r.bySource[source], e)
	return nil
}

// RemoveBySource removes every entry owned by the given source.
func (r *Router) RemoveBySource(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	for _, ne := range newEntries {
		e := &entry{source: source, hosts: append([]string(nil), ne.Hosts...), upstream: ne.Upstream}
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
				Source:   source,
				Hosts:    append([]string(nil), e.hosts...),
				Upstream: e.upstream,
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
