package app

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/origin"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openHostRegistryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.OrgHost{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestHostRegistryBindOwnsAndServes(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	if err := reg.Bind(ctx, 1, "Acme.Example:8443", true, "dns"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	var row models.OrgHost
	if err := db.Where("host = ?", "acme.example").First(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.OrgID != 1 || !row.Traffic || row.Source != "dns" {
		t.Errorf("row = %+v, want org 1, traffic true, source dns", row)
	}
	if org, ok := reg.OwnerOf("APP.acme.example"); !ok || org != 1 {
		t.Errorf("OwnerOf(APP.acme.example) = (%d, %v), want (1, true)", org, ok)
	}
	if !reg.ServesTraffic("acme.example") {
		t.Error("ServesTraffic(acme.example) = false, want true")
	}
	if reg.ServesTraffic("app.acme.example") {
		t.Error("ServesTraffic(app.acme.example) = true, want false for an unbound host")
	}
	if !reg.Any() {
		t.Error("Any() = false, want true")
	}
}

func TestHostRegistryBindConflict(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	if err := reg.Bind(ctx, 1, "acme.example", false, "dns"); err != nil {
		t.Fatalf("first Bind: %v", err)
	}
	if err := reg.Bind(ctx, 2, "acme.example", true, "dns"); !errors.Is(err, plugin.ErrHostTaken) {
		t.Fatalf("conflicting Bind error = %v, want ErrHostTaken", err)
	}
	if org, ok := reg.OwnerOf("acme.example"); !ok || org != 1 {
		t.Fatalf("OwnerOf after conflict = (%d, %v), want (1, true)", org, ok)
	}
	if err := reg.Bind(ctx, 1, "acme.example", true, "dns-2"); err != nil {
		t.Fatalf("same-org bind from another source: %v", err)
	}
	var rows []models.OrgHost
	if err := db.Where("host = ?", "acme.example").Order("source ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows for acme.example = %d, want 2 (one per source)", len(rows))
	}
	bySource := map[string]models.OrgHost{}
	for _, row := range rows {
		bySource[row.Source] = row
	}
	if bySource["dns"].Traffic {
		t.Error("the dns row traffic flag changed when dns-2 bound the same host")
	}
	if !bySource["dns-2"].Traffic {
		t.Error("the dns-2 row must be created with traffic true")
	}
	if err := reg.Bind(ctx, 1, "acme.example", false, "dns-2"); err != nil {
		t.Fatalf("same-org same-source rebind: %v", err)
	}
	var updated models.OrgHost
	if err := db.Where("host = ? AND source = ?", "acme.example", "dns-2").First(&updated).Error; err != nil {
		t.Fatalf("read dns-2 row: %v", err)
	}
	if updated.Traffic {
		t.Error("same-source rebind did not update the traffic flag")
	}
}

func TestHostRegistryBindValidation(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	for _, host := range []string{
		"",
		"localhost",
		"127.0.0.1",
		"[::1]",
		"*.example.com",
		"acme.example/path",
		"acme.example?x=1",
		"internal",
		"bad host.example",
	} {
		if err := reg.Bind(ctx, 1, host, true, "dns"); !errors.Is(err, plugin.ErrHostInvalid) {
			t.Errorf("Bind(%q) error = %v, want ErrHostInvalid", host, err)
		}
	}

	if err := db.Create(&models.Setting{Key: models.BaseDomainSetting, Value: "app.example.com"}).Error; err != nil {
		t.Fatalf("set base: %v", err)
	}
	if err := reg.Bind(ctx, 1, "app.example.com", true, "dns"); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("Bind(base apex) error = %v, want ErrHostInvalid", err)
	}
	if err := reg.Bind(ctx, 1, "tenant.app.example.com", true, "dns"); err != nil {
		t.Errorf("Bind(tenant subdomain) error = %v, want nil", err)
	}

	if err := reg.Bind(ctx, 0, "acme.example", true, "dns"); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("Bind(org 0) error = %v, want ErrHostOrgRequired", err)
	}
	if err := reg.Bind(ctx, 1, "acme.example", true, " "); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("Bind(empty source) error = %v, want ErrHostSourceRequired", err)
	}
}

func TestHostRegistryUnbindIsOwnershipScoped(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	if err := reg.Bind(ctx, 1, "acme.example", true, "dns"); err != nil {
		t.Fatalf("bind dns: %v", err)
	}
	if err := reg.Bind(ctx, 1, "acme.example", true, "links"); err != nil {
		t.Fatalf("bind links: %v", err)
	}
	if err := reg.Bind(ctx, 2, "other.example", true, "dns"); err != nil {
		t.Fatalf("bind org 2: %v", err)
	}

	if err := reg.Unbind(ctx, 1, "acme.example", "dns"); err != nil {
		t.Fatalf("Unbind dns: %v", err)
	}
	if org, ok := reg.OwnerOf("acme.example"); !ok || org != 1 {
		t.Errorf("OwnerOf after unbinding one source = (%d, %v), want (1, true) from the links row", org, ok)
	}
	if !reg.ServesTraffic("acme.example") {
		t.Error("ServesTraffic = false after unbinding one source; the links row still serves")
	}
	if err := reg.Unbind(ctx, 1, "acme.example", "links"); err != nil {
		t.Fatalf("Unbind links: %v", err)
	}
	if _, ok := reg.OwnerOf("acme.example"); ok {
		t.Error("unbound host still resolves to an owner")
	}

	if err := reg.Unbind(ctx, 1, "other.example", "dns"); err != nil {
		t.Fatalf("Unbind other org: %v", err)
	}
	if org, ok := reg.OwnerOf("other.example"); !ok || org != 2 {
		t.Errorf("another org's host was removed: OwnerOf = (%d, %v), want (2, true)", org, ok)
	}

	if err := reg.Unbind(ctx, 1, "acme.example", "missing-source"); err != nil {
		t.Errorf("Unbind for an absent source = %v, want nil", err)
	}
	if err := reg.Unbind(ctx, 1, "", "dns"); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("Unbind(empty host) error = %v, want ErrHostInvalid", err)
	}
}

func TestHostRegistryReplaceOwnedIsTransactionalPerSource(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	for _, bind := range []struct {
		org     uint
		host    string
		traffic bool
		source  string
	}{
		{1, "keep.example", false, "keep"},
		{1, "old.example", true, "dns"},
		{1, "gone.example", true, "dns"},
		{2, "other.example", true, "dns"},
	} {
		if err := reg.Bind(ctx, bind.org, bind.host, bind.traffic, bind.source); err != nil {
			t.Fatalf("seed bind %s: %v", bind.host, err)
		}
	}

	if err := reg.ReplaceOwned(ctx, 1, "dns", []string{"new.example", "second.example", "NEW.example", "keep.example"}, true); err != nil {
		t.Fatalf("ReplaceOwned: %v", err)
	}
	for _, host := range []string{"old.example", "gone.example"} {
		if _, ok := reg.OwnerOf(host); ok {
			t.Errorf("%s survived ReplaceOwned", host)
		}
	}
	for _, host := range []string{"new.example", "second.example"} {
		org, ok := reg.OwnerOf(host)
		if !ok || org != 1 {
			t.Errorf("OwnerOf(%s) = (%d, %v), want (1, true)", host, org, ok)
		}
		if !reg.ServesTraffic(host) {
			t.Errorf("ServesTraffic(%s) = false, want true", host)
		}
	}
	var keepRows []models.OrgHost
	if err := db.Where("host = ?", "keep.example").Order("source ASC").Find(&keepRows).Error; err != nil {
		t.Fatalf("read keep rows: %v", err)
	}
	if len(keepRows) != 2 {
		t.Fatalf("keep.example rows = %d, want 2 (keep and dns)", len(keepRows))
	}
	if keepRows[0].Source != "dns" || !keepRows[0].Traffic || keepRows[1].Source != "keep" || keepRows[1].Traffic {
		t.Errorf("keep.example rows = %+v, want an independent keep source with traffic false and a dns source with traffic true", keepRows)
	}
	if org, ok := reg.OwnerOf("other.example"); !ok || org != 2 {
		t.Errorf("ReplaceOwned touched another org: OwnerOf = (%d, %v), want (2, true)", org, ok)
	}

	err := reg.ReplaceOwned(ctx, 1, "dns-2", []string{"fresh.example", "other.example"}, true)
	if !errors.Is(err, plugin.ErrHostTaken) {
		t.Fatalf("conflicting ReplaceOwned error = %v, want ErrHostTaken", err)
	}
	if _, ok := reg.OwnerOf("fresh.example"); ok {
		t.Error("a failed ReplaceOwned left a partially inserted host")
	}
	if org, ok := reg.OwnerOf("other.example"); !ok || org != 2 {
		t.Errorf("a failed ReplaceOwned changed another org's host: (%d, %v)", org, ok)
	}

	if err := reg.ReplaceOwned(ctx, 0, "dns", nil, true); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("ReplaceOwned(org 0) error = %v, want ErrHostOrgRequired", err)
	}
	if err := reg.ReplaceOwned(ctx, 1, "", nil, true); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("ReplaceOwned(empty source) error = %v, want ErrHostSourceRequired", err)
	}
	if err := reg.ReplaceOwned(ctx, 1, "dns", []string{"ok.example", "bad host"}, true); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("ReplaceOwned(invalid host) error = %v, want ErrHostInvalid", err)
	}
}

func TestHostRegistryInvalidatesResolverCache(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()
	resolver := origin.NewResolver(db)

	if err := reg.Bind(ctx, 1, "acme.example", false, "dns"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if org, ok := resolver.OwnerOf("acme.example"); !ok || org != 1 {
		t.Fatalf("resolver OwnerOf = (%d, %v), want (1, true)", org, ok)
	}
	if resolver.ServesTraffic("acme.example") {
		t.Fatal("resolver ServesTraffic cached true for a non-serving host")
	}

	if err := reg.Bind(ctx, 1, "acme.example", true, "dns"); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if !resolver.ServesTraffic("acme.example") {
		t.Error("resolver still reports the old traffic flag after a rebind")
	}

	if err := reg.Unbind(ctx, 1, "acme.example", "dns"); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if err := reg.Bind(ctx, 2, "acme.example", true, "dns"); err != nil {
		t.Fatalf("rebind to org 2: %v", err)
	}
	if org, ok := resolver.OwnerOf("acme.example"); !ok || org != 2 {
		t.Errorf("resolver OwnerOf after ownership move = (%d, %v), want (2, true)", org, ok)
	}
}

func TestHostRegistryNilDatabaseFailsClosed(t *testing.T) {
	reg := newHostRegistry(nil)
	if reg.Any() {
		t.Error("Any() = true with no database")
	}
	if _, ok := reg.OwnerOf("acme.example"); ok {
		t.Error("OwnerOf found an owner with no database")
	}
	if reg.ServesTraffic("acme.example") {
		t.Error("ServesTraffic = true with no database")
	}
	if err := reg.Bind(context.Background(), 1, "acme.example", true, "dns"); !errors.Is(err, plugin.ErrHostsUnavailable) {
		t.Errorf("Bind error = %v, want ErrHostsUnavailable", err)
	}
	if err := reg.ReplaceOwned(context.Background(), 1, "dns", nil, true); !errors.Is(err, plugin.ErrHostsUnavailable) {
		t.Errorf("ReplaceOwned error = %v, want ErrHostsUnavailable", err)
	}
}
