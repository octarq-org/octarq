package ontology

import (
	"fmt"

	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

func (p *Plugin) purge(orgID uint) error {
	if p.db == nil {
		return ErrNoDatabase
	}
	tdb, err := plugin.NewTenantDB(p.db, orgID)
	if err != nil {
		return err
	}
	for _, e := range p.entities {
		scope, err := tdb.TableScoped(e.table, core.ColumnOrgID)
		if err != nil {
			return err
		}
		if err := scope.Delete(nil).Error; err != nil {
			return fmt.Errorf("ontology: purge %s: %w", e.table, err)
		}
	}
	return nil
}
