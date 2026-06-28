// Package plugin defines the contract a commercial (Pro) module implements to
// extend led without forking it. It is the public, importable seam of the
// Core-as-Library split: the open-core binary depends only on this package and
// app; the private led-core consumer registers plugins through it.
//
// AutoMigrate timing: a plugin contributes its GORM models via Models(). The
// app intentionally does NOT migrate at db-open time — it waits until every
// plugin has been registered, then runs AutoMigrate over the union of core and
// plugin models exactly once. A plugin therefore never has to (and must not)
// call AutoMigrate itself; doing so early would race the core schema.
package plugin

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

// EmailEvent is a stable, external snapshot of a freshly received inbound email,
// delivered to handlers registered via Context.OnEmail. It mirrors only the
// fields a plugin needs so plugins never import led's internal/models. The full
// row (including Raw RFC822 bytes, e.g. for attachment OCR) remains reachable
// via the shared DB using ID.
//
// This is the low-latency entry point Inbox AI needs (summary/classification,
// OTP extraction): the core fires it the moment an email is stored, instead of
// a plugin having to poll for unsummarized rows.
type EmailEvent struct {
	ID         uint      // emails.id — load the full row from the DB if more is needed
	MailboxID  uint      // owning mailbox
	OrgID      uint      // tenant scope (mailbox owner) for org-scoped processing
	From       string    // envelope/header From
	To         string    // recipient the mail was routed to
	Subject    string    // header Subject
	Text       string    // plaintext body (may be empty if HTML-only)
	HTML       string    // HTML body (may be empty)
	ReceivedAt time.Time // when the MTA received it
}

// Context carries the shared dependencies a plugin needs to wire its routes.
// It exposes only stable, external types so plugins in a separate module never
// reach into led's internal packages.
type Context struct {
	// DB is the shared GORM handle. By the time Mount is called the plugin's
	// own Models() have already been migrated.
	DB *gorm.DB
	// Guard wraps a handler so it requires an authenticated dashboard session,
	// the same gate core endpoints use.
	Guard func(http.Handler) http.Handler
	// Notify delivers a notification via a configured channel. typ is the
	// channel type ("telegram", "webhook"), cfgJSON is the channel's JSON
	// config blob, and text is the message body. It mirrors notify.Send so
	// plugins never import led's internal/notify package directly.
	Notify func(ctx context.Context, typ, cfgJSON, text string) error
	// UserID extracts the authenticated user ID from the request session (0 if unauthed).
	UserID func(*http.Request) uint
	// OrgID extracts the authenticated org ID from the request session (0 if unauthed).
	OrgID func(*http.Request) uint
	// Audit writes an audit log entry asynchronously. action follows the
	// "resource.verb" convention (e.g. "subscription.create"). targetType is
	// the resource name, targetID is its primary key, meta is optional JSON
	// context (pass nil to omit). Mirrors the core h.audit() helper so plugins
	// never import led's internal/api or internal/models directly.
	Audit func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	// Encrypt seals plaintext with AES-256-GCM and returns base64(nonce||ciphertext).
	Encrypt func(plaintext []byte) (string, error)
	// Decrypt reverses Encrypt.
	Decrypt func(encoded string) ([]byte, error)
	// OnEmail registers a handler invoked (asynchronously, in its own goroutine)
	// after each inbound email is stored. Multiple plugins may register; a
	// handler must not block the request path and should bound its own work with
	// the context it captures. This is the inbound hook Inbox AI subscribes to.
	OnEmail func(handler func(EmailEvent))
	// DNS manages DNS records for a domain through the core's configured provider
	// (Cloudflare, …) so a plugin can change real records without importing led's
	// internal/dnsprovider. This is what makes "point the A record at a new IP"
	// an actual operation rather than a flag flip.
	DNS DNSManager
}

// DNSRecord is a provider-agnostic DNS record, mirroring the fields of led's
// internal dnsprovider.Record using only stable types so plugins in a separate
// module never import internal packages. An empty ID on a write means "create".
type DNSRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // A, AAAA, CNAME, TXT, MX, …
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Comment  string `json:"comment"`
	Priority *int   `json:"priority,omitempty"`
}

// DNSManager is the DNS-management seam exposed to plugins via Context.DNS. All
// operations take a led domain ID and resolve its zone + provider internally.
type DNSManager interface {
	// List returns all records in the domain's zone.
	List(ctx context.Context, domainID uint) ([]DNSRecord, error)
	// Set creates the record when r.ID is empty, otherwise updates it. Returns
	// the stored record.
	Set(ctx context.Context, domainID uint, r DNSRecord) (DNSRecord, error)
	// Delete removes a record by provider record ID.
	Delete(ctx context.Context, domainID uint, recordID string) error
}

// Plugin is a unit of Pro functionality mounted onto the core app.
type Plugin interface {
	// Name is a short identifier used in logs.
	Name() string
	// Models returns the GORM models this plugin owns. They are collected and
	// migrated together with core models before any route is served.
	Models() []any
	// Mount registers the plugin's HTTP routes (typically under /api/...) on the
	// shared API mux. Use ctx.Guard to require a session.
	Mount(mux *http.ServeMux, ctx *Context)
}

// Starter is an optional interface a Plugin may implement. If present, the app
// calls Start in a goroutine after all plugins are mounted, passing the
// server's root context so the plugin can run background work (e.g. schedulers)
// and stop cleanly on shutdown.
type Starter interface {
	Start(ctx context.Context)
}

// MenuItem represents a menu item exposed by a plugin for rendering in the UI.
type MenuItem struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Icon     string `json:"icon"`     // emoji or icon key
	Category string `json:"category"` // default category
}

// MenuProvider is an optional interface a Plugin may implement if it registers
// dynamic menu links for the frontend sidebar.
type MenuProvider interface {
	Menus() []MenuItem
}

// MCPProvider is an optional interface a Plugin may implement if it registers
// dynamic MCP tools.
type MCPProvider interface {
	RegisterMCP(srv *mcp.Server)
}

