package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/app"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm/logger"
)

func Generate(w io.Writer, plugins []plugin.Plugin) error {
	doc, err := Document(plugins)
	if err != nil {
		return err
	}
	b, err := Marshal(doc, plugins)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func Document(plugins []plugin.Plugin) (*huma.OpenAPI, error) {
	tmp, err := os.MkdirTemp("", "octarq-openapi-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	origDriver, hadDriver := os.LookupEnv("OCTARQ_DB_DRIVER")
	origDSN, hadDSN := os.LookupEnv("OCTARQ_DB_DSN")
	origSecret, hadSecret := os.LookupEnv("OCTARQ_SECRET_KEY")
	origAdmin, hadAdmin := os.LookupEnv("OCTARQ_ADMIN_PASSWORD")
	origListen, hadListen := os.LookupEnv("OCTARQ_LISTEN")
	origEntities, hadEntities := os.LookupEnv("OCTARQ_ENTITIES")
	origEntitiesFile, hadEntitiesFile := os.LookupEnv("OCTARQ_ENTITIES_FILE")

	defer func() {
		restoreEnv("OCTARQ_DB_DRIVER", origDriver, hadDriver)
		restoreEnv("OCTARQ_DB_DSN", origDSN, hadDSN)
		restoreEnv("OCTARQ_SECRET_KEY", origSecret, hadSecret)
		restoreEnv("OCTARQ_ADMIN_PASSWORD", origAdmin, hadAdmin)
		restoreEnv("OCTARQ_LISTEN", origListen, hadListen)
		restoreEnv("OCTARQ_ENTITIES", origEntities, hadEntities)
		restoreEnv("OCTARQ_ENTITIES_FILE", origEntitiesFile, hadEntitiesFile)
	}()

	setIfUnset("OCTARQ_DB_DRIVER", "sqlite")
	setIfUnset("OCTARQ_DB_DSN", filepath.Join(tmp, "openapi.db"))
	setIfUnset("OCTARQ_SECRET_KEY", "openapi-spec-generation-key-not-a-secret")
	setIfUnset("OCTARQ_ADMIN_PASSWORD", "openapi-spec-generation-password")
	os.Setenv("OCTARQ_LISTEN", "127.0.0.1:0")
	os.Setenv("OCTARQ_ENTITIES", `{"entities":[]}`)
	os.Setenv("OCTARQ_ENTITIES_FILE", "")

	a, err := app.New()
	if err != nil {
		return nil, fmt.Errorf("boot for spec generation: %w", err)
	}
	a.DB().Logger = logger.Discard
	for _, p := range plugins {
		a.Use(p)
	}
	capture := &specCapture{}
	a.Use(capture)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx); err != nil {
		return nil, fmt.Errorf("mount for spec generation: %w", err)
	}
	if capture.doc == nil {
		return nil, fmt.Errorf("spec generation: no OpenAPI document was captured")
	}
	return capture.doc, nil
}

func Marshal(doc *huma.OpenAPI, plugins []plugin.Plugin) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("marshal: nil OpenAPI document")
	}
	raw, err := doc.Downgrade()
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	paths, _ := m["paths"].(map[string]any)
	if paths == nil {
		paths = map[string]any{}
		m["paths"] = paths
	}
	schemas := componentSchemas(m)

	for path, item := range extraPaths() {
		if _, generated := paths[path]; !generated {
			paths[path] = item
		}
	}
	for _, p := range plugins {
		contributor, ok := p.(plugin.OpenAPIContributor)
		if !ok {
			continue
		}
		for path, item := range contributor.OpenAPIPaths() {
			if _, generated := paths[path]; !generated {
				paths[path] = item
			}
		}
		for name, schema := range contributor.OpenAPISchemas() {
			if _, generated := schemas[name]; !generated {
				schemas[name] = schema
			}
		}
	}
	if len(schemas) > 0 {
		components, _ := m["components"].(map[string]any)
		if components == nil {
			components = map[string]any{}
			m["components"] = components
		}
		components["schemas"] = schemas
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func componentSchemas(m map[string]any) map[string]any {
	components, _ := m["components"].(map[string]any)
	if components == nil {
		return map[string]any{}
	}
	schemas, _ := components["schemas"].(map[string]any)
	if schemas == nil {
		return map[string]any{}
	}
	return schemas
}

func setIfUnset(key, value string) {
	if os.Getenv(key) == "" {
		if err := os.Setenv(key, value); err != nil {
			slog.Warn("openapi: setenv failed", "key", key, "err", err)
		}
	}
}

func restoreEnv(key, orig string, had bool) {
	if had {
		if err := os.Setenv(key, orig); err != nil {
			slog.Warn("openapi: restore env failed", "key", key, "err", err)
		}
	} else {
		if err := os.Unsetenv(key); err != nil {
			slog.Warn("openapi: unset env failed", "key", key, "err", err)
		}
	}
}

type specCapture struct {
	doc *huma.OpenAPI
}

func (*specCapture) Name() string  { return "openapi-spec-capture" }
func (*specCapture) Models() []any { return nil }

func (s *specCapture) Mount(_ plugin.Mux, ctx *plugin.Context) {
	if ctx != nil && ctx.Huma != nil {
		s.doc = ctx.Huma.OpenAPI()
	}
}
