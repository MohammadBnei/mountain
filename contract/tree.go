package contract

// GET /api/tree?repo= answers a repo's file list at the sha atlas read: every regular file with its size. The game
// classifies the files into rooms; atlas serves only the facts.

// TreeFile is one regular file of a tree.
type TreeFile struct {
	Path string `json:"path"` // repo-relative
	Size int64  `json:"size"` // bytes
}

// Tree is GET /api/tree's answer.
type Tree struct {
	Repo      string     `json:"repo"`
	SHA       string     `json:"sha"`
	Files     []TreeFile `json:"files"`
	Truncated bool       `json:"truncated,omitempty"` // the list stopped at its cap
}
