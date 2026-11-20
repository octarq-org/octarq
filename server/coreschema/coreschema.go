package coreschema

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
)

func Models() []any {
	return models.AllModels()
}

func CheckMirror(db *gorm.DB, mirror any) error {
	if db == nil {
		return fmt.Errorf("coreschema: nil db")
	}

	parsed, err := gormschema.Parse(mirror, &sync.Map{}, db.NamingStrategy)
	if err != nil {
		return fmt.Errorf("coreschema: parse mirror %T: %w", mirror, err)
	}
	table := parsed.Table
	if table == "" {
		return fmt.Errorf("coreschema: mirror %T resolves to an empty table name", mirror)
	}

	if !db.Migrator().HasTable(table) {
		return fmt.Errorf("coreschema: mirror %T names table %q, which does not exist in the core schema — "+
			"either the table was renamed or it is not a core table at all", mirror, table)
	}

	columnTypes, err := db.Migrator().ColumnTypes(table)
	if err != nil {
		return fmt.Errorf("coreschema: read columns of %q: %w", table, err)
	}
	actual := make(map[string]bool, len(columnTypes))
	for _, c := range columnTypes {
		actual[strings.ToLower(c.Name())] = true
	}

	var missing []string
	for _, f := range parsed.Fields {
		if f.DBName == "" {
			continue
		}
		if !actual[strings.ToLower(f.DBName)] {
			missing = append(missing, f.DBName)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)

	have := make([]string, 0, len(actual))
	for name := range actual {
		have = append(have, name)
	}
	sort.Strings(have)

	return fmt.Errorf("coreschema: mirror %T is out of date with core table %q: "+
		"declares column(s) %s, which the table does not have (table has: %s)",
		mirror, table, strings.Join(missing, ", "), strings.Join(have, ", "))
}
