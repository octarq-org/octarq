package app

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestHostRegistryValidationErrors(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	if err := reg.Bind(ctx, 1, "localhost", true, "dns"); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("Bind(localhost) = %v, want ErrHostInvalid", err)
	}
	if err := reg.Bind(ctx, 0, "acme.example", true, "dns"); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("Bind(org 0) = %v, want ErrHostOrgRequired", err)
	}
	if err := reg.Bind(ctx, 1, "acme.example", true, " "); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("Bind(empty source) = %v, want ErrHostSourceRequired", err)
	}

	if err := reg.Bind(ctx, 1, "acme.example", true, "dns"); err != nil {
		t.Fatalf("Bind org 1: %v", err)
	}
	if err := reg.Bind(ctx, 2, "acme.example", true, "dns"); !errors.Is(err, plugin.ErrHostTaken) {
		t.Errorf("Bind taken host = %v, want ErrHostTaken", err)
	}

	if err := reg.Unbind(ctx, 1, "", "dns"); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("Unbind(empty host) = %v, want ErrHostInvalid", err)
	}
	if err := reg.Unbind(ctx, 0, "acme.example", "dns"); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("Unbind(org 0) = %v, want ErrHostOrgRequired", err)
	}
	if err := reg.Unbind(ctx, 1, "acme.example", ""); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("Unbind(empty source) = %v, want ErrHostSourceRequired", err)
	}

	if err := reg.ReplaceOwned(ctx, 0, "dns", nil, true); !errors.Is(err, plugin.ErrHostOrgRequired) {
		t.Errorf("ReplaceOwned(org 0) = %v, want ErrHostOrgRequired", err)
	}
	if err := reg.ReplaceOwned(ctx, 1, "", nil, true); !errors.Is(err, plugin.ErrHostSourceRequired) {
		t.Errorf("ReplaceOwned(empty source) = %v, want ErrHostSourceRequired", err)
	}
	if err := reg.ReplaceOwned(ctx, 1, "dns", []string{"bad host"}, true); !errors.Is(err, plugin.ErrHostInvalid) {
		t.Errorf("ReplaceOwned(invalid host) = %v, want ErrHostInvalid", err)
	}
}

func TestHostRegistryReplaceOwnedDeduplicatesAndUpdates(t *testing.T) {
	db := openHostRegistryDB(t)
	reg := newHostRegistry(db)
	ctx := context.Background()

	if err := reg.Bind(ctx, 1, "one.example", false, "dns"); err != nil {
		t.Fatalf("Bind one: %v", err)
	}
	if err := reg.ReplaceOwned(ctx, 1, "dns", []string{"one.example", "two.example", "two.example"}, true); err != nil {
		t.Fatalf("ReplaceOwned: %v", err)
	}
	if !reg.ServesTraffic("one.example") {
		t.Error("existing row traffic flag was not updated")
	}
	if !reg.ServesTraffic("two.example") {
		t.Error("new row was not bound")
	}
	if err := reg.ReplaceOwned(ctx, 1, "dns", nil, true); err != nil {
		t.Fatalf("ReplaceOwned empty: %v", err)
	}
	if reg.ServesTraffic("one.example") || reg.ServesTraffic("two.example") {
		t.Error("empty ReplaceOwned did not remove the source rows")
	}
}
