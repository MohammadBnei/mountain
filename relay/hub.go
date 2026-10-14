package main

// WebSocket hub (06 "WS protocol", "Security", "Keepalive", limits). Orchestrator-owned.
// coder/websocket v1.8.15: its own origin check ignores the scheme and passes an empty Origin,
// so originOK runs first and Accept gets InsecureSkipVerify. Ping needs a concurrent reader.

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	pingEvery   = 30 * time.Second
	perIPMax    = 4
	msgPerSec   = 10
	maxFrame    = 4096
	sendBuffer  = 64
	origin      = "https://mountain.bnei.dev"
	writeTimout = 10 * time.Second
)

// clientIP keys the per-IP limit. Lane A replaces it (from init()) with the
// CF-Connecting-IP-when-peer-is-Cloudflare rule.
var clientIP = func(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// onClientMsg handles client → server frames (subscribe, scrub, mark). Lanes set it from init().
var onClientMsg = func(c *Client, msg []byte) {}

// Client is one /ws connection. Send queues a frame to it alone (e.g. a scrub replay).
type Client struct {
	IP   string
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

// Hub fans frames out to every client. A client whose buffer is full is dropped; it resyncs via hello.
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

// Broadcast queues b to every client (nil = nothing, a dropped frame).
func (h *Hub) Broadcast(b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broadcastLocked(b)
}

// broadcastLocked queues each non-nil frame to every client, in order. Caller holds h.mu.
func (h *Hub) broadcastLocked(bs ...[]byte) {
	for _, b := range bs {
		if b == nil {
			continue
		}
		for c := range h.clients {
			if !c.Send(b) {
				c.conn.CloseNow() // never block the hub lock on a close handshake
			}
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
	if !originOK(r.Header.Get("Origin"), h.dev) {
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

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true}) // origin checked above
	if err != nil {
		return
	}
	conn.SetReadLimit(maxFrame)
	c := &Client{IP: ip, conn: conn, send: make(chan []byte, sendBuffer)}
	// hello + register under the broadcast lock: every frame lands either in the snapshot or after it.
	// Lock order h.mu → app.mu (Hello); nothing may Broadcast while holding app.mu.
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

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() { // reader: rate limit + dispatch; also what lets Ping return
		defer cancel()
		win, n := time.Now(), 0
		for {
			_, msg, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if time.Since(win) > time.Second {
				win, n = time.Now(), 0
			}
			if n++; n > msgPerSec {
				conn.Close(websocket.StatusPolicyViolation, "rate limit")
				return
			}
			onClientMsg(c, msg)
		}
	}()

	tick := time.NewTicker(pingEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
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
