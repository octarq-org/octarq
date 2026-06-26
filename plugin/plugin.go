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
