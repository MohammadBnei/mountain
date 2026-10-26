package main

// WebSocket hub: the live overlay and graph notices, out to admin sessions only (adminOnly gates the upgrade; the
// socket closes when its session lapses). Clients send nothing: anything they write is read and dropped, which is
// also what lets Ping return. coder/websocket's own origin check ignores the scheme and passes an empty Origin, so
// originOK runs first and Accept gets InsecureSkipVerify.

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	pingEvery   = 30 * time.Second
	perIPMax    = 8
	maxFrame    = 4096
	sendBuffer  = 64
	origin      = "https://mountain.bnei.dev"
	writeTimout = 10 * time.Second
)

// Client is one /ws connection.
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

func (c *Client) Send(b []byte) bool {
	select {
	case c.send <- b:
		return true
	default:
		return false
	}
}

// Hub fans frames out to every client. A client whose buffer is full is dropped; it resyncs through the next hello.
type Hub struct {
	dev   bool
	hello func() []byte

	mu      sync.Mutex
	clients map[*Client]struct{}
	perIP   map[string]int
}

func NewHub(dev bool, hello func() []byte) *Hub {
	return &Hub{dev: dev, hello: hello, clients: map[*Client]struct{}{}, perIP: map[string]int{}}
}

// Broadcast queues b to every client (nil = nothing).
func (h *Hub) Broadcast(b []byte) {
	if b == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if !c.Send(b) {
			c.conn.CloseNow() // never block the hub lock on a close handshake
		}
	}
}

// originOK: exact scheme + host; empty rejects; --dev also allows http://127.0.0.1:<port>.
func originOK(o string, dev bool) bool {
	if o == origin {
		return true
	}
	if !dev {
		return false
	}
	u, err := url.Parse(o)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path != "" || u.User != nil {
		return false
	}
	_, err = strconv.ParseUint(u.Port(), 10, 16)
	return err == nil
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// the machine principal (machine.go) carries no cookie a page could ride: no browser origin to check, and its
	// socket ends with its token
	mexp, isMachine := machineExp(r.Context())
	if !isMachine && !originOK(r.Header.Get("Origin"), h.dev) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	h.mu.Lock()
	if h.perIP[ip] >= perIPMax {
		h.mu.Unlock()
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}
	h.perIP[ip]++
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.perIP[ip]--; h.perIP[ip] <= 0 {
			delete(h.perIP, ip)
		}
		h.mu.Unlock()
	}()

	_, exp := sessionOf(r)
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true}) // origin checked above
	if err != nil {
		return
	}
	conn.SetReadLimit(maxFrame)
	c := &Client{conn: conn, send: make(chan []byte, sendBuffer)}
	// hello + register under the broadcast lock: every frame lands either in the hello or after it.
	h.mu.Lock()
	c.send <- h.hello()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	WSConnections.Add("", 1)
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		WSConnections.Add("", -1)
		conn.CloseNow()
	}()

	end := exp // the session's end is the socket's end: a lost group stops the overlay within the TTL
	if isMachine {
		end = mexp // the token's: a deactivated account loses the feed within the token's 10 minutes
	} else if end.IsZero() {
		end = time.Now().Add(100 * 365 * 24 * time.Hour) // devOpen: no session
	}
	ctx, cancel := context.WithDeadline(r.Context(), end)
	defer cancel()
	go func() { // reader: drops what it reads; also what lets Ping return
		defer cancel()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	tick := time.NewTicker(pingEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusPolicyViolation, "session ended")
			return
		case b := <-c.send:
			wctx, wc := context.WithTimeout(ctx, writeTimout)
			err := conn.Write(wctx, websocket.MessageText, b)
			wc()
			if err != nil {
				return
			}
		case <-tick.C:
			pctx, pc := context.WithTimeout(ctx, writeTimout)
			err := conn.Ping(pctx)
			pc()
			if err != nil {
				return
			}
		}
	}
}

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

// clientIP keys the per-IP limit. CF-Connecting-IP is trusted only when the TCP peer is a Cloudflare address. When
// the TCP peer is private (Traefik in the pod network), the peer Traefik saw is the last X-Forwarded-For hop.
func clientIP(r *http.Request) string {
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
