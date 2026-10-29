package extract

import "github.com/MohammadBnei/mountain/realms"

// SkipDir is skipDir for atlas's tree route (cmd/atlas/tree.go): the tree it serves leaves out what the extractors never read.
func SkipDir(rl *realms.Realms, repo, dir string) bool { return skipDir(rl, repo, dir) }
