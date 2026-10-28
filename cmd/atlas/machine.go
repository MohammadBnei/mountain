package main

// The machine principal (plan/13 §1, 00 Audience R10 as amended; infra-bootstrap ADR-0068): one authentik service
// account, the game's (atlas-game), reads the town's two routes, GET /api/graph and GET /ws, with an access token from
// the `atlas-api` provider's client_credentials grant. Read-only by construction: those two routes write nothing, and
// it reaches no other.
//
// A request that carries Authorization is judged by it alone, never by a cookie too: a token that fails is a 401
// with WWW-Authenticate, on any gated route. The token is checked offline, as the bridge checks its clients (ADR-0066):
// RS256 only, decided before any key is used (no kid, jku, x5u, x5c or jwk is read), against the pinned certificate
// (MOUNTAIN_MACHINE_CERT, one or more PEM certificates during a rotation: the trust anchor, never a JWKS fetched over
// the pod network); iss, aud and azp the provider's; sub the one principal; exp with no leeway and at least 5 s left
// (a socket ends at it); nbf and iat with a minute.
//
// It is independent of people's sign-in: configured, it reads its two routes whether or not OIDC is. The game's server
// holds one socket, so the hub's per-address cap never binds it.
//
// Env: MOUNTAIN_MACHINE_CERT, MOUNTAIN_MACHINE_ISSUER, MOUNTAIN_MACHINE_CLIENT_ID, MOUNTAIN_MACHINE_SUB: all set, or
// none (the machine path off); some set is a start-up error.

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Machine verifies the machine principal's tokens.
type Machine struct {
	keys             []*rsa.PublicKey
	issuer, aud, sub string
}

// machineRoutes are the routes the machine principal may read: the town's.
var machineRoutes = []string{"GET /api/graph", "GET /ws"}

// machineFrom is the machine principal from its configuration (config.go): all four set, or none (nil, nil).
func machineFrom(e machineEnv) (*Machine, error) {
	certs, iss, aud, sub := e.Cert, e.Issuer, e.ClientID, e.Sub
	set := 0
	for _, v := range []string{certs, iss, aud, sub} {
		if v != "" {
			set++
		}
	}
	switch set {
	case 0:
		return nil, nil
	case 4:
	default:
		return nil, errors.New("atlas: MOUNTAIN_MACHINE_CERT, _ISSUER, _CLIENT_ID and _SUB are all set or none")
	}
	m := &Machine{issuer: iss, aud: aud, sub: sub}
	rest := []byte(certs)
	for {
		var b *pem.Block
		if b, rest = pem.Decode(rest); b == nil {
			break
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("atlas: MOUNTAIN_MACHINE_CERT: %w", err)
		}
		k, ok := c.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("atlas: MOUNTAIN_MACHINE_CERT: not an RSA certificate")
		}
		m.keys = append(m.keys, k)
	}
	if len(m.keys) == 0 {
		return nil, errors.New("atlas: MOUNTAIN_MACHINE_CERT holds no certificate")
	}
	return m, nil
}

// minLeft is how long a token must still hold to open anything: a socket ends at its exp.
const minLeft = 5 * time.Second

// verify checks a token and returns its exp.
func (m *Machine) verify(tok string, now time.Time) (time.Time, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("not a JWT")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return time.Time{}, errors.New("bad header encoding")
	}
	head, err := exactKeys(hb)
	if err != nil {
		return time.Time{}, err
	}
	var alg string
	if json.Unmarshal(head["alg"], &alg) != nil || alg != "RS256" {
		return time.Time{}, errors.New("RS256 only")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return time.Time{}, errors.New("bad signature encoding")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !slices.ContainsFunc(m.keys, func(k *rsa.PublicKey) bool { return rsa.VerifyPKCS1v15(k, crypto.SHA256, sum[:], sig) == nil }) {
		return time.Time{}, errors.New("bad signature")
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, errors.New("bad claims encoding")
	}
	claims, err := exactKeys(pb)
	if err != nil {
		return time.Time{}, err
	}
	var c struct {
		Iss, Azp, Sub string
		Aud           json.RawMessage
		Exp, Nbf, Iat *int64
	}
	for k, dst := range map[string]any{"iss": &c.Iss, "azp": &c.Azp, "sub": &c.Sub, "exp": &c.Exp, "nbf": &c.Nbf, "iat": &c.Iat} {
		if v, ok := claims[k]; ok && json.Unmarshal(v, dst) != nil {
			return time.Time{}, errors.New("bad claim " + k)
		}
	}
	c.Aud = claims["aud"]
	var auds []string
	if json.Unmarshal(c.Aud, &auds) != nil {
		var one string
		if json.Unmarshal(c.Aud, &one) == nil {
			auds = []string{one}
		}
	}
	skew := time.Minute
	switch {
	case c.Iss != m.issuer:
		return time.Time{}, errors.New("wrong issuer")
	case !slices.Contains(auds, m.aud) || c.Azp != m.aud:
		return time.Time{}, errors.New("wrong audience")
	case c.Sub != m.sub:
		return time.Time{}, errors.New("not the machine principal")
	case c.Exp == nil || !now.Add(minLeft).Before(time.Unix(*c.Exp, 0)):
		return time.Time{}, errors.New("expired")
	case c.Nbf != nil && now.Add(skew).Before(time.Unix(*c.Nbf, 0)), c.Iat != nil && now.Add(skew).Before(time.Unix(*c.Iat, 0)):
		return time.Time{}, errors.New("not valid yet")
	}
	return time.Unix(*c.Exp, 0), nil
}

// exactKeys decodes a JWT segment into its top-level members by their exact names. encoding/json matches a struct's
// fields ignoring case and lets a repeated key overwrite the first, so a "Sub" or a second "azp" that another
// provider's scope mapping adds would be read as the claim; a segment holding two keys equal but for case, or one key
// twice, is refused instead.
func exactKeys(b []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	out, folded := map[string]json.RawMessage{}, map[string]bool{}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, errors.New("bad JSON")
		}
		k, _ := t.(string)
		if folded[strings.ToLower(k)] {
			return nil, errors.New("a repeated or case-folded key: " + k)
		}
		folded[strings.ToLower(k)] = true
		var v json.RawMessage
		if err := d.Decode(&v); err != nil {
			return nil, errors.New("bad JSON")
		}
		out[k] = v
	}
	if t, err := d.Token(); err != nil || t != json.Delim('}') {
		return nil, errors.New("bad JSON")
	}
	if d.More() {
		return nil, errors.New("trailing data")
	}
	return out, nil
}

type machineKey struct{}

// machineExp is the verified token's exp, when the request is the machine principal's.
func machineExp(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(machineKey{}).(time.Time)
	return t, ok
}

// unauthorized refuses a bearer.
func unauthorized(w http.ResponseWriter, why string) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "machine token refused: "+why, http.StatusUnauthorized)
}

// bearerFirst judges a request that carries Authorization by it alone: on a machine route, a verified token goes on
// (its exp in the context); anywhere else, or unverified, it is a 401. A request without one goes to next (the
// cookie's path).
func (g *gate) bearerFirst(pattern string, h, next http.Handler) http.Handler {
	allowed := slices.Contains(machineRoutes, pattern)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vals, present := r.Header["Authorization"] // present, even empty or twice: judged by it, never the cookie
		if !present {
			next.ServeHTTP(w, r)
			return
		}
		tok, ok := "", false
		if len(vals) == 1 {
			tok, ok = strings.CutPrefix(vals[0], "Bearer ")
		}
		switch {
		case !ok || tok == "":
			unauthorized(w, "a Bearer token only")
		case g.machine == nil:
			unauthorized(w, "no machine principal is configured")
		case !allowed:
			unauthorized(w, "the machine principal reads the town's routes only")
		default:
			exp, err := g.machine.verify(tok, time.Now())
			if err != nil {
				unauthorized(w, err.Error())
				return
			}
			h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), machineKey{}, exp)))
		}
	})
}
