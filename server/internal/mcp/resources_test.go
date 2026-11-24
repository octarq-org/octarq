package mcp

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listResourceInventory(t *testing.T, srv *mcp.Server) (resources, templates []string) {
	t.Helper()
	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()

	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "resource-guard", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })

	res, err := clientSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	for _, r := range res.Resources {
		resources = append(resources, r.URI)
	}

	listed, err := clientSession.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("list resource templates: %v", err)
	}
	for _, r := range listed.ResourceTemplates {
		templates = append(templates, r.URITemplate)
	}
	return resources, templates
}

func TestMCPResources_NoBusinessResources(t *testing.T) {
	forbiddenExact := []string{
		"octarq://metrics/traffic",
		"octarq://domains/health",
		"octarq://links/trending",
	}
	forbiddenPrefixes := []string{
		"octarq://metrics/",
		"octarq://domains/",
		"octarq://links/",
		"octarq://mailboxes/",
	}

	cases := []struct {
		name string
		srv  *mcp.Server
	}{
		{"stdio", NewServerInstance(nil, 1, nil, nil)},
		{"networked", NewNetworkedServerInstance(nil, 42, nil, nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resources, templates := listResourceInventory(t, tc.srv)

			for _, uri := range resources {
				if slices.Contains(forbiddenExact, uri) {
					t.Errorf("resource %q is still advertised by core", uri)
				}
			}
			for _, uri := range append(resources, templates...) {
				for _, prefix := range forbiddenPrefixes {
					if strings.HasPrefix(uri, prefix) {
						t.Errorf("core still advertises business resource %q", uri)
					}
				}
			}
		})
	}
}
