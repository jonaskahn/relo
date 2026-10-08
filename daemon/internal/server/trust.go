// Network trust: loopback, private, and forwarded-client checks.
package server

import (
	"net"
	"net/http"
	"strings"
)

func (s *Server) consoleLocal(r *http.Request) bool {
	peer := remoteIP(r.RemoteAddr)
	if !isLocalAddress(peer) {
		return false
	}
	if forwarded := forwardedClientIP(r); forwarded != nil && !isLocalAddress(forwarded) {
		return false
	}
	if peer.IsLoopback() {
		return isLocalHost(r.Host)
	}
	// A peer from the private network reaches this machine by one of its
	// addresses; a loopback or localhost Host does not describe how it got
	// here, so only a private address counts as local.
	return isPrivateHost(r.Host)
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}

func isLocalAddress(address net.IP) bool {
	if address == nil || address.IsUnspecified() {
		return false
	}
	return address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast()
}

func isPrivateHost(hostPort string) bool {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	address := net.ParseIP(strings.Trim(strings.TrimSpace(host), "[]"))
	return address != nil && address.IsPrivate()
}

func forwardedClientIP(r *http.Request) net.IP {
	if raw := r.Header.Get("X-Forwarded-For"); raw != "" {
		first, _, _ := strings.Cut(raw, ",")
		if address := net.ParseIP(strings.TrimSpace(first)); address != nil {
			return address
		}
	}
	if raw := strings.TrimSpace(r.Header.Get("X-Real-Ip")); raw != "" {
		if address := net.ParseIP(raw); address != nil {
			return address
		}
	}
	return nil
}
