package proxy

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// otherHost groups every host a picker connection asked for that Relo does not
// intercept, so the counters hold no list of where the app has been.
const otherHost = "other"

// maxLoggedRefusals bounds how many refused peers are logged one by one.
const maxLoggedRefusals = 5

// HostCount is how many connections one host has asked the picker proxy for,
// and when the last one arrived.
type HostCount struct {
	Host       string
	Count      int
	LastUnixMs int64
}

// Connections is what the picker proxy has been asked, as counts only: no
// header, path or credential is ever held.
type Connections struct {
	Hosts []HostCount
	// RefusedPeers counts connections from an address that is not loopback,
	// which the proxy drops before it reads them.
	RefusedPeers int
}

type connectionStats struct {
	mu      sync.Mutex
	hosts   map[string]*HostCount
	refused int
}

// watched reports whether Relo intercepts the host, which is the only kind a
// count or a log line names.
func watched(host string) bool {
	return slices.Contains(InterceptHosts, strings.ToLower(host))
}

// record counts one connection for a host and answers the label it was counted
// under and the new total.
func (s *connectionStats) record(host string, now time.Time) (string, int) {
	label := otherHost
	if watched(host) {
		label = strings.ToLower(host)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hosts == nil {
		s.hosts = map[string]*HostCount{}
	}
	entry := s.hosts[label]
	if entry == nil {
		entry = &HostCount{Host: label}
		s.hosts[label] = entry
	}
	entry.Count++
	entry.LastUnixMs = now.UnixMilli()
	return label, entry.Count
}

// refusePeer counts one connection from a non-loopback address.
func (s *connectionStats) refusePeer() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refused++
	return s.refused
}

func (s *connectionStats) snapshot() Connections {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Connections{RefusedPeers: s.refused, Hosts: make([]HostCount, 0, len(s.hosts))}
	for _, entry := range s.hosts {
		out.Hosts = append(out.Hosts, *entry)
	}
	slices.SortFunc(out.Hosts, func(a, b HostCount) int { return strings.Compare(a.Host, b.Host) })
	return out
}
