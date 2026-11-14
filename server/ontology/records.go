package ontology

import "context"

// ServiceRecords is the name the ontology plugin provides its write seam under.
//
// A declared entity is reachable over REST and MCP, both of which carry a
// session. A module that runs work of its own — a webhook handler, a worker —
// holds a workspace id and no session, so it cannot use either surface. This
// service is that caller's door: the same store, the same predicates, without
// an HTTP round trip to itself.
const ServiceRecords = "ontology.records"

// RecordWriter writes and reads the rows of a declared entity.
//
// The declaration still owns the shape. Values are checked against the fields
// the declaration names: an unknown key, a reserved key, a value of the wrong
// type, or a missing required field is refused, so a caller cannot write a
// column the declaration never declared.
//
// orgID is stamped as the row's workspace and as the predicate every read and
// update carries. A user-scoped entity binds the member from ctx
// (plugin.WithUserID) and refuses a call that carries none: the writer never
// falls back to the whole workspace.
//
// Role ACLs are not applied here. This is a module seam, not a user-facing
// surface: the caller is in-process code the operator already runs. The tenant
// and scope predicates are not optional and are always applied.
type RecordWriter interface {
	// Create stores one row and returns it as the REST surface renders it.
	Create(ctx context.Context, orgID uint, slug string, values map[string]any) (map[string]any, error)
	// Update applies a patch and returns the stored row.
	Update(ctx context.Context, orgID uint, slug, id string, patch map[string]any) (map[string]any, error)
	// Get reads one row. A row of another workspace is ErrNotFound.
	Get(ctx context.Context, orgID uint, slug, id string) (map[string]any, error)
	// Find returns the rows of one workspace whose declared field equals value,
	// newest first. field must be a declared field or "id"; anything else is
	// refused before SQL is built.
	Find(ctx context.Context, orgID uint, slug, field string, value any) ([]map[string]any, error)
}
