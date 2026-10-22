package main

// Lane A plug-ins to the seam hub/mux: the client-IP rule for the per-IP /ws limit.
// Client -> server frames: scrub.go sets onClientMsg (scrub, W2); subscribe and mark arrive later.

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// cloudflare: https://www.cloudflare.com/ips-v4 and ips-v6.
var cloudflare = func() (p []netip.Prefix) {
	for _, s := range strings.Fields(`173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22 141.101.64.0/18
		108.162.192.0/18 190.93.240.0/20 188.114.96.0/20 197.234.240.0/22 198.41.128.0/17 162.158.0.0/15 104.16.0.0/13
		104.24.0.0/14 172.64.0.0/13 131.0.72.0/22 2400:cb00::/32 2606:4700::/32 2803:f800::/32 2405:b500::/32
		2405:8100::/32 2a06:98c0::/29 2c0f:f248::/32`) {
		p = append(p, netip.MustParsePrefix(s))
	}
	return
}()

func inCloudflare(a netip.Addr) bool {
	for _, p := range cloudflare {
		if p.Contains(a.Unmap()) {
			return true
		}
	}
	return false
}

// cfClientIP: CF-Connecting-IP is trusted only when the TCP peer is a Cloudflare address (06).
// When the TCP peer is private (Traefik in the pod network), the peer Traefik saw is the last
// X-Forwarded-For hop; LAN/pod-CIDR spoofing of that hop is accepted (06).
func cfClientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	if peer.IsPrivate() || peer.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			hops := strings.Split(xff, ",")
			if a, err := netip.ParseAddr(strings.TrimSpace(hops[len(hops)-1])); err == nil {
				peer = a
			}
		}
	}
	if inCloudflare(peer) {
		if a, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); err == nil {
			return a.String()
		}
	}
	return peer.String()
}

func init() {
	clientIP = cfClientIP
}
