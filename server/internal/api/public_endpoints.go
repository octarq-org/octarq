package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

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

var publicSubtreePrefixes = []string{
	"/api/webhook/",
}

func isPublicPath(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
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

type PublicEndpoint struct {
	Method string
	Path   string
	Reason string
}

func (e PublicEndpoint) String() string { return e.Method + " " + e.Path + "  (" + e.Reason + ")" }

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

func PublicGETMatcher(api huma.API) func(path string) bool {
	var exact []string
	var templates [][]string
	for _, ep := range PublicOperations(api) {
		if ep.Method != http.MethodGet {
			continue
		}
		if strings.ContainsRune(ep.Path, '{') {
			templates = append(templates, strings.Split(ep.Path, "/"))
			continue
		}
		exact = append(exact, ep.Path)
	}
	return func(path string) bool {
		for _, p := range exact {
			if p == path {
				return true
			}
		}
		got := strings.Split(path, "/")
		for _, tmpl := range templates {
			if templateMatches(tmpl, got) {
				return true
			}
		}
		return false
	}
}

func templateMatches(tmpl, got []string) bool {
	if len(tmpl) != len(got) {
		return false
	}
	for i, seg := range tmpl {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if got[i] == "" {
				return false
			}
			continue
		}
		if seg != got[i] {
			return false
		}
	}
	return true
}
