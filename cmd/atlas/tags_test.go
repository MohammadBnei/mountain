package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/MohammadBnei/mountain/internal/bridgev1"
	"github.com/MohammadBnei/mountain/internal/bridgev1/bridgev1connect"
	"github.com/MohammadBnei/mountain/realms"
)

// fakeBridge serves messages in order, then ends the stream; refuse maps a seq to the error its Tag returns.
type fakeBridge struct {
	events    []bridgeEvent
	refuse    map[int64]error
	tags      []eventTag
	acked     []int64
	confirmed []int64
}

var errStreamEnd = errors.New("stream ended")

func (f *fakeBridge) Next(context.Context) (bridgeEvent, error) {
	if len(f.events) == 0 {
		return bridgeEvent{}, errStreamEnd
	}
	ev := f.events[0]
	f.events = f.events[1:]
	return ev, nil
}

func (f *fakeBridge) Ack(_ context.Context, seq int64) error {
	f.acked = append(f.acked, seq)
	return nil
}

func (f *fakeBridge) ConfirmGap(_ context.Context, v int64) error {
	f.confirmed = append(f.confirmed, v)
	return nil
}

func (f *fakeBridge) Tag(_ context.Context, tags []eventTag) error {
	if err := f.refuse[tags[0].Seq]; err != nil {
		return err
	}
	f.tags = append(f.tags, tags...)
	return nil
}

func (f *fakeBridge) Close() error { return nil }

func testSystems(t *testing.T) *realms.Systems {
	t.Helper()
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := realms.LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	al, err := realms.LoadAliases(rl, reg)
	if err != nil {
		t.Fatal(err)
	}
	app := func(name, repo string, res ...realms.AppResource) realms.Application {
		var a realms.Application
		a.Metadata.Namespace, a.Metadata.Name = "argocd", name
		a.Spec.Sources = []realms.AppSource{{RepoURL: "git@git.bnei.dev:MohammadBnei/infra-bootstrap.git"}, {RepoURL: repo}}
		a.Status.Resources = res
		return a
	}
	cm := realms.AppResource{Kind: "ConfigMap", Namespace: "x", Name: "shared"}
	return realms.NewSystems([]realms.Application{
		app("hq", "https://git.bnei.dev/MohammadBnei/hq.git", realms.AppResource{Group: "apps", Kind: "Deployment", Namespace: "hq", Name: "hq"}),
		app("a", "", cm), app("b", "", cm),
	}, al)
}

// The adapters' payloads, in the shapes they send (ukubi-bridge cmd/kwatch settle.go appData and source.go's
// warning; internal/forgejo sets the source, the data is Forgejo's own webhook body).
const (
	kwatchSynced  = `{"sync":"Synced","previous":"OutOfSync","refs":{"app":"hq","revisions":["0123abc"]},"hints":{"system":"app:hq"}}`
	kwatchWarning = `{"refs":{"namespace":"","kind":"Node","name":"k8s-worker-02","uid":"u1","api_version":"v1","field_path":"","reason":"NodeNotReady"},"message_hash":"00","count":3,"component":"kubelet"}`
	forged        = `{"app":"hq","refs":{"app":"hq"},"owner_chain":[{"group":"apps","kind":"Deployment","namespace":"hq","name":"hq","tracking_id":"hq:apps/Deployment:hq/hq"}]}`
)

// TestTagLoop: every event is tagged then acked, at its resolution (an ambiguous tag as one row per candidate); hints
// come only from kwatch's and Forgejo's own fields, so the same hints from any other source, or a field kwatch does
// not set, are unresolved; a gap is confirmed; a tag refused for good is acked untagged; a refusal that may pass later
// stops the loop with that event unacked.
func TestTagLoop(t *testing.T) {
	sys := testSystems(t)
	f := &fakeBridge{
		refuse: map[int64]error{
			20: connect.NewError(connect.CodeNotFound, errors.New("no event")),
			21: connect.NewError(connect.CodeInvalidArgument, errors.New("check failed")),
			22: connect.NewError(connect.CodeUnavailable, errors.New("store down")),
		},
		events: []bridgeEvent{
			{GapVersion: 3},
			{Seq: 1, Source: kwatchSource, Type: "dev.bnei.deploy.synced", Data: []byte(kwatchSynced)},
			{Seq: 2, Source: kwatchSource, Type: "dev.bnei.k8s.warning", Data: []byte(kwatchWarning)},
			{Seq: 3, Source: "git.bnei.dev/MohammadBnei/hq", Type: "dev.bnei.forge.pull_request.opened", Data: []byte(`{"number":12}`)},
			{Seq: 4, Source: "git.bnei.dev/MohammadBnei/infra-bootstrap", Type: "dev.bnei.forge.push", Data: []byte(`{}`)},
			// forged: a client's Publish (its source is client/<name>) with every hint the cascade reads
			{Seq: 5, Source: "client/bridge-core", Type: "dev.bnei.deploy.synced", Data: []byte(forged)},
			{Seq: 6, Source: "client/bridge-core", Type: "dev.bnei.agent.done", Data: []byte(forged)},
			{Seq: 7, Source: "github.com/MohammadBnei/hq", Type: "dev.bnei.github.pull_request.opened", Data: []byte(forged)},
			// kwatch's own source, but the fields it does not set
			{Seq: 8, Source: kwatchSource, Type: "dev.bnei.k8s.warning", Data: []byte(forged)},
			{Seq: 9, Source: kwatchSource, Type: "dev.bnei.deploy.health", Data: []byte(`not json`)},
			{Seq: 10, Source: kwatchSource, Type: "dev.bnei.k8s.warning", Data: []byte(`{"refs":{"kind":"Node","name":"k8s-worker-02","api_version":""}}`)},
			{Seq: 11, Source: "git.bnei.dev/MohammadBnei/hq", Type: "dev.bnei.github.push", Data: []byte(`{}`)},
			{Seq: 20, Source: kwatchSource, Type: "dev.bnei.deploy.synced", Data: []byte(kwatchSynced)},
			{Seq: 21, Source: "alertmanager", Type: "dev.bnei.alert.firing", Data: []byte(`{}`)},
			{Seq: 22, Source: kwatchSource, Type: "dev.bnei.deploy.synced", Data: []byte(kwatchSynced)},
			{Seq: 23, Source: kwatchSource, Type: "dev.bnei.deploy.synced", Data: []byte(kwatchSynced)},
		}}
	n, err := tagLoop(context.Background(), f, func() *realms.Systems { return sys })
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("the Unavailable tag did not stop the loop: %v", err)
	}
	if !slices.Equal(f.confirmed, []int64{3}) {
		t.Errorf("confirmed %v, want the gap's version 3", f.confirmed)
	}
	if want := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 20, 21}; !slices.Equal(f.acked, want) || n != len(want) {
		t.Errorf("acked %v (%d), want %v (22's tag unavailable, 23 never read)", f.acked, n, want)
	}
	v := sys.Version
	want := []eventTag{
		{1, "app:hq", realms.Applied, v},
		{2, "vm:k8s-worker-02", realms.Placement, v},
		{3, "app:hq", realms.Applied, v},
		{4, "app:a", realms.Ambiguous, v}, {4, "app:b", realms.Ambiguous, v}, {4, "app:hq", realms.Ambiguous, v},
		{5, "", realms.Unresolved, v}, {6, "", realms.Unresolved, v}, {7, "", realms.Unresolved, v},
		{8, "", realms.Unresolved, v}, {9, "", realms.Unresolved, v}, {10, "", realms.Unresolved, v}, {11, "", realms.Unresolved, v},
	}
	if !reflect.DeepEqual(f.tags, want) {
		t.Errorf("tags %+v, want %+v", f.tags, want)
	}
}

// TestTagLoopNoSystems: before the first Applications list, nothing is tagged or acked.
func TestTagLoopNoSystems(t *testing.T) {
	f := &fakeBridge{events: []bridgeEvent{{Seq: 1, Source: kwatchSource, Type: "dev.bnei.deploy.synced", Data: []byte(kwatchSynced)}}}
	if _, err := tagLoop(context.Background(), f, func() *realms.Systems { return nil }); !errors.Is(err, errNoSystems) {
		t.Fatalf("got %v, want errNoSystems", err)
	}
	if len(f.acked)+len(f.tags) != 0 {
		t.Errorf("acked %v, tagged %v: want neither", f.acked, f.tags)
	}
}

// TestRunTags: a stream that tags nothing reconnects backing off (1 s, doubling, to a minute), one that acked starts
// over at 1 s, and PermissionDenied stops the loop.
func TestRunTags(t *testing.T) {
	sys := testSystems(t)
	ev := bridgeEvent{Seq: 1, Source: kwatchSource, Type: "dev.bnei.deploy.synced", Data: []byte(kwatchSynced)}
	var opens int
	var slept []time.Duration
	open := func(context.Context) (bridgeTags, error) {
		opens++
		switch opens {
		case 4: // one event, then the stream ends: progress
			return &fakeBridge{events: []bridgeEvent{ev}}, nil
		case 6:
			return &fakeBridge{events: []bridgeEvent{ev}, refuse: map[int64]error{1: connect.NewError(connect.CodePermissionDenied, errors.New("no tag right"))}}, nil
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("down"))
	}
	runTags(context.Background(), open, func() *realms.Systems { return sys }, func(_ context.Context, d time.Duration) bool {
		slept = append(slept, d)
		return true
	})
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, time.Second, 2 * time.Second}; opens != 6 || !slices.Equal(slept, want) {
		t.Errorf("%d opens, slept %v, want 6 and %v", opens, slept, want)
	}

	// no Applications yet: the same backoff, never a hot loop; capped at a minute
	slept, opens = nil, 0
	runTags(context.Background(), func(context.Context) (bridgeTags, error) {
		return &fakeBridge{events: []bridgeEvent{ev}}, nil
	}, func() *realms.Systems { return nil }, func(_ context.Context, d time.Duration) bool {
		slept = append(slept, d)
		return len(slept) < 8
	})
	if want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60}; len(slept) != len(want) || slept[0] != time.Second || slept[7] != time.Minute {
		t.Errorf("slept %v, want %v seconds", slept, want)
	}
}

// TestPollSystems: each list is a new Systems; a failed list keeps the one before.
func TestPollSystems(t *testing.T) {
	rl, _ := realms.Load()
	reg, _ := realms.LoadSources()
	al, err := realms.LoadAliases(rl, reg)
	if err != nil {
		t.Fatal(err)
	}
	var cur atomic.Pointer[realms.Systems]
	var hq realms.Application
	hq.Metadata.Name = "hq"
	lists := []error{nil, errors.New("apiserver down")}
	var versions []string
	pollSystems(context.Background(), func(context.Context) ([]realms.Application, error) {
		err := lists[0]
		lists = lists[1:]
		return []realms.Application{hq}, err
	}, al, &cur, func(context.Context, time.Duration) bool {
		versions = append(versions, cur.Load().Version)
		return len(lists) > 0
	})
	if len(versions) != 2 || versions[0] == "" || versions[0] != versions[1] {
		t.Errorf("versions %v: want the first list's, kept after the failure", versions)
	}
	if got := cur.Load().System(realms.Hints{App: "hq"}); got.Resolution != realms.Applied {
		t.Errorf("hq: %+v", got)
	}
}

// fakeServer is the bridge's API as the tag client meets it: it checks the bearer, streams events, records calls.
type fakeServer struct {
	bridgev1connect.UnimplementedBridgeServiceHandler
	mu     sync.Mutex
	tokens []string // the bearers seen, in order
	sub    *bridgev1.SubscribeRequest
	acked  []int64
	tagged []*bridgev1.EventTag
	expire bool // the first Ack is Unauthenticated, as a token past its end
}

func (s *fakeServer) seen(h http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = append(s.tokens, h.Get("Authorization"))
}

func (s *fakeServer) Subscribe(_ context.Context, r *connect.Request[bridgev1.SubscribeRequest], out *connect.ServerStream[bridgev1.SubscribeResponse]) error {
	s.seen(r.Header())
	s.sub = r.Msg
	for _, m := range []*bridgev1.SubscribeResponse{
		{Gap: &bridgev1.Gap{FromSeq: 1, ToSeq: 2, Version: 1}},
		{Event: &bridgev1.CloudEvent{Seq: 7, Source: kwatchSource, Type: "dev.bnei.deploy.synced", Data: []byte(kwatchSynced)}},
	} {
		if err := out.Send(m); err != nil {
			return err
		}
	}
	return nil
}

func (s *fakeServer) ConfirmGap(_ context.Context, r *connect.Request[bridgev1.ConfirmGapRequest]) (*connect.Response[bridgev1.ConfirmGapResponse], error) {
	s.seen(r.Header())
	return connect.NewResponse(&bridgev1.ConfirmGapResponse{}), nil
}

func (s *fakeServer) Tag(_ context.Context, r *connect.Request[bridgev1.TagRequest]) (*connect.Response[bridgev1.TagResponse], error) {
	s.seen(r.Header())
	s.tagged = append(s.tagged, r.Msg.GetTags()...)
	return connect.NewResponse(&bridgev1.TagResponse{}), nil
}

func (s *fakeServer) Ack(_ context.Context, r *connect.Request[bridgev1.AckRequest]) (*connect.Response[bridgev1.AckResponse], error) {
	s.seen(r.Header())
	if s.expire {
		s.expire = false
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("token expired"))
	}
	s.acked = append(s.acked, r.Msg.GetSeq())
	return connect.NewResponse(&bridgev1.AckResponse{}), nil
}

// TestBridgeClient: the Connect client over HTTP. It signs in as bridge-atlas with the password from its file (read
// at each sign-in), subscribes to every dev.bnei. type under its own name, confirms the gap, tags and acks; an
// Unauthenticated call drops the token, so the next stream signs in again.
func TestBridgeClient(t *testing.T) {
	var signIns atomic.Int32
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Host != "authentik.bnei.dev" || r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "ukubi-bridge-api" ||
			r.Form.Get("username") != "bridge-atlas" || r.Form.Get("password") != "pw-"+string(rune('0'+signIns.Load())) {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok" + string(rune('0'+signIns.Add(1))), "token_type": "Bearer", "expires_in": 300})
	}))
	defer auth.Close()
	fs := &fakeServer{expire: true}
	_, h := bridgev1connect.NewBridgeServiceHandler(fs)
	br := httptest.NewServer(h)
	defer br.Close()
	pw := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(pw, []byte("pw-0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newBridgeClient(tagsEnv{BridgeURL: br.URL, TokenURL: auth.URL + "/application/o/token/", AuthHost: "authentik.bnei.dev", PasswordFile: pw})
	sys := testSystems(t)
	ctx := context.Background()

	b, err := c.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n, err := tagLoop(ctx, b, func() *realms.Systems { return sys })
	_ = b.Close()
	if connect.CodeOf(err) != connect.CodeUnauthenticated || n != 0 {
		t.Fatalf("first stream: %d acked, %v; want the expired Ack's Unauthenticated", n, err)
	}
	if err := os.WriteFile(pw, []byte("pw-1"), 0o600); err != nil { // rotated: the next sign-in reads it
		t.Fatal(err)
	}
	if b, err = c.open(ctx); err != nil {
		t.Fatal(err)
	}
	n, err = tagLoop(ctx, b, func() *realms.Systems { return sys })
	if n != 1 || err == nil {
		t.Fatalf("second stream: %d acked, %v; want 1 and the stream's end", n, err)
	}
	if fs.sub.GetName() != "bridge-atlas" || !slices.Equal(fs.sub.GetTypes(), []string{"dev.bnei."}) {
		t.Errorf("subscribed %+v", fs.sub)
	}
	if !slices.Equal(fs.acked, []int64{7}) || len(fs.tagged) != 2 || fs.tagged[1].GetSystem() != "app:hq" ||
		fs.tagged[1].GetResolution() != realms.Applied || fs.tagged[1].GetRegistryVersion() != sys.Version {
		t.Errorf("acked %v, tagged %v", fs.acked, fs.tagged)
	}
	// subscribe, confirm, tag, ack (refused) on tok1; then all four again on tok2
	if want := []string{"Bearer tok1", "Bearer tok1", "Bearer tok1", "Bearer tok1", "Bearer tok2", "Bearer tok2", "Bearer tok2", "Bearer tok2"}; !slices.Equal(fs.tokens, want) {
		t.Errorf("bearers %v, want %v", fs.tokens, want)
	}
}

// TestTagsFrom: both settings or the loop is off; one alone, or a URL that is not http(s), is off with a WARN line.
// The token URL defaults to authentik's public one.
func TestTagsFrom(t *testing.T) {
	const tok = "https://authentik.bnei.dev/application/o/token/"
	for c, want := range map[tagsEnv]bool{
		{}:                                false,
		{BridgeURL: "http://bridge:8080"}: false,
		{PasswordFile: "/p"}:              false,
		{BridgeURL: "bridge:8080", PasswordFile: "/p"}:                               false,
		{BridgeURL: "http://bridge:8080", PasswordFile: "/p", TokenURL: "authentik"}: false,
		{BridgeURL: "http://bridge:8080", PasswordFile: "/p"}:                        true,
	} {
		if got, on := tagsFrom(c); on != want || (c.TokenURL == "" && got.TokenURL != tok) {
			t.Errorf("%+v: %+v %v, want %v", c, got, on, want)
		}
	}
}
