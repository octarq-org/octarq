package geo

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func buildMMDB(noCityNames bool) []byte {
	country := []byte{0xE2}
	country = append(country, mmdbKey("iso_code", mmdbString("US"))...)
	country = append(country, mmdbKey("names", mmdbMap(1, mmdbKey("en", mmdbString("United States"))))...)

	city := mapNameValue("San Francisco", noCityNames)

	subdivInner := []byte{0xE2}
	subdivInner = append(subdivInner, mmdbKey("iso_code", mmdbString("CA"))...)
	if noCityNames {
		subdivInner = append(subdivInner, mmdbKey("names", mmdbMap(0))...)
	} else {
		subdivInner = append(subdivInner, mmdbKey("names", mmdbMap(1, mmdbKey("en", mmdbString("California"))))...)
	}
	subdivisions := mmdbArray(subdivInner)

	var data bytes.Buffer
	data.WriteByte(0xE3)
	data.Write(bytes.Join([][]byte{
		mmdbKey("country", country),
		mmdbKey("city", city),
		mmdbKey("subdivisions", subdivisions),
	}, nil))

	const tree = "\x00\x00\x11\x00\x00\x11"
	var out bytes.Buffer
	out.WriteString(tree)
	out.Write(make([]byte, 16))
	out.Write(data.Bytes())

	out.WriteString("\xAB\xCD\xEFMaxMind.com")
	meta := []byte{0xE9}
	meta = append(meta, mmdbKey("binary_format_major_version", mmdbUint(0xA1, 2))...)
	meta = append(meta, mmdbKey("binary_format_minor_version", mmdbUint(0xA1, 0))...)
	meta = append(meta, mmdbKey("build_epoch", mmdbUint64(0))...)
	meta = append(meta, mmdbKey("database_type", mmdbString("GeoLite2-City"))...)
	meta = append(meta, mmdbKey("description", mmdbMap(0))...)
	meta = append(meta, mmdbKey("ip_version", mmdbUint(0xA1, 6))...)
	meta = append(meta, mmdbKey("languages", mmdbArray(mmdbString("en")))...)
	meta = append(meta, mmdbKey("node_count", mmdbUint(0xA1, 1))...)
	meta = append(meta, mmdbKey("record_size", mmdbUint(0xA1, 24))...)
	out.Write(meta)
	return out.Bytes()
}

func mmdbString(s string) []byte {
	return append([]byte{0x40 | byte(len(s))}, s...)
}

func mmdbUint(ctrl byte, v byte) []byte { return []byte{ctrl, v} }

func mmdbUint64(v uint64) []byte {
	return append([]byte{0x08, 0x02}, uint64Bytes(v)...)
}

func uint64Bytes(v uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, v)
	return out
}

func mmdbKey(key string, val []byte) []byte {
	out := mmdbString(key)
	return append(out, val...)
}

func mmdbMap(n int, vals ...[]byte) []byte {
	ctr := []byte{0xE0 | byte(n)}
	return bytes.Join(append([][]byte{ctr}, vals...), nil)
}

func mmdbArray(vals ...[]byte) []byte {
	ctr := []byte{byte(len(vals)), 0x04}
	return bytes.Join(append([][]byte{ctr}, vals...), nil)
}

func mapNameValue(name string, empty bool) []byte {
	if empty {
		return mmdbMap(1, mmdbKey("names", mmdbMap(0)))
	}
	return mmdbMap(1, mmdbKey("names", mmdbMap(1, mmdbKey("en", mmdbString(name)))))
}

func writeMMDB(t *testing.T, dir string, noCityNames bool) string {
	t.Helper()
	path := filepath.Join(dir, "GeoLite2-City.mmdb")
	if err := os.WriteFile(path, buildMMDB(noCityNames), 0o644); err != nil {
		t.Fatalf("write mmdb: %v", err)
	}
	return path
}

func TestLoadAndLocate(t *testing.T) {
	r := &Resolver{}
	dir := t.TempDir()
	path := writeMMDB(t, dir, false)

	if err := r.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer r.Close()

	country, region, city := r.Locate("203.0.113.7")
	if country != "US" || region != "California" || city != "San Francisco" {
		t.Errorf("Locate = (%q,%q,%q), want (US,California,San Francisco)", country, region, city)
	}

	if c, _, _ := r.Locate("::ffff:203.0.113.7"); c != "US" {
		t.Errorf("IPv4-mapped Lookup country = %q, want US", c)
	}
	if c, _, _ := r.Locate("2001:db8::1"); c != "US" {
		t.Errorf("IPv6 Lookup country = %q, want US", c)
	}

	path2 := writeMMDB(t, dir, true)
	if err := r.Load(path2); err != nil {
		t.Fatalf("Load replacement: %v", err)
	}
	country, region, city = r.Locate("203.0.113.7")
	if country != "US" || city != "" {
		t.Errorf("Locate after replacement = (%q,%q,%q), want (US,'','')", country, region, city)
	}

	r.Close()
	if c, r2, c2 := r.Locate("203.0.113.7"); c != "" || r2 != "" || c2 != "" {
		t.Errorf("Locate after Close = (%q,%q,%q), want empty", c, r2, c2)
	}
	if err := r.Load(path); err != nil {
		t.Errorf("Load after Close = %v, want nil", err)
	}
	if c, _, _ := r.Locate("203.0.113.7"); c != "" {
		t.Errorf("Locate after Close+Load = %q, want empty", c)
	}
}

func TestLoadErrors(t *testing.T) {
	r := &Resolver{}

	if err := r.Load(filepath.Join(t.TempDir(), "missing.mmdb")); err == nil {
		t.Error("Load of a missing file must error")
	}
	if err := r.Load(filepath.Join(t.TempDir(), "")); err == nil {
		t.Error("Load of a directory path must error")
	}

	garbage := filepath.Join(t.TempDir(), "garbage.mmdb")
	if err := os.WriteFile(garbage, []byte("this is not an mmdb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Load(garbage); err == nil {
		t.Error("Load of garbage bytes must error")
	}
}

func TestOpenManualPath(t *testing.T) {
	dir := t.TempDir()
	path := writeMMDB(t, dir, false)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open(manual): %v", err)
	}
	if country, _, _ := r.Locate("8.8.8.8"); country != "US" {
		t.Errorf("manual Open locate = %q, want US", country)
	}
	r.Close()

	if _, err := Open(filepath.Join(dir, "nope.mmdb")); err == nil {
		t.Error("Open of a missing manual path must error")
	}
}

func TestOpenDisabledAndLocateNilSafety(t *testing.T) {
	r, err := Open("")
	if err != nil {
		t.Fatalf("Open disabled: %v", err)
	}
	defer r.Close()
	if c, _, _ := r.Locate("1.2.3.4"); c != "" {
		t.Errorf("disabled locate = %q, want empty", c)
	}

	var nilR *Resolver
	if c, _, _ := nilR.Locate("1.2.3.4"); c != "" {
		t.Errorf("nil resolver locate = %q, want empty", c)
	}
	nilR.Close()
}
