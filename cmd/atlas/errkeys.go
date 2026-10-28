package main

import "github.com/MohammadBnei/mountain/pkg/errs"

// atlas's error keys (plan/15 §4): its routes answer in mountain/pkg/errs's model, with one of these keys. The page
// decides on the key, never on the message (www/js/reader.js readError). TestErrKeys fails on a key that is not one
// of these constants. The gate's refusals (adminOnly's 403, the Origin and machine checks, the hub's before the
// upgrade) stay plain text, as the game's do: they answer before any route does.
const (
	keyGraphSeq        errs.Key = "graph.seq"         // ?seq= is not a positive integer
	keyGraphNone       errs.Key = "graph.none"        // no projection yet
	keyGraphSeqMissing errs.Key = "graph.seq_missing" // no stored projection with that seq
	keyGraphNoHistory  errs.Key = "graph.no_history"  // an older projection, without a database
	keyReadPath        errs.Key = "read.path"         // no repo, or a path that is not clean and relative
	keyReadRepo        errs.Key = "read.repo"         // a repo atlas does not hold
	keyReadNotText     errs.Key = "read.not_text"
	keyReadTooLarge    errs.Key = "read.too_large"
	keyReadFile        errs.Key = "read.file" // no such file at the sha atlas holds
)

// internalError is a failure that is not the caller's to fix (internal, 500), with today's words: atlas logs its
// cause itself, so the model's own log line for a plain error is not wanted.
func internalError(say string) *errs.Error {
	return &errs.Error{Code: errs.Internal, Key: errs.KeyInternal, Say: say}
}
