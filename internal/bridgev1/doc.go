// Package bridgev1 is ukubi-bridge's bridge.v1 Go code, copied: the bridge module is private and no mountain build
// reads a private module (plan/15 §3, X4), so atlas's tag loop (cmd/atlas/tags.go) carries the client it uses.
// bridge.pb.go is ukubi-bridge's proto/gen/go/bridge/v1/bridge.pb.go, byte for byte; bridgev1connect/bridge.connect.go
// is its bridgev1connect file with the one import of this package rewritten. Both from ukubi-bridge 4d0bbcd (branch
// feat/forge-relist-system-tags: the Tag RPC). To update: copy both again from the bridge's commit and rewrite that
// import; never edit them here.
package bridgev1
