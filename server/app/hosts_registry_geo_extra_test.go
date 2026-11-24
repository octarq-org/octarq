package app

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestHostRegistryGeoSliceValidation(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	if err := reg.Bind(ctx, 0, "acme.example", true, "dns"); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("Bind(org 0) = %v", err)
	}
	if err := reg.Bind(ctx, 1, "acme.example", true, " "); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("Bind(empty source) = %v", err)
	}
	if err := reg.Bind(ctx, 1, "localhost", true, "dns"); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("Bind(localhost) = %v", err)
	}
	if err := reg.Bind(ctx, 1, "acme.example", true, "dns"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := reg.Bind(ctx, 2, "acme.example", true, "dns"); !errors.Is(err, plugin.ErrHostTaken) {
		t.Errorf("Bind taken = %v", err)
	}
	if err := reg.Unbind(ctx, 1, "", "dns"); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("Unbind(empty host) = %v", err)
	}
	if err := reg.Unbind(ctx, 0, "acme.example", "dns"); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("Unbind(org 0) = %v", err)
	}
	if err := reg.Unbind(ctx, 1, "acme.example", ""); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("Unbind(source) = %v", err)
	}
	if err := reg.ReplaceOwned(ctx, 0, "dns", nil, true); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("ReplaceOwned(org 0) = %v", err)
	}
	if err := reg.ReplaceOwned(ctx, 1, "", nil, true); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("ReplaceOwned(source) = %v", err)
	}
}

func TestHostRegistryGeoSliceReplaceOwned(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	if err := reg.Bind(ctx, 1, "one.example", false, "dns"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := reg.ReplaceOwned(ctx, 1, "dns", []string{"one.example", "two.example", "two.example"}, true); err != nil {
		t.Fatalf("ReplaceOwned: %v", err)
	}
	if !reg.ServesTraffic("one.example") || !reg.ServesTraffic("two.example") {
		t.Error("ReplaceOwned did not update and bind hosts")
	}
	if err := reg.ReplaceOwned(ctx, 1, "dns", nil, true); err != nil {
		t.Fatalf("ReplaceOwned empty: %v", err)
	}
	if reg.ServesTraffic("one.example") || reg.ServesTraffic("two.example") {
		t.Error("ReplaceOwned empty did not remove rows")
	}
}

func TestHostUniqueViolationGeoSlice(t *testing.T) {
	if isHostUniqueViolation(nil) {
		t.Error("nil error reported as unique violation")
	}
	if !isHostUniqueViolation(errors.New("UNIQUE constraint failed: org_hosts.host")) {
		t.Error("sqlite unique text not detected")
	}
	if isHostUniqueViolation(errors.New("disk I/O error")) {
		t.Error("generic error reported as unique violation")
	}
}
