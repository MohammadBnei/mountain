package main

// Sign-in (plan/10 Security, R10 Audience row: admins only). One authentik OIDC app `mountain`, authorization code
// with PKCE (S256) + random state + nonce. No token reaches the client and none is kept: the callback verifies the
// ID token (iss, aud, signature, nonce) and, for a member of platform-admins only, sets a signed session cookie
// {v: {sub, name}, exp}, TTL 1 h; the group is re-checked at the next sign-in. Anyone else gets no session and a
// page saying why. The return view rides a 10 min signed flow cookie.
//
// Env (Secret mountain-oidc): MOUNTAIN_OIDC_ISSUER, MOUNTAIN_OIDC_CLIENT_ID, MOUNTAIN_OIDC_CLIENT_SECRET,
// MOUNTAIN_SESSION_KEY (HMAC key, ≥ 32 bytes). Any unset = off: every admin route refuses (main.go adminOnly),
// except under --dev on a loopback address (devOpen), which is how the page is worked on locally.
//
//	GET  /auth/login?return=/#view  → authentik
//	GET  /auth/callback             → session cookie, 303 to the return view
//	POST /auth/logout               (same Origin only: CSRF) → cookie cleared, 303 to /auth/me
//	GET  /auth/me                   → {"enabled": bool, "admin": bool, "name"?: the admin's username}

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	adminGroup    = "platform-admins"
	sessionCookie = "__Host-mountain" // __Host-: Secure, Path=/, no Domain
	flowCookie    = "__Host-mountain-flow"
	sessionTTL    = time.Hour
	flowTTL       = 10 * time.Minute
)

// returnRE: the return view is the page itself plus a query or fragment (an atlas route), else "/": no other path,
// so no "//host", backslash or scheme trick can make it an open redirect.
var returnRE = regexp.MustCompile(`^/(?:[?#][A-Za-z0-9=&?_.:/@-]{0,160})?$`)

// Auth is the OIDC client; nil = sign-in off.
type Auth struct {
	issuer string
	oauth  oauth2.Config // Endpoint filled by discovery
	key    []byte
	dev    bool

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier // nil until the first successful discovery
}

var (
	auth    *Auth // set by serve (authFromEnv) before routes run
	devOpen bool  // --dev on loopback with sign-in unset: everyone is an admin (local work only)
)

func authFromEnv(dev bool) *Auth {
	iss, id, sec, key := os.Getenv("MOUNTAIN_OIDC_ISSUER"), os.Getenv("MOUNTAIN_OIDC_CLIENT_ID"), os.Getenv("MOUNTAIN_OIDC_CLIENT_SECRET"), os.Getenv("MOUNTAIN_SESSION_KEY")
	if iss == "" || id == "" || sec == "" || len(key) < 32 {
		return nil
	}
	return newAuth(iss, id, sec, origin+"/auth/callback", []byte(key), dev)
}

func newAuth(issuer, id, secret, redirect string, key []byte, dev bool) *Auth {
	return &Auth{issuer: issuer, key: key, dev: dev, oauth: oauth2.Config{ClientID: id, ClientSecret: secret, RedirectURL: redirect,
		Scopes: []string{oidc.ScopeOpenID, "profile"}}} // authentik's profile scope carries groups, in the ID token
}

// discover runs OIDC discovery once it succeeds (lazily: authentik down at boot must not stop atlas).
func (au *Auth) discover() (*oidc.IDTokenVerifier, error) {
	au.mu.Lock()
	defer au.mu.Unlock()
	if au.verifier == nil {
		// Background, not the request: go-oidc keeps this context for later JWKS refreshes.
		p, err := oidc.NewProvider(context.Background(), au.issuer)
		if err != nil {
			return nil, err
		}
		au.oauth.Endpoint = p.Endpoint()
		au.verifier = p.Verifier(&oidc.Config{ClientID: au.oauth.ClientID}) // checks iss + aud: authentik shares one signing key across apps
	}
	return au.verifier, nil
}

// flow is the login round trip's state, in the flow cookie. Signed, not encrypted: it only ever goes back to the
// browser that started the flow, and the verifier is useless without the code it is bound to.
type flow struct{ State, Nonce, Verifier, Return string }

// set writes a signed cookie: base64url(json{v, exp}) "." base64url(HMAC(key, name \0 json)). The name is in the
// MAC, so a flow cookie can never pass as a session.
func (au *Auth) set(w http.ResponseWriter, name string, v any, ttl time.Duration) {
	b := mustJSON(map[string]any{"v": v, "exp": time.Now().Add(ttl).Unix()})
	http.SetCookie(w, &http.Cookie{Name: name, Value: base64.RawURLEncoding.EncodeToString(b) + "." + au.mac(name, b),
		Path: "/", MaxAge: int(ttl.Seconds()), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (au *Auth) mac(name string, b []byte) string {
	m := hmac.New(sha256.New, au.key)
	m.Write([]byte(name + "\x00"))
	m.Write(b)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// get decodes the cookie name into v when its MAC holds and it has not expired; it returns the expiry.
func (au *Auth) get(r *http.Request, name string, v any) (time.Time, bool) {
	c, err := r.Cookie(name)
	if err != nil {
		return time.Time{}, false
	}
	p, sig, _ := strings.Cut(c.Value, ".")
	b, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil || !hmac.Equal([]byte(sig), []byte(au.mac(name, b))) {
		return time.Time{}, false
	}
	var s struct {
		V   json.RawMessage
		Exp int64
	}
	ok := json.Unmarshal(b, &s) == nil && time.Now().Unix() < s.Exp && json.Unmarshal(s.V, v) == nil
	return time.Unix(s.Exp, 0), ok
}

// Session is the session cookie's value. Admin is set only from the groups claim at sign-in; a cookie from the town
// (role, sub, name) decodes with Admin false and is refused until the next sign-in.
type Session struct {
	Admin     bool
	Sub, Name string
}

// sessionOf is r's session ({} = none: no auth, no cookie, a bad MAC or expired) and its expiry: a socket closes
// when it lapses (hub.go), so a lost group ends access within the 1 h TTL.
func sessionOf(r *http.Request) (Session, time.Time) {
	var s Session
	if auth == nil {
		return s, time.Time{}
	}
	if exp, ok := auth.get(r, sessionCookie, &s); ok {
		return s, exp
	}
	return Session{}, time.Time{}
}

// isAdmin: r carries a platform admin's session, or the page runs locally with sign-in off (devOpen).
func isAdmin(r *http.Request) bool {
	if devOpen {
		return true
	}
	s, _ := sessionOf(r)
	return s.Admin && s.Sub != ""
}

// authFail answers a failed or refused sign-in with a page a person can read, not a bare status line.
func authFail(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html><meta name="viewport" content="width=device-width"><title>mountain</title><p>%s</p>`, html.EscapeString(msg))
}

func onAuth(h func(*Auth, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			http.NotFound(w, r)
			return
		}
		h(auth, w, r)
	}
}

func (au *Auth) login(w http.ResponseWriter, r *http.Request) {
	if _, err := au.discover(); err != nil {
		log.Printf("auth: discovery: %v", err)
		authFail(w, "Sign-in is unavailable right now. Reload to try again.", http.StatusServiceUnavailable)
		return
	}
	f := flow{State: rand.Text(), Nonce: rand.Text(), Verifier: oauth2.GenerateVerifier(), Return: r.URL.Query().Get("return")}
	if !returnRE.MatchString(f.Return) {
		f.Return = "/"
	}
	au.set(w, flowCookie, f, flowTTL)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, au.oauth.AuthCodeURL(f.State, oidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier)), http.StatusFound)
}

func (au *Auth) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var f flow
	_, ok := au.get(r, flowCookie, &f)
	clearCookie(w, flowCookie) // one use
	q := r.URL.Query()
	if !ok || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(f.State)) != 1 || q.Get("code") == "" {
		authFail(w, "This sign-in link expired. Reload the page to sign in again.", http.StatusBadRequest)
		return
	}
	v, err := au.discover()
	if err != nil {
		authFail(w, "Sign-in is unavailable right now. Reload to try again.", http.StatusServiceUnavailable)
		return
	}
	tok, err := au.oauth.Exchange(r.Context(), q.Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		log.Printf("auth: exchange: %v", err)
		authFail(w, "Sign-in failed.", http.StatusForbidden)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := v.Verify(r.Context(), raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(f.Nonce)) != 1 {
		log.Printf("auth: id token: %v", err)
		authFail(w, "Sign-in failed.", http.StatusForbidden)
		return
	}
	var c struct {
		Groups []string
		Name   string `json:"preferred_username"`
	}
	idt.Claims(&c) // no groups claim = not an admin (a missing claim yields no access)
	if !slices.Contains(c.Groups, adminGroup) {
		clearCookie(w, sessionCookie)
		authFail(w, "mountain is for platform admins only.", http.StatusForbidden)
		return
	}
	au.set(w, sessionCookie, Session{Admin: true, Sub: idt.Subject, Name: c.Name}, sessionTTL)
	http.Redirect(w, r, f.Return, http.StatusSeeOther)
}

// logout is POST + same Origin (originOK), so another site can neither GET nor form-POST a sign-out. The authentik
// session stays: the page would sign straight back in, so it lands on /auth/me, which says who is signed in.
func (au *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if !originOK(r.Header.Get("Origin"), au.dev) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	clearCookie(w, sessionCookie)
	http.Redirect(w, r, "/auth/me", http.StatusSeeOther)
}

func me(w http.ResponseWriter, r *http.Request) {
	s, _ := sessionOf(r)
	admin := isAdmin(r)
	m := map[string]any{"enabled": auth != nil, "admin": admin}
	if admin && s.Name != "" {
		m["name"] = s.Name
	}
	h := w.Header()
	h.Set("Cache-Control", "private, no-store") // per session: never a shared cache
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Type", "application/json")
	w.Write(mustJSON(m))
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
