package main

import "net/http"

// serveTree is GET /api/tree?repo= (admin-only by the route table): a repo's contract.Tree. Stub until B3 M2.
func (a *App) serveTree(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
