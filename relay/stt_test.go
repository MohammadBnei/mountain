package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeSTT is ukubi-stt's Recognize over h2c: it checks the token and echoes the request's session, last flag and
// audio length as the text (plus a float field to skip), with grpc-status in the trailers.
func fakeSTT(t *testing.T, status string) *httptest.Server {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 || r.URL.Path != "/stt.v1.Stt/Recognize" || r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("Content-Type") != "application/grpc" {
			t.Errorf("request: %s %s %v", r.Proto, r.URL.Path, r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		req := b[5:]
		_, rest := field(t, req, 1) // config
		sess, rest := field(t, rest, 3)
		last := len(rest) > 2 && rest[0] == 5<<3 && rest[1] == 1
		if last {
			rest = rest[2:]
		}
		audio, _ := field(t, rest, 2)
		text := string(sess) + "|" + map[bool]string{true: "last", false: "more"}[last] + "|" + string(rune('0'+len(audio)/2))
		msg := append([]byte{2<<3 | 5, 0, 0, 0x80, 0x3f}, 1<<3|2, byte(len(text))) // audio_seconds float first, then text
		msg = append(msg, text...)
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
		w.Write(binary.BigEndian.AppendUint32([]byte{0}, uint32(len(msg))))
		w.Write(msg)
		w.Header().Set("Grpc-Status", status)
		w.Header().Set("Grpc-Message", "busy%20gpu")
	}))
	p := new(http.Protocols)
	p.SetUnencryptedHTTP2(true)
	srv.Config.Protocols = p
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func field(t *testing.T, b []byte, n byte) ([]byte, []byte) {
	t.Helper()
	if len(b) < 2 || b[0] != n<<3|2 {
		t.Fatalf("field %d: % x", n, b[:min(len(b), 8)])
	}
	l, k := binary.Uvarint(b[1:])
	return b[1+k : 1+k+int(l)], b[1+k+int(l):]
}

func TestTranscribe(t *testing.T) {
	auth = newAuth("http://idp.invalid", "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	t.Cleanup(func() { auth = nil; sttToken, sttAddr = "", "" })
	cookieFor := func(role, sub string) *http.Cookie {
		w := httptest.NewRecorder()
		auth.set(w, sessionCookie, Session{Role: role, Sub: sub}, time.Hour)
		return w.Result().Cookies()[0]
	}
	post := func(c *http.Cookie, q string, body []byte, org string) (int, string) {
		r := httptest.NewRequest("POST", "/api/transcribe?"+q, bytes.NewReader(body))
		if c != nil {
			r.AddCookie(c)
		}
		r.Header.Set("Origin", org)
		w := httptest.NewRecorder()
		Public.ServeHTTP(w, r)
		return w.Code, strings.TrimSpace(w.Body.String())
	}
	pcm := make([]byte, 8) // 4 samples
	vis := cookieFor(roleVisitor, "u1")

	if c, _ := post(nil, "stream=a", pcm, origin); c != http.StatusUnauthorized {
		t.Fatalf("no session: %d", c)
	}
	if c, _ := post(cookieFor("", "u0"), "stream=a", pcm, origin); c != http.StatusUnauthorized {
		t.Fatalf("view role: %d", c)
	}
	if c, _ := post(vis, "stream=a", pcm, "https://evil.example"); c != http.StatusForbidden {
		t.Fatalf("cross origin: %d", c)
	}
	if c, _ := post(vis, "stream=a", pcm, origin); c != http.StatusServiceUnavailable {
		t.Fatalf("no token: %d", c)
	}

	sttToken, sttAddr = "tok", fakeSTT(t, "0").URL
	sess := sttSession("u1", "a")
	if c, b := post(vis, "stream=a&lang=fr", pcm, origin); c != 200 || b != `{"text":"`+sess+`|more|4"}` {
		t.Fatalf("chunk: %d %s", c, b)
	}
	if sttSession("u2", "a") == sess || len(sess) != 32 {
		t.Fatal("session id must be per account")
	}
	if c, b := post(vis, "stream=a&last=1", nil, origin); c != 200 || !strings.Contains(b, "|last|0") {
		t.Fatalf("empty close: %d %s", c, b)
	}
	for _, q := range []string{"stream=", "stream=" + strings.Repeat("x", 65)} {
		if c, _ := post(cookieFor(roleAdmin, "u3"), q, pcm, origin); c != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, c)
		}
	}
	if c, _ := post(cookieFor(roleAdmin, "u4"), "stream=a", make([]byte, sttChunkMax+2), origin); c != http.StatusBadRequest {
		t.Fatalf("oversize: %d", c)
	}
	if c, _ := post(cookieFor(roleAdmin, "u5"), "stream=a", []byte{1, 2, 3}, origin); c != http.StatusBadRequest {
		t.Fatalf("odd bytes: %d", c)
	}

	// the bucket: 4 of burst, then 429 until it refills
	adm := cookieFor(roleAdmin, "u6")
	codes := []int{}
	for range 6 {
		c, _ := post(adm, "stream=b", pcm, origin)
		codes = append(codes, c)
	}
	if codes[3] != 200 || codes[4] != http.StatusTooManyRequests {
		t.Fatalf("rate: %v", codes)
	}
	now := time.Now()
	if !sttAllow("u7", now) || !sttAllow("u7", now) || !sttAllow("u7", now) || !sttAllow("u7", now) || sttAllow("u7", now) || !sttAllow("u7", now.Add(600*time.Millisecond)) {
		t.Fatal("bucket refills at 2/s")
	}

	sttAddr = fakeSTT(t, "8").URL
	if c, _ := post(cookieFor(roleAdmin, "u8"), "stream=a", pcm, origin); c != http.StatusServiceUnavailable {
		t.Fatalf("RESOURCE_EXHAUSTED: %d", c)
	}
	sttAddr = fakeSTT(t, "3").URL
	if c, _ := post(cookieFor(roleAdmin, "u9"), "stream=a", pcm, origin); c != http.StatusBadGateway {
		t.Fatalf("INVALID_ARGUMENT: %d", c)
	}
}
