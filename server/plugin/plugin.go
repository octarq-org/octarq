package plugin

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Mux interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

type Plugin interface {
	Name() string
	Models() []any
	Mount(mux Mux, ctx *Context)
}

type Starter interface {
	Start(ctx context.Context)
}

type Stopper interface {
	Stop(ctx context.Context) error
}

type Validator interface {
	Validate(ctx context.Context, host Host) error
}

type EncryptedDataReporter interface {
	HasEncryptedData() (bool, error)
}

type MCPProvider interface {
	MCPTier() string
	RegisterMCP(srv *mcp.Server)
}

type HelpProvider interface {
	HelpDocs() []HelpDoc
}

type OpenAPIContributor interface {
	OpenAPIPaths() map[string]any
	OpenAPISchemas() map[string]any
}
