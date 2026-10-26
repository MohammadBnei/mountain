package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/MohammadBnei/mountain"
)

const (
	mIss = "https://authentik.example/application/o/atlas-api/"
	mAud = "atlas-api-client"
	mSub = "atlas-game"
)

// certPEM is a self-signed certificate for key, as authentik's signing key is pinned.
func certPEM(t *testing.T, key any, pub any) string {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "authentik"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}, &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "authentik"}}, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// withMachine configures the machine principal on the given certificates, as serve does from the env.
func withMachine(t *testing.T, certs string) {
	t.Helper()
	for k, v := range map[string]string{"MOUNTAIN_MACHINE_CERT": certs, "MOUNTAIN_MACHINE_ISSUER": mIss, "MOUNTAIN_MACHINE_CLIENT_ID": mAud, "MOUNTAIN_MACHINE_SUB": mSub} {
		t.Setenv(k, v)
	}
	m, err := machineFromEnv()
	if err != nil || m == nil {
		t.Fatalf("machineFromEnv: %v %v", m, err)
	}
	machine = m
	t.Cleanup(func() { machine = nil })
}

// mint signs claims as authentik's client_credentials grant would (RS256), or with the header's alg as given.
func mint(t *testing.T, key *rsa.PrivateKey, head string, claims map[string]any) string {
	t.Helper()
	in := base64.RawURLEncoding.EncodeToString([]byte(head)) + "." + base64.RawURLEncoding.EncodeToString(mustJSON(claims))
	sum := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return in + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// mintRaw signs a payload written by hand (keys a map can't hold: repeated, or equal but for case).
func mintRaw(t *testing.T, key *rsa.PrivateKey, head, payload string) string {
	t.Helper()
	in := base64.RawURLEncoding.EncodeToString([]byte(head)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload))
	sum := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return in + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func good(exp time.Duration) map[string]any {
	now := time.Now()
	return map[string]any{"iss": mIss, "aud": mAud, "azp": mAud, "sub": mSub, "exp": now.Add(exp).Unix(), "iat": now.Unix(), "nbf": now.Unix()}
}

func with(c map[string]any, k string, v any) map[string]any {
	out := map[string]any{}
	for kk, vv := range c {
		out[kk] = vv
	}
	if v == nil {
		delete(out, k)
	} else {
		out[k] = v
	}
	return out
}

// TestMachineEnv: all four set or none; the certificate must hold an RSA key.
func TestMachineEnv(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if m, err := machineFromEnv(); m != nil || err != nil {
		t.Errorf("none set: %v %v", m, err)
	}
	for _, k := range []string{"MOUNTAIN_MACHINE_CERT", "MOUNTAIN_MACHINE_ISSUER", "MOUNTAIN_MACHINE_CLIENT_ID", "MOUNTAIN_MACHINE_SUB"} {
		t.Setenv(k, "x")
		if _, err := machineFromEnv(); err == nil {
			t.Errorf("only %s set: started", k)
		}
		t.Setenv(k, "")
	}
	t.Setenv("MOUNTAIN_MACHINE_SUB", mSub)
	t.Setenv("MOUNTAIN_MACHINE_ISSUER", mIss)
	t.Setenv("MOUNTAIN_MACHINE_CLIENT_ID", mAud)
	notCert := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("not a certificate")}))
	for name, certs := range map[string]string{"not PEM": "abc", "an EC key": certPEM(t, ec, &ec.PublicKey), "a PEM block not a certificate": notCert} {
		t.Setenv("MOUNTAIN_MACHINE_CERT", certs)
		if _, err := machineFromEnv(); err == nil {
			t.Errorf("%s: started", name)
		}
	}
	t.Setenv("MOUNTAIN_MACHINE_CERT", certPEM(t, key, &key.PublicKey))
	if m, err := machineFromEnv(); err != nil || len(m.keys) != 1 {
		t.Errorf("all set: %v %v", m, err)
	}
}

// TestMachineVerify: RS256 on the pinned key, the provider's iss/aud/azp, the one sub, exp with no leeway and 5 s left.
func TestMachineVerify(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	next, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	withMachine(t, certPEM(t, key, &key.PublicKey)+certPEM(t, next, &next.PublicKey)) // a rotation: both pinned
	rs := `{"alg":"RS256","typ":"JWT"}`
	now := time.Now()
	if exp, err := machine.verify(mint(t, key, rs, good(10*time.Minute)), now); err != nil || exp.Before(now.Add(9*time.Minute)) {
		t.Errorf("good token: %v %v", exp, err)
	}
	if _, err := machine.verify(mint(t, next, rs, good(10*time.Minute)), now); err != nil {
		t.Errorf("the rotation's second key: %v", err)
	}
	if _, err := machine.verify(mint(t, key, rs, with(good(time.Minute), "aud", []string{"x", mAud})), now); err != nil {
		t.Errorf("aud as a list holding it: %v", err)
	}
	hs := func(c map[string]any) string { // HS256 keyed with the public key's bytes: the classic confusion
		in := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`)) + "." + base64.RawURLEncoding.EncodeToString(mustJSON(c))
		m := hmac.New(sha256.New, x509.MarshalPKCS1PublicKey(&key.PublicKey))
		m.Write([]byte(in))
		return in + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	}
	for name, tok := range map[string]string{
		"HS256":             hs(good(time.Minute)),
		"alg none":          base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(mustJSON(good(time.Minute))) + ".",
		"another key":       mint(t, other, rs, good(time.Minute)),
		"a jwk of its own":  mint(t, other, `{"alg":"RS256","jwk":{"kty":"RSA"},"kid":"x","jku":"https://evil.example/jwks"}`, good(time.Minute)),
		"wrong iss":         mint(t, key, rs, with(good(time.Minute), "iss", "https://authentik.example/application/o/other/")),
		"wrong aud":         mint(t, key, rs, with(good(time.Minute), "aud", "other")),
		"wrong azp":         mint(t, key, rs, with(good(time.Minute), "azp", "other")),
		"no azp":            mint(t, key, rs, with(good(time.Minute), "azp", nil)),
		"another sub":       mint(t, key, rs, with(good(time.Minute), "sub", "ak-admin")),
		"no exp":            mint(t, key, rs, with(good(time.Minute), "exp", nil)),
		"expired":           mint(t, key, rs, with(good(time.Minute), "exp", now.Add(-time.Second).Unix())),
		"under 5 s left":    mint(t, key, rs, with(good(time.Minute), "exp", now.Add(4*time.Second).Unix())),
		"nbf ahead":         mint(t, key, rs, with(good(time.Hour), "nbf", now.Add(2*time.Minute).Unix())),
		"iat ahead":         mint(t, key, rs, with(good(time.Hour), "iat", now.Add(2*time.Minute).Unix())),
		"two parts":         "a.b",
		"a Sub beside sub":  mintRaw(t, key, rs, fmt.Sprintf(`{"iss":%q,"aud":%q,"azp":%q,"sub":"ak-other","Sub":%q,"exp":%d}`, mIss, mAud, mAud, mSub, now.Add(time.Hour).Unix())),
		"an AZP beside azp": mintRaw(t, key, rs, fmt.Sprintf(`{"iss":%q,"aud":%q,"azp":"other","AZP":%q,"sub":%q,"exp":%d}`, mIss, mAud, mAud, mSub, now.Add(time.Hour).Unix())),
		"sub twice":         mintRaw(t, key, rs, fmt.Sprintf(`{"iss":%q,"aud":%q,"azp":%q,"sub":"ak-other","sub":%q,"exp":%d}`, mIss, mAud, mAud, mSub, now.Add(time.Hour).Unix())),
		"an ALG header":     mintRaw(t, key, `{"ALG":"RS256"}`, string(mustJSON(good(time.Hour)))),
		"Sub alone":         mintRaw(t, key, rs, fmt.Sprintf(`{"iss":%q,"aud":%q,"azp":%q,"Sub":%q,"exp":%d}`, mIss, mAud, mAud, mSub, now.Add(time.Hour).Unix())),
		"trailing data":     mintRaw(t, key, rs, string(mustJSON(good(time.Hour)))+`{}`),
		"tampered claims": func() string {
			p := strings.Split(mint(t, key, rs, good(time.Minute)), ".")
			p[1] = base64.RawURLEncoding.EncodeToString(mustJSON(with(good(time.Minute), "sub", "x")))
			return strings.Join(p, ".")
		}(),
	} {
		if _, err := machine.verify(tok, now); err == nil {
			t.Errorf("%s: verified", name)
		}
	}
}

// TestMachineRoutes: a request with Authorization is judged by it alone. The machine principal reads the town's two
// routes and is refused on every other gated route; a bad token is a 401 everywhere, an admin's cookie beside it
// changes nothing; with the machine path off, a bearer is refused.
func TestMachineRoutes(t *testing.T) {
	p := withAuth(t)
	admin := adminCookie(t, p)
	a, _ := testApp(t)
	www, _ := fs.Sub(mountain.WWW, "www")
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	withMachine(t, certPEM(t, key, &key.PublicKey))
	tok := "Bearer " + mint(t, key, `{"alg":"RS256"}`, good(10*time.Minute))
	bad := "Bearer " + mint(t, key, `{"alg":"RS256"}`, with(good(10*time.Minute), "sub", "someone"))
	refused := func(r *http.Response) bool {
		return r.StatusCode == http.StatusUnauthorized && strings.HasPrefix(r.Header.Get("WWW-Authenticate"), "Bearer") && r.Header.Get("Cache-Control") == "no-store"
	}
	seen := 0
	for _, rt := range routeTable(a, www) {
		if rt.open {
			continue
		}
		method, path, _ := strings.Cut(rt.pattern, " ")
		r := serveReq(method, path, nil, "Authorization", tok)
		switch rt.pattern {
		case "GET /api/graph":
			seen++
			if r.StatusCode == http.StatusUnauthorized || r.StatusCode == http.StatusForbidden || r.StatusCode == http.StatusFound {
				t.Errorf("machine %s: %d", rt.pattern, r.StatusCode)
			}
		case "GET /ws":
			seen++ // a plain GET isn't an upgrade: past the gate, the hub answers it (TestMachineSocket dials it)
			if r.StatusCode == http.StatusUnauthorized || r.StatusCode == http.StatusForbidden {
				t.Errorf("machine %s: %d", rt.pattern, r.StatusCode)
			}
		default:
			if !refused(r) {
				t.Errorf("machine on %s: %d %q, want 401", rt.pattern, r.StatusCode, r.Header.Get("WWW-Authenticate"))
			}
		}
		for name, h := range map[string]string{"bad token": bad, "basic": "Basic YTpi", "empty bearer": "Bearer ", "lowercase scheme": strings.Replace(tok, "Bearer", "bearer", 1)} {
			if r := serveReq(method, path, []*http.Cookie{admin}, "Authorization", h, "Origin", origin); !refused(r) {
				t.Errorf("%s with an admin's cookie on %s: %d, want 401 (judged by the header alone)", name, rt.pattern, r.StatusCode)
			}
		}
	}
	for name, vals := range map[string][]string{"an empty Authorization": {""}, "two, the first empty": {"", tok}, "two valid": {tok, tok}} {
		r := httptest.NewRequest("GET", "/api/read?repo=x&path=y", nil)
		r.AddCookie(admin)
		r.Header["Authorization"] = vals
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if !refused(w.Result()) {
			t.Errorf("%s with an admin's cookie: %d, want 401 (never the cookie)", name, w.Code)
		}
	}
	if r := serveReq("HEAD", "/api/graph", nil, "Authorization", tok); r.StatusCode == http.StatusUnauthorized || r.StatusCode == http.StatusForbidden {
		t.Errorf("HEAD /api/graph: %d (a read)", r.StatusCode)
	}
	for _, m := range [][2]string{{"HEAD", "/api/read"}, {"GET", "/api/graph/"}, {"GET", "/ws/"}, {"GET", "/"}} {
		if r := serveReq(m[0], m[1], nil, "Authorization", tok); !refused(r) {
			t.Errorf("machine %s %s: %d, want 401", m[0], m[1], r.StatusCode)
		}
	}
	if seen != 2 {
		t.Errorf("the machine's routes are gone from the table: %d of 2 seen", seen)
	}
	if r := serveReq("GET", "/api/graph?access_token="+strings.TrimPrefix(tok, "Bearer "), nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("a token in the query: %d, want the cookie path's 403", r.StatusCode)
	}
	machine = nil
	if r := serveReq("GET", "/api/graph", nil, "Authorization", tok); !refused(r) {
		t.Errorf("machine path off, a bearer: %d", r.StatusCode)
	}
}

// TestMachineSocket: the machine principal's socket needs no Origin, gets the hello, and ends at the token's exp.
func TestMachineSocket(t *testing.T) {
	admin := adminCookie(t, withAuth(t)) // ridden along: a bearer is judged alone, so the socket still ends at the token's exp
	_, h := testApp(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	withMachine(t, certPEM(t, key, &key.PublicKey))
	exp := time.Unix(time.Now().Add(10*time.Second).Unix(), 0)
	tok := mint(t, key, `{"alg":"RS256"}`, with(good(time.Minute), "exp", exp.Unix()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + tok}, "Cookie": {admin.String()}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(-1)
	if f := readT(t, conn); f["t"] != "hello" {
		t.Fatalf("hello: %v", f)
	}
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			break
		}
	}
	if end := time.Now(); end.Before(exp.Add(-time.Second)) || end.After(exp.Add(3*time.Second)) {
		t.Errorf("the socket ended at %v, the token at %v", end.Format(time.StampMilli), exp.Format(time.StampMilli))
	}
}
