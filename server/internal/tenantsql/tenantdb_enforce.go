package tenantsql

import (
	"fmt"
	"reflect"
	"strings"
)

func (t *TenantDB) enforceTenantScope(val any) error {
	if val == nil {
		return nil
	}
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			if err := t.enforceSingleEntityScope(elem); err != nil {
				return err
			}
		}
		return nil
	}

	if v.Kind() == reflect.Map {
		iter := v.MapRange()
		for iter.Next() {
			k := strings.ToLower(fmt.Sprintf("%v", iter.Key().Interface()))
			if k == "org_id" || k == "owner_id" || k == "orgid" || k == "ownerid" {
				valInt := toUint(iter.Value().Interface())
				if valInt != 0 && valInt != t.orgID {
					return fmt.Errorf("%w: map key %s is %d, want %d", ErrTenantMismatch, iter.Key().Interface(), valInt, t.orgID)
				}
			}
		}
		return nil
	}

	return t.enforceSingleEntityScope(v)
}

func toUint(val any) uint {
	switch v := val.(type) {
	case uint:
		return v
	case uint8:
		return uint(v)
	case uint16:
		return uint(v)
	case uint32:
		return uint(v)
	case uint64:
		return uint(v)
	case int:
		return uint(v)
	case int8:
		return uint(v)
	case int16:
		return uint(v)
	case int32:
		return uint(v)
	case int64:
		return uint(v)
	default:
		return 0
	}
}

func (t *TenantDB) enforceSingleEntityScope(v reflect.Value) error {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	for _, fieldName := range []string{"OrgID", "OwnerID"} {
		f := v.FieldByName(fieldName)
		if f.IsValid() && f.CanSet() {
			switch f.Kind() {
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				curVal := f.Uint()
				if curVal == 0 {
					f.SetUint(uint64(t.orgID))
				} else if curVal != uint64(t.orgID) {
					return fmt.Errorf("%w: field %s is %d, want %d", ErrTenantMismatch, fieldName, curVal, t.orgID)
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				curVal := f.Int()
				if curVal == 0 {
					f.SetInt(int64(t.orgID))
				} else if curVal != int64(t.orgID) {
					return fmt.Errorf("%w: field %s is %d, want %d", ErrTenantMismatch, fieldName, curVal, t.orgID)
				}
			}
		}
	}
	return nil
}
