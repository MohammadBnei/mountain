package extract

import (
	"slices"
	"testing"
)

// TestForwardAuthNames: the declared Middlewares and the file provider's forwardAuths (<name>@file), and nothing else.
func TestForwardAuthNames(t *testing.T) {
	traefik := func(enabled string) sourceFile {
		return sourceFile{repo: "infra-bootstrap", path: "gitops/platform/values/traefik/values.yaml", b: []byte("providers:\n  file:\n    enabled: " + enabled +
			"\n    content:\n      http:\n        middlewares:\n          gate-forwardauth:\n            forwardAuth: {address: http://x}\n          origin-lock:\n            ipAllowList: {}\n")}
	}
	crd := sourceFile{repo: "infra-bootstrap", path: "gitops/bootstrap/gate-forwardauth-middleware.yaml", b: []byte("kind: Middleware\nmetadata: {name: gate-forwardauth}\n")}
	other := sourceFile{repo: "app", path: "gitops/bootstrap/x-forwardauth-middleware.yaml", b: []byte("kind: Middleware\nmetadata: {name: x}\n")}
	if got, want := forwardAuthNames([]sourceFile{crd, traefik("true"), other}), []string{"gate-forwardauth", "gate-forwardauth@file"}; !slices.Equal(got, want) {
		t.Errorf("enabled: %v, want %v", got, want)
	}
	if got, want := forwardAuthNames([]sourceFile{crd, traefik("false")}), []string{"gate-forwardauth"}; !slices.Equal(got, want) {
		t.Errorf("disabled: %v, want %v", got, want)
	}
}
