package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/MohammadBnei/mountain/internal/bridgev1"
	"github.com/MohammadBnei/mountain/internal/bridgev1/bridgev1connect"
	"github.com/MohammadBnei/mountain/realms"
)

// System tags (conception-2026-11-21 §6, §16 decision 6): atlas, a bridge client, resolves each event's System and
// attaches it through the bridge, so the bridge never imports mountain. The tag lives beside the event, never in it.
//
// Every event the stream delivers is tagged, at its resolution: hints come only from the adapters below, read from
// the fields each sets, and any other event (an alert, a GitHub, fleet or game event, anything a client published
// that is not kwatch's) is unresolved without its data being read. Only applied may route a wake (realms.Tag): atlas
// does not enforce it, it writes the resolution with the tag, and the bridge delivers applied tags only to a System
// filter by default and carries the tag on SubscribeResponse (ukubi-bridge README "Tag").
//
// Off unless ATLAS_BRIDGE_URL and one way to sign in are set (ATLAS_BRIDGE_TOKEN_FILE: atlas's own projected
// ServiceAccount token, audience ukubi-bridge, the bridge row's "sa"; or ATLAS_BRIDGE_PASSWORD_FILE: the bridge-atlas
// authentik app password), and atlas runs in the cluster (the Applications are read with its own service account,
// infra-bootstrap gitops/platform/mountain). Off is never silent: mountain_source_last_success{source="bridge-tags"}
// is seeded at start whatever the config and renewed only by server proof (see runTags), and so is source="bridge-apps" for the Applications list, so MountainSourceStale fires for either.

// The adapters atlas takes hints from. The bridge sets every event's source: a client's Publish is always
// "client/<its name>", so no other client can send as kwatch, and the Forgejo route's is git.bnei.dev/<owner>/<repo>,
// from a signed webhook or the relist loop's own reads.
const (
	kwatchSource  = "client/bridge-kwatch" // ukubi-bridge cmd/kwatch: deploy.* (refs.app) and k8s.warning (refs.kind, …)
	forgejoSource = "git.bnei.dev/"        // ukubi-bridge internal/forgejo: dev.bnei.forge.*
)

// hintsOf is what ev's adapter says it is about, from the fields that adapter sets; nothing for any other event.
//   - kwatch's deploy.*: refs.app, the Application (its name, or <namespace>/<name> outside argocd);
//   - kwatch's k8s.warning: refs' object, a chain of one. Anyone who may create an Event in a namespace can name any
//     object in it, and kwatch sends no tracking id, so a warning never reaches applied: placement at most;
//   - Forgejo's forge.*: the repo, which is the event's source itself.
func hintsOf(ev bridgeEvent) realms.Hints {
	var d struct {
		Refs struct {
			App        string `json:"app"`
			Namespace  string `json:"namespace"`
			Kind       string `json:"kind"`
			Name       string `json:"name"`
			APIVersion string `json:"api_version"`
		} `json:"refs"`
	}
	switch {
	case ev.Source == kwatchSource && strings.HasPrefix(ev.Type, "dev.bnei.deploy."):
		if json.Unmarshal(ev.Data, &d) == nil {
			return realms.Hints{App: d.Refs.App}
		}
	case ev.Source == kwatchSource && ev.Type == "dev.bnei.k8s.warning":
		if json.Unmarshal(ev.Data, &d) != nil || d.Refs.Kind == "" || d.Refs.Name == "" {
			break
		}
		g, _, ok := strings.Cut(d.Refs.APIVersion, "/")
		if !ok { // the core group's is "v1"; kwatch blanks one that is not a shape Kubernetes writes: group unknown
			if d.Refs.APIVersion != "v1" {
				break
			}
			g = ""
		}
		return realms.Hints{OwnerChain: []realms.Ref{{Group: g, Kind: d.Refs.Kind, Namespace: d.Refs.Namespace, Name: d.Refs.Name}}}
	case strings.HasPrefix(ev.Source, forgejoSource) && strings.HasPrefix(ev.Type, "dev.bnei.forge."):
		return realms.Hints{Repo: ev.Source}
	}
	return realms.Hints{}
}

// bridgeEvent is what the tag loop reads of a bridge.v1.SubscribeResponse: an event's log position, source, type and
// JSON data, or a gap's version (GapVersion != 0, no event).
type bridgeEvent struct {
	Seq          int64
	Source, Type string
	Data         []byte
	GapVersion   int64
}

// eventTag is a bridge.v1.EventTag: one System an event resolved to.
type eventTag struct {
	Seq                                 int64
	System, Resolution, RegistryVersion string
}

// bridgeTags is one Subscribe stream as the tag loop uses it, with Ack, ConfirmGap and Tag. Errors are the Connect
// client's (connect.CodeOf reads them).
type bridgeTags interface {
	Next(ctx context.Context) (bridgeEvent, error) // the stream's next message; the bridge sends it after the last is acked
	Ack(ctx context.Context, seq int64) error
	ConfirmGap(ctx context.Context, version int64) error
	Tag(ctx context.Context, tags []eventTag) error
	Close() error
}

// eventTags are t's bridge rows: one per candidate of an ambiguous tag (the bridge keys a tag on its System), else one.
func eventTags(seq int64, t realms.Tag) []eventTag {
	if t.Resolution != realms.Ambiguous {
		return []eventTag{{seq, t.System, t.Resolution, t.RegistryVersion}}
	}
	out := make([]eventTag, 0, len(t.Candidates))
	for _, c := range t.Candidates {
		out = append(out, eventTag{seq, c, t.Resolution, t.RegistryVersion})
	}
	return out
}

var errNoSystems = errors.New("tags: no registry version yet")

// tagLoop tags each event against what systems returns then, and acks it, until the stream fails or ctx ends; it
// returns how many it acked. A gap is confirmed at once: atlas tags what the log holds, a deleted event has nothing
// to tag. Atlas always acks (§6): an event whose tag the bridge refuses for good (NotFound: retention pruned it;
// InvalidArgument: the schema's checks) is logged and acked untagged, never Nacked (a dead letter would park its
// subject). Any other refusal leaves it unacked, so the bridge sends it again on the next stream; runTags decides
// whether there is one.
func tagLoop(ctx context.Context, b bridgeTags, systems func() *realms.Systems) (int, error) {
	acked := 0
	for {
		ev, err := b.Next(ctx)
		if err != nil {
			return acked, err
		}
		if ev.GapVersion != 0 {
			if err := b.ConfirmGap(ctx, ev.GapVersion); err != nil {
				return acked, err
			}
			continue
		}
		sys := systems()
		if sys == nil {
			return acked, errNoSystems // unacked: tagged once the Applications are listed
		}
		switch err := b.Tag(ctx, eventTags(ev.Seq, sys.System(hintsOf(ev)))); connect.CodeOf(err) {
		case connect.CodeNotFound, connect.CodeInvalidArgument:
			slog.Error(fmt.Sprintf("tags: event %d: tag refused, acked untagged: %v", ev.Seq, err))
		default:
			if err != nil {
				return acked, fmt.Errorf("tags: event %d: %w", ev.Seq, err)
			}
		}
		if err := b.Ack(ctx, ev.Seq); err != nil {
			return acked, err
		}
		acked++
	}
}

// runTags opens a stream and runs tagLoop on it until ctx ends, reconnecting after each end: at once after a stream
// that acked something or lived at least every (the bridge ends one at its token's exp, so an idle stream ends too),
// else backing off from 1 s to a minute (no Applications listed yet, the bridge down, a token refused).
// PermissionDenied stops it for good, logged once: the bridge's client table does not let bridge-atlas read every type
// or tag, and no retry changes that.
//
// ok is the health mark, renewed only on proof from the server: after a stream that acked, and every `every` while one
// lives by a probe (an empty Tag: the bridge authenticates and checks the tag right, then refuses it InvalidArgument
// before it touches its store), so a wedged handler, a half-open connection or an open stream the server never
// answers on does not count. No probe while no Applications are listed (systems nil): the loop cannot tag then.
func runTags(ctx context.Context, open func(context.Context) (bridgeTags, error), systems func() *realms.Systems,
	sleep func(context.Context, time.Duration) bool, ok func(), every time.Duration) {
	delay := time.Second
	for {
		var n int
		start := time.Now()
		b, err := open(ctx)
		if err == nil {
			sctx, stop := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() {
				defer close(done)
				for sleepCtx(sctx, every) {
					if systems() == nil {
						continue
					}
					if err := b.Tag(sctx, nil); err == nil || connect.CodeOf(err) == connect.CodeInvalidArgument {
						ok()
					}
				}
			}()
			n, err = tagLoop(sctx, b, systems)
			stop()
			<-done
			_ = b.Close()
			if n > 0 {
				ok()
			}
		}
		if ctx.Err() != nil {
			return
		}
		if connect.CodeOf(err) == connect.CodePermissionDenied {
			slog.Error(fmt.Sprintf("tags: the bridge refuses bridge-atlas, the tag loop stops: %v", err))
			return
		}
		if n > 0 || time.Since(start) >= every {
			delay = time.Second
		}
		slog.Warn(fmt.Sprintf("tags: stream ended, again in %s: %v", delay, err))
		if !sleep(ctx, delay) {
			return
		}
		delay = min(2*delay, time.Minute)
	}
}

// okEvery is how often a living stream is probed (MountainSourceStale fires past 180 s).
const okEvery = 30 * time.Second

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// tagsEnv is the tag loop's configuration (config.go), as given.
type tagsEnv struct {
	BridgeURL, TokenURL, AuthHost, PasswordFile, TokenFile string
}

// tagsFrom is c with its defaults (the token URL: authentik's public one), and whether it turns the loop on: the
// bridge's URL and a token file or a password file (the token file wins when both are set). Optional (plan/15 §5), so
// a URL without a way to sign in, or the reverse, or a URL that is not http(s), is one WARN line and the loop off.
func tagsFrom(c tagsEnv) (tagsEnv, bool) {
	if c.TokenURL == "" {
		c.TokenURL = "https://authentik.bnei.dev/application/o/token/"
	}
	switch {
	case c.BridgeURL == "" && c.PasswordFile == "" && c.TokenFile == "":
		slog.Warn("atlas: ATLAS_BRIDGE_URL is unset; System tags are off")
		return c, false
	case c.BridgeURL == "" || (c.PasswordFile == "" && c.TokenFile == ""):
		slog.Warn("atlas: ATLAS_BRIDGE_URL and ATLAS_BRIDGE_TOKEN_FILE (or ATLAS_BRIDGE_PASSWORD_FILE) go together; System tags are off")
		return c, false
	}
	for _, s := range []string{c.BridgeURL, c.TokenURL} {
		if u, err := url.Parse(s); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			slog.Warn(fmt.Sprintf("atlas: %q is not an http(s) URL; System tags are off", s))
			return c, false
		}
	}
	return c, true
}

// tagsSource is the source label of the loop's health series.
const (
	tagsSource = "bridge-tags"
	appsSource = "bridge-apps" // the Applications list the tags resolve against
)

// startTags starts the tag loop and the Applications poll it resolves against, when c turns them on. The series is
// seeded here either way: a loop that is off, or that never works, ages into MountainSourceStale.
func startTags(ctx context.Context, c tagsEnv, rl *realms.Realms, met *Metrics) {
	met.SourceOK(tagsSource, float64(time.Now().Unix()))
	ok := func() { met.SourceOK(tagsSource, float64(time.Now().Unix())) }
	met.SourceOK(appsSource, float64(time.Now().Unix()))
	okApps := func() { met.SourceOK(appsSource, float64(time.Now().Unix())) }
	c, on := tagsFrom(c)
	if !on {
		return
	}
	reg, err := realms.LoadSources()
	if err != nil {
		slog.Warn(fmt.Sprintf("atlas: %v; System tags are off", err))
		return
	}
	al, err := realms.LoadAliases(rl, reg)
	if err != nil {
		slog.Warn(fmt.Sprintf("atlas: %v; System tags are off", err))
		return
	}
	kc, err := rest.InClusterConfig()
	if err != nil {
		slog.Warn(fmt.Sprintf("atlas: not in a cluster (%v); System tags are off", err))
		return
	}
	dc, err := dynamic.NewForConfig(kc)
	if err != nil {
		slog.Warn(fmt.Sprintf("atlas: %v; System tags are off", err))
		return
	}
	var cur atomic.Pointer[realms.Systems]
	go pollSystems(ctx, func(ctx context.Context) ([]realms.Application, error) { return listApps(ctx, dc) }, al, &cur, sleepCtx, okApps)
	go runTags(ctx, newBridgeClient(c).open, cur.Load, sleepCtx, ok, okEvery)
}

var appsGVR = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}

// listApps lists every namespace's Applications, read only.
func listApps(ctx context.Context, dc dynamic.Interface) ([]realms.Application, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	l, err := dc.Resource(appsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	b, err := l.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var v struct{ Items []realms.Application }
	return v.Items, json.Unmarshal(b, &v)
}

// pollSystems keeps cur at the Systems of the latest Applications list, listed every minute; a failed list keeps the
// one before (its version too), logs, and leaves ok (source "bridge-apps") unrenewed, so a missing RBAC grant or a
// failing apiserver ages into MountainSourceStale.
func pollSystems(ctx context.Context, list func(context.Context) ([]realms.Application, error), al *realms.Aliases,
	cur *atomic.Pointer[realms.Systems], sleep func(context.Context, time.Duration) bool, ok func()) {
	for {
		if apps, err := list(ctx); err != nil {
			slog.Warn(fmt.Sprintf("tags: Applications: %v", err))
		} else {
			cur.Store(realms.NewSystems(apps, al))
			ok()
		}
		if !sleep(ctx, time.Minute) {
			return
		}
	}
}

// bridgeUser is atlas's authentik service account on the bridge (ukubi-bridge helm/values.yaml BRIDGE_API_CLIENTS),
// and its subscription's name; the bridge's API provider is ukubi-bridge-api (issuer
// https://authentik.bnei.dev/application/o/ukubi-bridge-api/, which the bridge pins).
const (
	bridgeUser     = "bridge-atlas"
	bridgeClientID = "ukubi-bridge-api"
)

// bridgeClient signs in as bridge-atlas with its projected ServiceAccount token (fileToken) or, when only the password
// file is set, with a client_credentials grant (infra-bootstrap ADR-0066), as kwatch and
// agent-fleet core do: the service account's app password read from its file at each sign-in, so a rotation needs no
// restart. A call refused Unauthenticated (a token revoked or past its end) drops the token, and the next signs in.
type bridgeClient struct {
	api    bridgev1connect.BridgeServiceClient
	mu     sync.Mutex // src is swapped by seen, from the probe's goroutine as well as the loop's
	src    oauth2.TokenSource
	signIn func() oauth2.TokenSource
}

func newBridgeClient(c tagsEnv) *bridgeClient {
	hc := &http.Client{Timeout: 15 * time.Second, Transport: hostRewrite{c.AuthHost, http.DefaultTransport}}
	signIn := func() oauth2.TokenSource {
		if c.TokenFile != "" {
			return fileToken(c.TokenFile)
		}
		return oauth2.ReuseTokenSource(nil, passwordGrant{c, hc})
	}
	// no client timeout: a Subscribe stream lives until its token ends; each unary call has its own (call)
	return &bridgeClient{api: bridgev1connect.NewBridgeServiceClient(&http.Client{}, c.BridgeURL), src: signIn(), signIn: signIn}
}

// fileToken is atlas's projected ServiceAccount token (audience ukubi-bridge, expirationSeconds 3600), read from its
// file at every call: the kubelet renews the file at 80% of its life, so no token is cached here and none is stale.
// The bridge ends a stream at its token's exp, so runTags's next stream reads the renewed file.
type fileToken string

func (f fileToken) Token() (*oauth2.Token, error) {
	b, err := os.ReadFile(string(f))
	if err != nil {
		return nil, err
	}
	t := strings.TrimSpace(string(b))
	if t == "" {
		return nil, errors.New("token file is empty")
	}
	return &oauth2.Token{AccessToken: t, TokenType: "Bearer"}, nil
}

// passwordGrant is one sign-in, the password read from its file then.
type passwordGrant struct {
	c  tagsEnv
	hc *http.Client
}

func (p passwordGrant) Token() (*oauth2.Token, error) {
	b, err := os.ReadFile(p.c.PasswordFile)
	if err != nil {
		return nil, err
	}
	cc := clientcredentials.Config{ClientID: bridgeClientID, TokenURL: p.c.TokenURL, AuthStyle: oauth2.AuthStyleInParams,
		EndpointParams: url.Values{"username": {bridgeUser}, "password": {strings.TrimSpace(string(b))}}}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), oauth2.HTTPClient, p.hc), 15*time.Second)
	defer cancel()
	return cc.Token(ctx)
}

// hostRewrite sends the token request to authentik's in-cluster Service as its public host (ATLAS_BRIDGE_AUTH_HOST),
// so the token's issuer is the one the bridge pins.
type hostRewrite struct {
	host string
	base http.RoundTripper
}

func (h hostRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	if h.host != "" {
		r = r.Clone(r.Context())
		r.Host = h.host
		r.Header.Set("X-Forwarded-Proto", "https")
	}
	return h.base.RoundTrip(r)
}

// bearer is m as a request with the current token; a failed sign-in is Unavailable (the next stream retries it).
func bearer[T any](b *bridgeClient, m *T) (*connect.Request[T], error) {
	b.mu.Lock()
	src := b.src
	b.mu.Unlock()
	tok, err := src.Token()
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("sign-in: %w", err))
	}
	r := connect.NewRequest(m)
	r.Header().Set("Authorization", "Bearer "+tok.AccessToken)
	return r, nil
}

// seen drops the token on Unauthenticated, then returns err.
func (b *bridgeClient) seen(err error) error {
	if connect.CodeOf(err) == connect.CodeUnauthenticated {
		b.mu.Lock()
		b.src = b.signIn()
		b.mu.Unlock()
	}
	return err
}

func (b *bridgeClient) open(ctx context.Context) (bridgeTags, error) {
	// every type (the client table grants bridge-atlas dev.bnei., any subject), so every event is tagged
	r, err := bearer(b, &bridgev1.SubscribeRequest{Name: bridgeUser, Types: []string{"dev.bnei."}})
	if err != nil {
		return nil, err
	}
	s, err := b.api.Subscribe(ctx, r)
	if err != nil {
		return nil, b.seen(err)
	}
	return &bridgeStream{b, s}, nil
}

// bridgeStream is one Subscribe stream and the unary calls beside it.
type bridgeStream struct {
	b *bridgeClient
	s *connect.ServerStreamForClient[bridgev1.SubscribeResponse]
}

func (s *bridgeStream) Next(context.Context) (bridgeEvent, error) {
	if !s.s.Receive() {
		if err := s.s.Err(); err != nil {
			return bridgeEvent{}, s.b.seen(err)
		}
		return bridgeEvent{}, io.EOF // the bridge ended it (a replaced or gapped stream reconnects)
	}
	m := s.s.Msg()
	if e := m.GetEvent(); e != nil {
		return bridgeEvent{Seq: e.GetSeq(), Source: e.GetSource(), Type: e.GetType(), Data: e.GetData()}, nil
	}
	if v := m.GetGap().GetVersion(); v != 0 {
		return bridgeEvent{GapVersion: v}, nil
	}
	return bridgeEvent{}, errors.New("tags: a message with neither an event nor a gap version")
}

// call runs one unary call with its own timeout.
func call[T, R any](ctx context.Context, b *bridgeClient, f func(context.Context, *connect.Request[T]) (*connect.Response[R], error), m *T) error {
	r, err := bearer(b, m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err = f(ctx, r)
	return b.seen(err)
}

func (s *bridgeStream) Ack(ctx context.Context, seq int64) error {
	return call(ctx, s.b, s.b.api.Ack, &bridgev1.AckRequest{Subscription: bridgeUser, Seq: seq})
}

func (s *bridgeStream) ConfirmGap(ctx context.Context, v int64) error {
	return call(ctx, s.b, s.b.api.ConfirmGap, &bridgev1.ConfirmGapRequest{Subscription: bridgeUser, Version: v})
}

func (s *bridgeStream) Tag(ctx context.Context, tags []eventTag) error {
	m := &bridgev1.TagRequest{}
	for _, t := range tags {
		m.Tags = append(m.Tags, &bridgev1.EventTag{Seq: t.Seq, System: t.System, Resolution: t.Resolution, RegistryVersion: t.RegistryVersion})
	}
	return call(ctx, s.b, s.b.api.Tag, m)
}

func (s *bridgeStream) Close() error { return s.s.Close() }
