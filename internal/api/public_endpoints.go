package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// Anything served under /api/ normally requires a dashboard session. These two
// mechanisms punch holes in that, and both are exercised in the middleware in
// New (see the UseMiddleware block there):
//
//  1. an operation carrying Metadata["public"] = true, which any plugin — in
//     this repo or out of it — can set on its own routes; and
//  2. a path in publicExactPaths or under a publicSubtreePrefixes entry.
//
// Both are legitimate. Login cannot require a session, and a payment webhook
// arrives with no cookie at all. What neither had was an inventory: a route
// could become reachable without authentication and nothing anywhere would say
// so. PublicOperations plus the registry test in public_endpoints_test.go make
// that visible — a new hole fails the build until someone writes it down.

// publicExactPaths are exempt, and only at exactly these paths.
//
// This used to be prefix matching, and prefix matching had already misfired:
// "/api/auth/logout" also matched "/api/auth/logout-all", silently exempting an
// endpoint that nukes every session a user has. It was not exploitable — the
// handler re-authenticates (auth.go:189) — but it survived on the handler
// author's care rather than on the gate, which is the arrangement this whole
// inventory exists to stop relying on. Exact matching means a new endpoint
// cannot inherit an exemption by sharing a name prefix with an old one.
var publicExactPaths = map[string]bool{
	"/api/auth/login":               true,
	"/api/auth/register":            true,
	"/api/auth/2fa/verify":          true,
	"/api/auth/logout":              true,
	"/api/auth/config":              true,
	"/api/auth/methods":             true,
	"/api/auth/invite/accept":       true,
	"/api/auth/forgot":              true,
	"/api/auth/reset":               true,
	"/api/auth/verify-email":        true,
	"/api/auth/resend-verification": true,
	"/api/health":                   true,
	"/api/status":                   true,
}

// publicSubtreePrefixes exempt everything beneath them. There is exactly one,
// and it has to be a subtree: webhook paths carry the workspace slug and a
// per-mailbox token as path segments, so the concrete request path is never
// known in advance.
//
// A subtree is blunt — every route any plugin mounts under it is public whether
// the author considered that or not. That is why the registry test enumerates
// concrete operations instead of trusting this list: a new webhook route shows
// up there as a diff someone has to approve.
var publicSubtreePrefixes = []string{
	"/api/webhook/",
}

// isPublicPath reports whether the auth middleware lets path through without a
// session.
func isPublicPath(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		// Routes outside /api/ never entered this gate. They are still listed
		// by PublicOperations, because "not covered by the gate" and "safe to
		// call anonymously" are different claims and only the second one is
		// worth trusting without a look.
		return true
	}
	if publicExactPaths[path] {
		return true
	}
	for _, p := range publicSubtreePrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// PublicEndpoint is one operation reachable without a dashboard session.
type PublicEndpoint struct {
	Method string
	Path   string
	// Reason is "metadata" when the operation opted in via
	// Metadata["public"], or "prefix:<p>" when a path prefix exempted it. An
	// operation can be exempt both ways; metadata wins in the report because
	// it is the deliberate, per-route choice.
	Reason string
}

func (e PublicEndpoint) String() string { return e.Method + " " + e.Path + "  (" + e.Reason + ")" }

// PublicOperations returns every registered operation that the auth middleware
// will let through unauthenticated, sorted for stable comparison.
//
// It reads the operations off the OpenAPI document rather than re-deriving them,
// so it sees exactly what was registered — including routes contributed by
// out-of-tree plugins, which is the case a hand-maintained list would miss.
func PublicOperations(api huma.API) []PublicEndpoint {
	var out []PublicEndpoint
	doc := api.OpenAPI()
	if doc == nil || doc.Paths == nil {
		return out
	}
	for path, item := range doc.Paths {
		if item == nil {
			continue
		}
		for method, op := range map[string]*huma.Operation{
			"GET":     item.Get,
			"POST":    item.Post,
			"PUT":     item.Put,
			"PATCH":   item.Patch,
			"DELETE":  item.Delete,
			"HEAD":    item.Head,
			"OPTIONS": item.Options,
			"TRACE":   item.Trace,
		} {
			if op == nil {
				continue
			}
			marked, _ := op.Metadata["public"].(bool)
			switch {
			case marked:
				out = append(out, PublicEndpoint{Method: method, Path: path, Reason: "metadata"})
			case isPublicPath(path):
				out = append(out, PublicEndpoint{Method: method, Path: path, Reason: "prefix:" + matchedPrefix(path)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// matchedPrefix returns the longest prefix that exempted path, so the report
// names the specific rule rather than just asserting one applied.
func matchedPrefix(path string) string {
	if publicExactPaths[path] {
		return path
	}
	best := ""
	for _, p := range publicSubtreePrefixes {
		if strings.HasPrefix(path, p) && len(p) > len(best) {
			best = p
		}
	}
	return best
}

// PublicGETMatcher returns a predicate reporting whether a path hosts a public
// GET endpoint — i.e. one the auth gate lets through unauthenticated. It is
// derived from PublicOperations (which reads the OpenAPI document), so it sees
// exactly what was registered, including routes contributed by out-of-tree
// plugins such as the Pro storefront.
//
// The CORS allowlist grants cross-origin reads only to these endpoints. The
// matcher keys on the concrete path, never on a prefix, so a new endpoint
// cannot inherit cross-origin access by sharing a name prefix with an old one.
func PublicGETMatcher(api huma.API) func(path string) bool {
	publicGET := map[string]bool{}
	for _, ep := range PublicOperations(api) {
		if ep.Method == http.MethodGet {
			publicGET[ep.Path] = true
		}
	}
	return func(path string) bool { return publicGET[path] }
}
