package tenantsql

import (
	"errors"
	"testing"
)

// intOrgModel carries a signed tenant column, the shape a plugin model may use.
type intOrgModel struct {
	ID    uint `gorm:"primaryKey"`
	OrgID int  `gorm:"column:org_id;index"`
	Name  string
}

// toUint reads a tenant id out of an untyped value. It accepts every integer
// width because plugin models and maps reach this code with whatever width
// they declared. A wrong answer here would read a foreign tenant as zero and
// let the write through, so the table is explicit.
func TestToUintReadsEveryIntegerWidth(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want uint
	}{
		{"uint", uint(1), 1},
		{"uint8", uint8(2), 2},
		{"uint16", uint16(3), 3},
		{"uint32", uint32(4), 4},
		{"uint64", uint64(5), 5},
		{"int", int(6), 6},
		{"int8", int8(7), 7},
		{"int16", int16(8), 8},
		{"int32", int32(9), 9},
		{"int64", int64(10), 10},
		{"string", "7", 0},
		{"nil", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toUint(tc.in); got != tc.want {
				t.Fatalf("toUint(%#v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestEnforceTenantScopeFillsAndRefuses(t *testing.T) {
	db := setupTestDB(t)
	tdb, err := NewTenantDB(db, 7)
	if err != nil {
		t.Fatalf("new tenant db: %v", err)
	}

	t.Run("nil values pass", func(t *testing.T) {
		if err := tdb.enforceTenantScope(nil); err != nil {
			t.Fatalf("nil: %v", err)
		}
		var nilPtr *mockOrgModel
		if err := tdb.enforceTenantScope(nilPtr); err != nil {
			t.Fatalf("nil pointer: %v", err)
		}
	})

	t.Run("a pointer entity receives the tenant", func(t *testing.T) {
		row := &mockOrgModel{}
		if err := tdb.enforceTenantScope(row); err != nil {
			t.Fatalf("empty row: %v", err)
		}
		if row.OrgID != 7 {
			t.Fatalf("OrgID = %d, want 7", row.OrgID)
		}
	})

	t.Run("a pointer entity from another tenant is refused", func(t *testing.T) {
		row := &mockOrgModel{OrgID: 9}
		if err := tdb.enforceTenantScope(row); !errors.Is(err, ErrTenantMismatch) {
			t.Fatalf("foreign row: got %v, want ErrTenantMismatch", err)
		}
	})

	t.Run("a signed tenant column is filled and checked", func(t *testing.T) {
		row := &intOrgModel{}
		if err := tdb.enforceTenantScope(row); err != nil {
			t.Fatalf("empty row: %v", err)
		}
		if row.OrgID != 7 {
			t.Fatalf("OrgID = %d, want 7", row.OrgID)
		}
		foreign := &intOrgModel{OrgID: 9}
		if err := tdb.enforceTenantScope(foreign); !errors.Is(err, ErrTenantMismatch) {
			t.Fatalf("foreign row: got %v, want ErrTenantMismatch", err)
		}
	})

	t.Run("a slice of pointers is checked element by element", func(t *testing.T) {
		rows := []*mockOrgModel{{}, {}}
		if err := tdb.enforceTenantScope(rows); err != nil {
			t.Fatalf("rows: %v", err)
		}
		for i, row := range rows {
			if row.OrgID != 7 {
				t.Fatalf("row %d OrgID = %d, want 7", i, row.OrgID)
			}
		}
		bad := []*mockOrgModel{{OrgID: 7}, {OrgID: 9}}
		if err := tdb.enforceTenantScope(bad); !errors.Is(err, ErrTenantMismatch) {
			t.Fatalf("mixed slice: got %v, want ErrTenantMismatch", err)
		}
	})

	t.Run("a map carries the tenant under any spelling", func(t *testing.T) {
		if err := tdb.enforceTenantScope(map[string]any{"Org_ID": uint8(7)}); err != nil {
			t.Fatalf("matching map: %v", err)
		}
		for _, key := range []string{"org_id", "owner_id", "orgid", "ownerid"} {
			if err := tdb.enforceTenantScope(map[string]any{key: int64(9)}); !errors.Is(err, ErrTenantMismatch) {
				t.Fatalf("%s from another tenant: got %v, want ErrTenantMismatch", key, err)
			}
		}
	})

	t.Run("values that carry no entity pass", func(t *testing.T) {
		if err := tdb.enforceTenantScope("text"); err != nil {
			t.Fatalf("string: %v", err)
		}
		var nilInside any
		if err := tdb.enforceTenantScope(&nilInside); err != nil {
			t.Fatalf("pointer to nil interface: %v", err)
		}
	})
}

// Writing an entity of another tenant must fail before it reaches the database.
func TestWriteRefusesForeignTenant(t *testing.T) {
	db := setupTestDB(t)
	tdb, err := NewTenantDB(db, 7)
	if err != nil {
		t.Fatalf("new tenant db: %v", err)
	}

	if err := tdb.Create(&mockOrgModel{OrgID: 9}).Error; !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("create: got %v, want ErrTenantMismatch", err)
	}
	if err := tdb.Save(&mockOrgModel{OrgID: 9}).Error; !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("save: got %v, want ErrTenantMismatch", err)
	}
	if err := tdb.Updates(map[string]any{"org_id": 9}).Error; !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("updates: got %v, want ErrTenantMismatch", err)
	}

	row := &mockOrgModel{}
	if err := tdb.Create(row).Error; err != nil {
		t.Fatalf("create own row: %v", err)
	}
	if row.OrgID != 7 {
		t.Fatalf("stored OrgID = %d, want 7", row.OrgID)
	}
}
