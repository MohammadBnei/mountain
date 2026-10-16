package main

// Live transcription test on the stt crystal's card (W5; user: "a test record like the one we did in dreamer, only for
// connected users"). Same shape as dream-analyst's /api/transcribe (ADR-0044 amended, ADR-0046):
//
//	POST /api/transcribe?stream=<client id>&last=1&lang=fr
//	body: one raw 16 kHz mono s16le PCM chunk (js/stt-capture.js, ~560 ms), → {"text": the chunk's new text}
//
// Signed-in admins and visitors only (401 for view), same Origin only (CSRF), ≤ 64 KiB a chunk, ≤ 2 chunks/s per
// account (a dictation sends ~1.8/s). The relay calls ukubi-stt's Recognize over in-cluster h2c with its own token
// STT_TOKEN_MOUNTAIN; the browser holds no credential and never names the STT session: it is HMAC(token, sub:stream),
// so one user cannot feed audio into another's recognizer. No audio or text is kept or logged.
// Env (Infisical mountain-8c-cz, read at start): STT_TOKEN_MOUNTAIN (unset = 503), STT_ADDR (default in-cluster).

import (
	"bytes"
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

const sttChunkMax = 64 << 10 // 560 ms of 16 kHz s16 = 17.5 KiB

var (
	sttToken = os.Getenv("STT_TOKEN_MOUNTAIN")
	sttAddr  = cmp.Or(os.Getenv("STT_ADDR"), "http://ukubi-stt.ukubi-stt.svc.cluster.local:9090")
	sttHTTP  = func() *http.Client {
		p := new(http.Protocols)
		p.SetUnencryptedHTTP2(true) // gRPC over h2c: in-cluster, no TLS
		return &http.Client{Transport: &http.Transport{Protocols: p}, Timeout: 15 * time.Second}
	}()
	sttMu   sync.Mutex
	sttRate = map[string]*sttBucket{} // ponytail: never pruned; one small entry per signed-in account
)

type sttBucket struct {
	n  float64
	at time.Time
}

// sttAllow is a token bucket per account: 4 chunks of burst (the tail flush follows a chunk), 2 a second.
func sttAllow(sub string, now time.Time) bool {
	sttMu.Lock()
	defer sttMu.Unlock()
	b := sttRate[sub]
	if b == nil {
		b = &sttBucket{n: 4, at: now}
		sttRate[sub] = b
	}
	b.n = min(4, b.n+2*now.Sub(b.at).Seconds())
	b.at = now
	if b.n < 1 {
		return false
	}
	b.n--
	return true
}

func transcribe(w http.ResponseWriter, r *http.Request) {
	s, _ := sessionOf(r)
	switch {
	case s.Role == "":
		http.Error(w, "sign in to try it", http.StatusUnauthorized)
		return
	case !originOK(r.Header.Get("Origin"), auth.dev):
		http.Error(w, "bad origin", http.StatusForbidden)
		return
	case sttToken == "":
		http.Error(w, "transcription is off", http.StatusServiceUnavailable)
		return
	case !sttAllow(s.Sub, time.Now()):
		http.Error(w, "slow down", http.StatusTooManyRequests)
		return
	}
	q := r.URL.Query()
	stream, last, lang := q.Get("stream"), q.Get("last") == "1", q.Get("lang")
	if lang != "fr" {
		lang = "en"
	}
	pcm, err := io.ReadAll(http.MaxBytesReader(w, r.Body, sttChunkMax))
	if err != nil || len(stream) == 0 || len(stream) > 64 || len(pcm)%2 != 0 || (len(pcm) == 0 && !last) {
		http.Error(w, "bad chunk", http.StatusBadRequest)
		return
	}
	text, code, err := recognize(r, pcm, sttSession(s.Sub, stream), last, lang)
	if err != nil {
		http.Error(w, "transcription failed", code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{"text": text})
}

// sttSession: HMAC keyed on the token (ADR-0046), so the client never chooses the id the service sees.
func sttSession(sub, stream string) string {
	m := hmac.New(sha256.New, []byte(sttToken))
	m.Write([]byte(sub + ":" + stream))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// recognize is one unary /stt.v1.Stt/Recognize call; the protobuf is hand-written (dream-analyst sttService.ts):
// RecognizeRequest{config=1{sample_rate_hertz=1, language=2}, audio=2, session_id=3, last=5} → {text=1, ...}.
// Errors carry the HTTP status for the browser: 503 for RESOURCE_EXHAUSTED / UNAVAILABLE / transport, else 502.
func recognize(r *http.Request, pcm []byte, session string, last bool, lang string) (string, int, error) {
	field := func(b []byte, n int, v []byte) []byte {
		return append(binary.AppendUvarint(append(b, byte(n<<3|2)), uint64(len(v))), v...)
	}
	cfg := field(binary.AppendUvarint([]byte{1 << 3}, 16000), 2, []byte(lang))
	msg := field(field(nil, 1, cfg), 3, []byte(session))
	if last {
		msg = append(msg, 5<<3, 1)
	}
	msg = field(msg, 2, pcm)
	body := binary.BigEndian.AppendUint32([]byte{0}, uint32(len(msg))) // gRPC frame: uncompressed + length
	req, _ := http.NewRequestWithContext(r.Context(), "POST", sttAddr+"/stt.v1.Stt/Recognize", bytes.NewReader(append(body, msg...)))
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")
	req.Header.Set("Authorization", "Bearer "+sttToken)
	resp, err := sttHTTP.Do(req)
	if err != nil {
		return "", http.StatusServiceUnavailable, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", http.StatusServiceUnavailable, err
	}
	// a failed call is still HTTP 200: the status is in the trailers (or the headers, trailers-only)
	st := cmp.Or(resp.Trailer.Get("Grpc-Status"), resp.Header.Get("Grpc-Status"))
	if st != "0" {
		msg, _ := url.PathUnescape(cmp.Or(resp.Trailer.Get("Grpc-Message"), resp.Header.Get("Grpc-Message")))
		if st == "8" || st == "14" {
			return "", http.StatusServiceUnavailable, errors.New(msg)
		}
		return "", http.StatusBadGateway, errors.New("grpc-status " + st + ": " + msg)
	}
	text, err := sttText(out)
	if err != nil {
		return "", http.StatusBadGateway, err
	}
	return text, 0, nil
}

// sttText reads field 1 (text) of the RecognizeResponse frames, skipping every other field.
func sttText(b []byte) (string, error) {
	bad := errors.New("bad recognize response")
	for len(b) >= 5 {
		n := int(binary.BigEndian.Uint32(b[1:5]))
		if len(b) < 5+n {
			return "", bad
		}
		p := b[5 : 5+n]
		b = b[5+n:]
		for len(p) > 0 {
			key, k := binary.Uvarint(p)
			if k <= 0 {
				return "", bad
			}
			p = p[k:]
			switch key & 7 {
			case 0:
				_, k = binary.Uvarint(p)
			case 1:
				k = 8
			case 5:
				k = 4
			case 2:
				l, m := binary.Uvarint(p)
				if m <= 0 || uint64(len(p)-m) < l {
					return "", bad
				}
				if key>>3 == 1 {
					return string(p[m : m+int(l)]), nil
				}
				k = m + int(l)
			default:
				return "", bad
			}
			if k <= 0 || k > len(p) {
				return "", bad
			}
			p = p[k:]
		}
	}
	return "", nil
}

func init() { Public.HandleFunc("POST /api/transcribe", transcribe) }
