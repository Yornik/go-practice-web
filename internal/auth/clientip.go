package auth

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// IPResolver finds the client address of a request behind trusted proxies
// (Traefik in the cluster). X-Forwarded-For is read only when the direct
// peer is a trusted proxy, and from the right: every hop appends, so the
// rightmost address that is not a trusted proxy is the first one a client
// could not forge.
type IPResolver struct {
	trusted []netip.Prefix
}

// ParseTrustedProxies parses GPW_TRUSTED_PROXIES: CIDRs separated by
// commas or spaces. An empty string trusts nobody.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		p, err := netip.ParsePrefix(f)
		if err != nil {
			return nil, fmt.Errorf("auth: trusted proxy %q is not a CIDR: %w", f, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// NewIPResolver trusts X-Forwarded-For from the given networks.
func NewIPResolver(trusted []netip.Prefix) *IPResolver {
	return &IPResolver{trusted: trusted}
}

func (ip *IPResolver) isTrusted(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range ip.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// RateKey returns the key that per-address rate limits count ip under. An
// IPv6 client is usually given a whole /64 and can use any address in it,
// so limits keyed by the full address would give it 2^64 budgets; IPv6
// addresses count per /64. IPv4 addresses, and text that is no address,
// count alone.
func RateKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.Unmap()
	if a.Is4() {
		return a.String()
	}
	p, err := a.WithZone("").Prefix(64)
	if err != nil {
		return a.String()
	}
	return p.String()
}

// ClientIP returns the client address of r as text.
func (ip *IPResolver) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !ip.isTrusted(peer) {
		return peer.String()
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			// A garbled hop ends the trusted chain: what is left of it
			// came from the client.
			break
		}
		if !ip.isTrusted(a) {
			return a.Unmap().String()
		}
		peer = a.Unmap()
	}
	return peer.String()
}
