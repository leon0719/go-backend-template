package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

type clientIPCtxKey struct{}

// ParseTrustedProxies converts CIDR strings (e.g. "172.16.0.0/12") into
// prefixes for RealIP. A bare IP ("10.1.2.3") is accepted and treated as a
// single-host prefix. An empty slice means "trust nothing".
func ParseTrustedProxies(cidrs []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, raw := range cidrs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			prefixes = append(prefixes, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q is neither a CIDR nor an IP: %w", raw, err)
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}

// RealIP resolves the client IP and stores it in the request context.
//
// SECURITY CONTRACT: X-Forwarded-For is attacker-controlled. It is consulted
// ONLY when the direct peer (r.RemoteAddr) is inside one of trustedProxies.
// From an untrusted peer the header is ignored entirely, so a client cannot
// spoof its way past IP rate limiting. When the peer IS trusted, the client
// IP is the RIGHTMOST address in X-Forwarded-For that is not itself a trusted
// proxy — the rightmost entries are the ones appended by infrastructure you
// control, while anything further left may have been forged by the client.
//
// With an empty trustedProxies list the middleware degrades to RemoteAddr,
// which is the correct behavior when the server is directly internet-facing.
// If you deploy behind the bundled Caddy service (or any reverse proxy), set
// TRUSTED_PROXIES or every request will appear to come from the proxy and all
// clients will share a single rate-limit bucket.
func RealIP(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), clientIPCtxKey{}, resolveClientIP(r, trustedProxies))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveClientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	peer := remoteAddrHost(r)
	if len(trustedProxies) == 0 {
		return peer
	}

	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !isTrusted(peerAddr, trustedProxies) {
		return peer
	}

	// Peer is a trusted proxy: walk X-Forwarded-For right-to-left and take
	// the first hop that is not itself a trusted proxy.
	hops := forwardedHops(r)
	for _, hop := range slices.Backward(hops) {
		addr, err := netip.ParseAddr(hop)
		if err != nil {
			continue
		}
		if !isTrusted(addr, trustedProxies) {
			return addr.String()
		}
	}
	// Every hop was a trusted proxy (or the header was absent/garbage).
	return peer
}

func forwardedHops(r *http.Request) []string {
	var hops []string
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for part := range strings.SplitSeq(header, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			// Tolerate "ip:port" and "[v6]:port" forms.
			if host, _, err := net.SplitHostPort(part); err == nil {
				part = host
			}
			hops = append(hops, part)
		}
	}
	return hops
}

func isTrusted(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, p := range trustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func remoteAddrHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ClientIPFromContext returns the IP resolved by RealIP, or "" if the
// middleware was not mounted.
func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPCtxKey{}).(string)
	return ip
}

// ClientIP returns the RealIP-resolved client IP, falling back to the direct
// peer address when the RealIP middleware is not mounted (e.g. in handler
// unit tests that build a bare router).
func ClientIP(r *http.Request) string {
	if ip := ClientIPFromContext(r.Context()); ip != "" {
		return ip
	}
	return remoteAddrHost(r)
}
