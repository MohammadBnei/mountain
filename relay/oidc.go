package main

// Detail mode (08 "W3 detail mode", 06 "Security"): one authentik OIDC app `mountain`, authorization code with
// PKCE (S256) + random state + nonce. No token reaches the client and none is kept: the callback verifies the ID
// token (iss, aud, signature, nonce), maps its groups claim to a role and sets a signed session cookie {role, exp},
// TTL 1 h; the groups are re-checked at the next sign-in. The return view rides a 10 min signed flow cookie.
// /ws reads the session at upgrade (Client.Role), so signing in or out takes a reconnect (06 "Keepalive").
//
// Roles: platform-admins → admin (detail frames, Out.Detail); mountain-visitors → visitor (marks, W4);
// anyone else, and no cookie → view.
//
// Env (Secret mountain-oidc, optional): MOUNTAIN_OIDC_ISSUER, MOUNTAIN_OIDC_CLIENT_ID, MOUNTAIN_OIDC_CLIENT_SECRET,
// MOUNTAIN_SESSION_KEY (HMAC key, ≥ 32 bytes). Any unset = off: /auth/login 404, everyone views.
//
//	GET  /auth/login?return=/#view  → authentik
//	GET  /auth/callback             → session cookie, 303 to the return view
//	POST /auth/logout               (same Origin only: CSRF) → cookie cleared, 303 to /
//	GET  /auth/me                   → {"enabled": bool, "role": "admin"|"visitor"|"view"}

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
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
	roleAdmin, roleVisitor = "admin", "visitor" // "" = view

	sessionCookie = "__Host-mountain" // __Host-: Secure, Path=/, no Domain
	flowCookie    = "__Host-mountain-flow"
	sessionTTL    = time.Hour
	flowTTL       = 10 * time.Minute
)

// returnRE: the return view is the page itself plus a query or fragment (the story view), else "/": no other
// path, so no "//host", backslash or scheme trick can make it an open redirect.
var returnRE = regexp.MustCompile(`^/(?:[?#][A-Za-z0-9=&_.-]{0,64})?$`)

// Auth is detail mode's OIDC client; nil = off.
type Auth struct {
	issuer string
	oauth  oauth2.Config // Endpoint filled by discovery
	key    []byte
	dev    bool

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier // nil until the first successful discovery
}

var auth *Auth // set by serve (authFromEnv) before routes run

func authFromEnv(dev bool) *Auth {
	iss, id, sec, key := os.Getenv("MOUNTAIN_OIDC_ISSUER"), os.Getenv("MOUNTAIN_OIDC_CLIENT_ID"), os.Getenv("MOUNTAIN_OIDC_CLIENT_SECRET"), os.Getenv("MOUNTAIN_SESSION_KEY")
	if iss == "" || id == "" || sec == "" || len(key) < 32 {
		log.Printf("auth: MOUNTAIN_OIDC_* / MOUNTAIN_SESSION_KEY unset: detail mode off, everyone views")
		return nil
	}
	return newAuth(iss, id, sec, origin+"/auth/callback", []byte(key), dev)
}

func newAuth(issuer, id, secret, redirect string, key []byte, dev bool) *Auth {
	return &Auth{issuer: issuer, key: key, dev: dev, oauth: oauth2.Config{ClientID: id, ClientSecret: secret, RedirectURL: redirect,
		Scopes: []string{oidc.ScopeOpenID, "profile"}}} // authentik's profile scope carries groups, in the ID token (authentik-oidc skill)
}

// discover runs OIDC discovery once it succeeds (lazily: authentik down at boot must not stop the relay).
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

func roleOf(groups []string) string {
	switch {
	case slices.Contains(groups, "platform-admins"):
		return roleAdmin
	case slices.Contains(groups, "mountain-visitors"):
		return roleVisitor
	}
	return ""
}

// flow is the login round trip's state, in the flow cookie. Signed, not encrypted: it only ever goes back to
// the browser that started the flow, and the verifier is useless without the code it is bound to.
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

// sessionRole is the role of r's session cookie ("" = view: no auth, no cookie, a bad MAC or expired) and its
// expiry: a socket's role lapses with it (Hub.Detail), so a sign-out or a lost group ends detail within the 1 h TTL.
func sessionRole(r *http.Request) (string, time.Time) {
	var role string
	if auth == nil {
		return "", time.Time{}
	}
	if exp, ok := auth.get(r, sessionCookie, &role); ok {
		return role, exp
	}
	return "", time.Time{}
}

func (au *Auth) login(w http.ResponseWriter, r *http.Request) {
	if _, err := au.discover(); err != nil {
		log.Printf("auth: discovery: %v", err)
		http.Error(w, "sign-in unavailable", http.StatusServiceUnavailable)
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
		http.Error(w, "bad sign-in state", http.StatusBadRequest)
		return
	}
	v, err := au.discover()
	if err != nil {
		http.Error(w, "sign-in unavailable", http.StatusServiceUnavailable)
		return
	}
	tok, err := au.oauth.Exchange(r.Context(), q.Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		log.Printf("auth: exchange: %v", err)
		http.Error(w, "sign-in failed", http.StatusForbidden)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := v.Verify(r.Context(), raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(f.Nonce)) != 1 {
		log.Printf("auth: id token: %v", err)
		http.Error(w, "sign-in failed", http.StatusForbidden)
		return
	}
	var c struct{ Groups []string }
	idt.Claims(&c) // no groups claim = view (a missing claim yields the LOW role)
	au.set(w, sessionCookie, roleOf(c.Groups), sessionTTL)
	http.Redirect(w, r, f.Return, http.StatusSeeOther)
}

// logout is POST + same Origin (originOK), so another site can neither GET nor form-POST a sign-out.
// ponytail: the authentik session stays; the next sign-in is one click (end_session if that ever matters).
func (au *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if !originOK(r.Header.Get("Origin"), au.dev) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	clearCookie(w, sessionCookie)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func me(w http.ResponseWriter, r *http.Request) {
	role, _ := sessionRole(r)
	if role == "" {
		role = "view"
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Write(mustJSON(map[string]any{"enabled": auth != nil, "role": role}))
}

func init() {
	on := func(h func(*Auth, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if auth == nil {
				http.NotFound(w, r)
				return
			}
			h(auth, w, r)
		}
	}
	Public.Handle("GET /auth/login", on((*Auth).login))
	Public.Handle("GET /auth/callback", on((*Auth).callback))
	Public.Handle("POST /auth/logout", on((*Auth).logout))
	Public.HandleFunc("GET /auth/me", me)
}
