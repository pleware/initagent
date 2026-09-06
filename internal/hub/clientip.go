package hub

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// parseTrustedProxies turns a comma-separated list of CIDRs or single
// addresses into prefixes. Empty input is no trusted hop: X-Forwarded-For
// is ignored and the bucket key is the connection address (`26`).
func parseTrustedProxies(spec string) ([]netip.Prefix, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var out []netip.Prefix
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		prefix, err := parseProxyPrefix(part)
		if err != nil {
			return nil, err
		}
		out = append(out, prefix)
	}
	return out, nil
}

func parseProxyPrefix(part string) (netip.Prefix, error) {
	if strings.Contains(part, "/") {
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("trusted proxy %q: %w", part, err)
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(part)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("trusted proxy %q: %w", part, err)
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// clientAddr is the rate-limit key for a request. A header is only read when
// the TCP peer is a configured hop; trusting X-Forwarded-For from the
// internet lets an attacker mint buckets (`26`).
func (s *Server) clientAddr(r *http.Request) string {
	return clientKey(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), s.trusted)
}

func clientKey(remoteAddr, xff string, trusted []netip.Prefix) string {
	peer, ok := ipFromRemote(remoteAddr)
	if !ok {
		if remoteAddr == "" {
			return ""
		}
		return remoteAddr
	}
	if len(trusted) == 0 || !isTrusted(peer, trusted) {
		return peer.String()
	}
	hops := parseForwardedFor(xff)
	for i := len(hops) - 1; i >= 0; i-- {
		if !isTrusted(hops[i], trusted) {
			return hops[i].String()
		}
	}
	return peer.String()
}

func ipFromRemote(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func parseForwardedFor(xff string) []netip.Addr {
	if strings.TrimSpace(xff) == "" {
		return nil
	}
	var hops []netip.Addr
	for _, part := range strings.Split(xff, ",") {
		addr, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		hops = append(hops, addr.Unmap())
	}
	return hops
}

func isTrusted(ip netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
