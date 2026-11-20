package ontology

import "context"

const ServiceRecords = "ontology.records"

type RecordWriter interface {
	Create(ctx context.Context, orgID uint, slug string, values map[string]any) (map[string]any, error)
	Update(ctx context.Context, orgID uint, slug, id string, patch map[string]any) (map[string]any, error)
	Get(ctx context.Context, orgID uint, slug, id string) (map[string]any, error)
	Find(ctx context.Context, orgID uint, slug, field string, value any) ([]map[string]any, error)
}
