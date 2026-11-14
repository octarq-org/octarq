package ontology

import (
	"fmt"

	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

// purge deletes every row this workspace owns in every declared table.
//
// A generated table belongs to the workspace that wrote it, and deleting the
// workspace has to take its rows with it. The platform looks this up under
// plugin.PurgeServiceName("ontology") before it deletes an org; without it, an
// operator who deletes a workspace leaves every declared entity's rows behind,
// silently.
//
// The table list is the applied set, not the file: a declaration that arrived in
// Go is purged the same way, and a declaration that failed to apply has no table
// to purge. Tables that do not exist are not reached — Apply created them.
func (p *Plugin) purge(orgID uint) error {
	if p.db == nil {
		return ErrNoDatabase
	}
	// NewTenantDB refuses org 0, so a purge with no workspace fails here rather
	// than deleting nothing and reporting success.
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
