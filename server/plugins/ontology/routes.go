package ontology

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

// maxBodyBytes bounds a write body. A declaration carries no size limits of its
// own, so the platform supplies one.
const maxBodyBytes = 1 << 20

// requestCtx is embedded in every input. huma fills it before the handler runs,
// and the role check needs the request it carries: the huma middleware
// authenticates, but the ACL is judged here, from the session on that request.
type requestCtx struct {
	Ctx huma.Context `hidden:"true"`
}

func (r *requestCtx) Resolve(c huma.Context) []error { r.Ctx = c; return nil }

type catalogInput struct{ requestCtx }

type listInput struct {
	requestCtx
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Page size."`
	Offset int    `query:"offset" default:"0" minimum:"0" doc:"Rows to skip."`
	Sort   string `query:"sort" doc:"Comma-separated field names. Prefix a name with - for descending."`
}

type getInput struct {
	requestCtx
	ID string `path:"id" doc:"Record id."`
}

// createInput carries the body verbatim. The fields are runtime data, so huma
// cannot bind them to a struct: core.DecodeValues is the only validator, and it
// needs the exact bytes to keep integer precision and to reject unknown keys.
type createInput struct {
	requestCtx
	Body json.RawMessage
}

type updateInput struct {
	requestCtx
	ID   string `path:"id" doc:"Record id."`
	Body json.RawMessage
}

type deleteInput struct {
	requestCtx
	ID string `path:"id" doc:"Record id."`
}

type catalogBody struct {
	Entities []catalogEntry `json:"entities"`
}

type listBody struct {
	Items  []map[string]any `json:"items"`
	Total  int64            `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

type okBody struct {
	OK bool `json:"ok"`
}

type catalogOutput struct{ Body catalogBody }
type listOutput struct{ Body listBody }
type recordOutput struct{ Body map[string]any }
type okOutput struct{ Body okBody }

// registerRoutes publishes the catalog and one resource per entity.
//
// Paths are literal per entity rather than one wildcard route: the platform's
// route registry refuses two declarations of the same METHOD+path, and a literal
// path also lets each operation carry its own tier metadata.
func (p *Plugin) registerRoutes(ctx *plugin.Context) {
	if ctx == nil || ctx.Huma == nil {
		return
	}
	api := ctx.Huma

	huma.Register(api, huma.Operation{
		OperationID: "data_catalog",
		Method:      http.MethodGet,
		Path:        "/api/data",
		Summary:     "List entity declarations",
		Description: "Returns every declared entity with the operations the caller's role allows.",
		Tags:        []string{"data"},
	}, p.catalog)

	for _, e := range p.entities {
		p.registerEntityRoutes(api, e)
	}
}

func (p *Plugin) registerEntityRoutes(api huma.API, e *entity) {
	base := apiPath(e.spec)
	plural := pluralLabel(e.spec)

	huma.Register(api, huma.Operation{
		OperationID: "data_list_" + e.spec.Slug,
		Method:      http.MethodGet,
		Path:        base,
		Summary:     "List " + plural,
		Tags:        []string{"data"},
		Metadata:    tierMetadata(e.acl(core.OpList)),
	}, func(ctx context.Context, in *listInput) (*listOutput, error) {
		return p.list(ctx, e, in)
	})

	huma.Register(api, huma.Operation{
		OperationID: "data_get_" + e.spec.Slug,
		Method:      http.MethodGet,
		Path:        base + "/{id}",
		Summary:     "Read one " + e.spec.Label,
		Tags:        []string{"data"},
		Metadata:    tierMetadata(e.acl(core.OpGet)),
	}, func(ctx context.Context, in *getInput) (*recordOutput, error) {
		return p.get(ctx, e, in)
	})

	huma.Register(api, huma.Operation{
		OperationID:  "data_create_" + e.spec.Slug,
		Method:       http.MethodPost,
		Path:         base,
		Summary:      "Create a " + e.spec.Label,
		Tags:         []string{"data"},
		Metadata:     tierMetadata(e.acl(core.OpCreate)),
		MaxBodyBytes: maxBodyBytes,
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"application/json": {Schema: bodySchema()},
			},
		},
	}, func(ctx context.Context, in *createInput) (*recordOutput, error) {
		return p.create(ctx, e, in)
	})

	huma.Register(api, huma.Operation{
		OperationID:  "data_update_" + e.spec.Slug,
		Method:       http.MethodPatch,
		Path:         base + "/{id}",
		Summary:      "Update one " + e.spec.Label,
		Tags:         []string{"data"},
		Metadata:     tierMetadata(e.acl(core.OpUpdate)),
		MaxBodyBytes: maxBodyBytes,
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"application/json": {Schema: bodySchema()},
			},
		},
	}, func(ctx context.Context, in *updateInput) (*recordOutput, error) {
		return p.update(ctx, e, in)
	})

	huma.Register(api, huma.Operation{
		OperationID: "data_delete_" + e.spec.Slug,
		Method:      http.MethodDelete,
		Path:        base + "/{id}",
		Summary:     "Delete one " + e.spec.Label,
		Tags:        []string{"data"},
		Metadata:    tierMetadata(e.acl(core.OpDelete)),
	}, func(ctx context.Context, in *deleteInput) (*okOutput, error) {
		return p.delete(ctx, e, in)
	})
}

// bodySchema is the OpenAPI shape of a write body. It is an open object on
// purpose: the fields are runtime data, and every value is checked by
// core.DecodeValues, which is the one place that produces a 422 for this plugin.
func bodySchema() *huma.Schema {
	return &huma.Schema{Type: "object", AdditionalProperties: true}
}

// tierMetadata declares the operation's license tier where the platform gate
// reads it. An empty tier is free and needs no metadata.
func tierMetadata(acl core.OperationACL) map[string]any {
	if acl.Tier == "" {
		return nil
	}
	return map[string]any{plugin.MetadataTier: acl.Tier}
}

func (p *Plugin) catalog(ctx context.Context, in *catalogInput) (*catalogOutput, error) {
	role, err := p.httpRole(ctx, in.Ctx)
	if err != nil {
		return nil, humaError(err)
	}
	out := &catalogOutput{Body: catalogBody{Entities: make([]catalogEntry, 0, len(p.entities))}}
	for _, e := range p.entities {
		out.Body.Entities = append(out.Body.Entities, e.catalog(func(op core.Operation) bool {
			return aclAllows(e.acl(op).Roles, role)
		}))
	}
	return out, nil
}

func (p *Plugin) list(ctx context.Context, e *entity, in *listInput) (*listOutput, error) {
	if _, err := p.authorize(ctx, in.Ctx, e, core.OpList); err != nil {
		return nil, humaError(err)
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return nil, humaError(err)
	}
	sort, err := parseSort(e, in.Sort)
	if err != nil {
		return nil, humaError(err)
	}

	rows, total, err := store{}.list(ctx, tdb, e, listParams{limit: in.Limit, offset: in.Offset, sort: sort})
	if err != nil {
		return nil, humaError(err)
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, core.FromRow(e.spec, row))
	}
	return &listOutput{Body: listBody{Items: items, Total: total, Limit: in.Limit, Offset: in.Offset}}, nil
}

func (p *Plugin) get(ctx context.Context, e *entity, in *getInput) (*recordOutput, error) {
	if _, err := p.authorize(ctx, in.Ctx, e, core.OpGet); err != nil {
		return nil, humaError(err)
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return nil, humaError(err)
	}
	row, err := store{}.get(ctx, tdb, e, in.ID)
	if err != nil {
		return nil, humaError(err)
	}
	return &recordOutput{Body: core.FromRow(e.spec, row)}, nil
}

func (p *Plugin) create(ctx context.Context, e *entity, in *createInput) (*recordOutput, error) {
	if _, err := p.authorize(ctx, in.Ctx, e, core.OpCreate); err != nil {
		return nil, humaError(err)
	}
	values, err := core.DecodeValues(e.spec, in.Body)
	if err != nil {
		return nil, humaError(err)
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return nil, humaError(err)
	}
	row, err := store{}.create(ctx, tdb, e, values)
	if err != nil {
		return nil, humaError(err)
	}
	p.audit(ctx, in.Ctx, "data.create", e, rowID(row))
	return &recordOutput{Body: core.FromRow(e.spec, row)}, nil
}

func (p *Plugin) update(ctx context.Context, e *entity, in *updateInput) (*recordOutput, error) {
	if _, err := p.authorize(ctx, in.Ctx, e, core.OpUpdate); err != nil {
		return nil, humaError(err)
	}
	patch, err := core.DecodePatch(e.spec, in.Body)
	if err != nil {
		return nil, humaError(err)
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return nil, humaError(err)
	}
	row, err := store{}.update(ctx, tdb, e, in.ID, patch)
	if err != nil {
		return nil, humaError(err)
	}
	p.audit(ctx, in.Ctx, "data.update", e, in.ID)
	return &recordOutput{Body: core.FromRow(e.spec, row)}, nil
}

func (p *Plugin) delete(ctx context.Context, e *entity, in *deleteInput) (*okOutput, error) {
	if _, err := p.authorize(ctx, in.Ctx, e, core.OpDelete); err != nil {
		return nil, humaError(err)
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return nil, humaError(err)
	}
	if err := (store{}).delete(ctx, tdb, e, in.ID); err != nil {
		return nil, humaError(err)
	}
	p.audit(ctx, in.Ctx, "data.delete", e, in.ID)
	return &okOutput{Body: okBody{OK: true}}, nil
}

// httpRole is the caller's effective role in the active workspace. The huma
// middleware has already authenticated the request; a caller without a role here
// is authenticated but not a member of the workspace it addressed.
func (p *Plugin) httpRole(ctx context.Context, c huma.Context) (string, error) {
	r, _ := humago.Unwrap(c)
	return p.roleForRequest(ctx, r)
}

// roleForRequest asks the host about a request the middleware already resolved.
//
// The auth middleware authenticates the request and hands the resulting context
// to the handler as its first argument; the *http.Request the adapter returns
// still carries the context it arrived with. A cookie session hides that,
// because the auth manager falls back to reading the cookie itself — but a
// bearer token has no cookie to fall back to, so an agent calling these routes
// with an API token would read as holding no role and be refused on every one.
// The context has to travel with the request for both.
func (p *Plugin) roleForRequest(ctx context.Context, r *http.Request) (string, error) {
	if r == nil || p.host == nil {
		return "", ErrForbidden
	}
	role := p.host.Session().OrgRole(r.WithContext(ctx))
	if role == "" {
		return "", ErrForbidden
	}
	return role, nil
}

// authorize judges one operation's ACL for this request.
func (p *Plugin) authorize(ctx context.Context, c huma.Context, e *entity, op core.Operation) (string, error) {
	role, err := p.httpRole(ctx, c)
	if err != nil {
		return "", err
	}
	if !aclAllows(e.acl(op).Roles, role) {
		return "", ErrForbidden
	}
	return role, nil
}

// tenantDB is the scoped handle every read and write goes through. The org comes
// from the authenticated context, never from the request body.
func (p *Plugin) tenantDB(ctx context.Context) (*plugin.TenantDB, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 || p.db == nil {
		return nil, ErrNoTenantScope
	}
	return plugin.NewTenantDB(p.db, orgID)
}

// audit records a write. Generated ids are UUIDs, which the audit ledger's
// numeric target does not carry, so the id travels in the metadata instead.
func (p *Plugin) audit(ctx context.Context, c huma.Context, action string, e *entity, id string) {
	if p.ctx == nil || p.ctx.Audit == nil {
		return
	}
	r, _ := humago.Unwrap(c)
	if r == nil {
		return
	}
	p.ctx.Audit(r.WithContext(ctx), action, e.spec.Slug, 0, map[string]any{"id": id})
}

// rowID reads the id out of a stored row.
func rowID(row map[string]any) string {
	if v, ok := row[core.ColumnID]; ok {
		return fmt.Sprint(v)
	}
	return ""
}
