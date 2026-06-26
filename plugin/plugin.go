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

	"gorm.io/gorm"
)

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
