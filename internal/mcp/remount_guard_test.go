package mcp

import (
	"net/http"
	"sync"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

// fakePlugin mimics every real plugin: it caches ctx.OrgID into a field at
// Mount and uses that field to resolve the org of later HTTP requests
// (plugins/links/plugin.go:94-96, plugins/mail/plugin.go:105, and every Pro module).
type fakePlugin struct {
	mu     sync.Mutex
	mounts int
	orgFn  func(*http.Request) uint
}

func (f *fakePlugin) Name() string        { return "fake" }
func (f *fakePlugin) Description() string { return "test double" }
func (f *fakePlugin) Models() []any       { return nil }
func (f *fakePlugin) Mount(_ plugin.Mux, ctx *plugin.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mounts++
	if ctx.OrgID != nil {
		f.orgFn = ctx.OrgID
	}
}
func (f *fakePlugin) resolve(r *http.Request) uint {
	f.mu.Lock()
	fn := f.orgFn
	f.mu.Unlock()
	if fn == nil {
		return 0
	}
	return fn(r)
}

// A networked MCP connection must NOT re-Mount shared plugin instances. Doing so
// overwrites the org resolver every HTTP handler in the process relies on, so one
// tenant's MCP session silently repoints every other tenant's REST requests.
func TestNetworkedInstanceDoesNotRemountSharedPlugins(t *testing.T) {
	f := &fakePlugin{}

	// The app's HTTP boot mounts the shared instance with a per-REQUEST resolver.
	httpOrg := func(r *http.Request) uint {
		if r == nil {
			return 0
		}
		if r.Header.Get("X-Org") == "7" {
			return 7
		}
		return 9
	}
	f.Mount(nil, &plugin.Context{OrgID: httpOrg})
	mountsAfterBoot := f.mounts

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Org", "7")
	before := f.resolve(req)

	// Tenant 42 opens an MCP SSE/stream connection.
	NewNetworkedServerInstance(nil, 42, []plugin.Plugin{f}, nil)

	if f.mounts != mountsAfterBoot {
		t.Errorf("networked MCP connection re-Mounted the shared plugin (%d -> %d mounts)", mountsAfterBoot, f.mounts)
	}
	if after := f.resolve(req); after != before {
		t.Fatalf("CROSS-TENANT LEAK: MCP connect as org 42 changed HTTP org resolution %d -> %d", before, after)
	}
}

func TestNetworkedInstanceConcurrentRace(t *testing.T) {
	f := &fakePlugin{}
	var wg sync.WaitGroup
	const iterations = 50

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			NewNetworkedServerInstance(nil, 7, []plugin.Plugin{f}, nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			NewNetworkedServerInstance(nil, 42, []plugin.Plugin{f}, nil)
		}
	}()
	wg.Wait()
}

func TestStdioInstanceMountsPlugins(t *testing.T) {
	f := &fakePlugin{}
	before := f.mounts
	NewServerInstance(nil, 1, []plugin.Plugin{f}, true)
	if f.mounts != before+1 {
		t.Fatalf("stdio MCP connection must Mount plugins (%d -> %d mounts)", before, f.mounts)
	}
}
