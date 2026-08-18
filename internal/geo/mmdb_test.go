package geo

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// oldTime is a mod-time older than maxCacheAge, used to exercise the
// stale-cache warning path.
var oldTime = time.Now().Add(-(maxCacheAge + time.Hour))

// buildMMDB assembles a minimal but valid GeoLite2-City database in the
// MaxMind DB binary format. Every IP in the database resolves to one record:
// country US, subdivision CA, city San Francisco. The tree is a single root
// node whose records are both direct pointers to the first data record, so the
// search hits the record in one step for any address. noCityNames drops the
// "en" city/region names so tests can exercise the resolver's fallbacks.
func buildMMDB(noCityNames bool) []byte {
	country := []byte{0xE2} // map(2)
	country = append(country, mmdbKey("iso_code", mmdbString("US"))...)
	country = append(country, mmdbKey("names", mmdbMap(1, mmdbKey("en", mmdbString("United States"))))...)

	city := mapNameValue("San Francisco", noCityNames)

	subdivInner := []byte{0xE2} // map(2)
	subdivInner = append(subdivInner, mmdbKey("iso_code", mmdbString("CA"))...)
	if noCityNames {
		subdivInner = append(subdivInner, mmdbKey("names", mmdbMap(0))...)
	} else {
		subdivInner = append(subdivInner, mmdbKey("names", mmdbMap(1, mmdbKey("en", mmdbString("California"))))...)
	}
	subdivisions := mmdbArray(subdivInner)

	var data bytes.Buffer
	data.WriteByte(0xE3) // map(3)
	data.Write(bytes.Join([][]byte{
		mmdbKey("country", country),
		mmdbKey("city", city),
		mmdbKey("subdivisions", subdivisions),
	}, nil))

	// Record pointers are relative to nodeCount+separatorSize: nodeCount=1,
	// separator=16, so pointer 17 resolves to data offset 0.
	const tree = "\x00\x00\x11\x00\x00\x11" // node 0: left=17, right=17
	var out bytes.Buffer
	out.WriteString(tree)
	out.Write(make([]byte, 16)) // data-section separator
	out.Write(data.Bytes())

	// Metadata starts with the magic marker...
	out.WriteString("\xAB\xCD\xEFMaxMind.com")
	// ...followed by a map of the fields maxminddb.Reader.Metadata needs.
	meta := []byte{0xE9} // map(9)
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

// mmdbString encodes a MaxMind-DB string (lengths here are all < 29).
func mmdbString(s string) []byte {
	return append([]byte{0x40 | byte(len(s))}, s...)
}

// mmdbUint encodes a numeric value with the given control byte and 1 data byte.
func mmdbUint(ctrl byte, v byte) []byte { return []byte{ctrl, v} }

func mmdbUint64(v uint64) []byte {
	// uint64 is type 9 > 7, so it uses the extended-type form: control byte
	// 0x00|size(8), an extra byte (type-7 = 2), then the 8 data bytes.
	return append([]byte{0x08, 0x02}, uint64Bytes(v)...)
}

func uint64Bytes(v uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, v)
	return out
}

// mmdbKey prepends an encoded key string before the encoded value.
func mmdbKey(key string, val []byte) []byte {
	out := mmdbString(key)
	return append(out, val...)
}

// mmdbMap encodes a map with the given pre-encoded value byte-slices.
func mmdbMap(n int, vals ...[]byte) []byte {
	ctr := []byte{0xE0 | byte(n)}
	return bytes.Join(append([][]byte{ctr}, vals...), nil)
}

// mmdbArray encodes an array of pre-encoded elements.
func mmdbArray(vals ...[]byte) []byte {
	// Array type is 11 > 7: extended form is control 0x00|size, then type-7=4.
	ctr := []byte{byte(len(vals)), 0x04}
	return bytes.Join(append([][]byte{ctr}, vals...), nil)
}

// mapNameValue encodes a { names: { en: <name> } } map for a city/region.
func mapNameValue(name string, empty bool) []byte {
	if empty {
		return mmdbMap(1, mmdbKey("names", mmdbMap(0)))
	}
	return mmdbMap(1, mmdbKey("names", mmdbMap(1, mmdbKey("en", mmdbString(name)))))
}

// writeMMDB writes the synthetic database into a temp dir and returns its path.
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

	country, region, city := r.Locate("203.0.113.7") // any IP resolves to the single record
	if country != "US" || region != "California" || city != "San Francisco" {
		t.Errorf("Locate = (%q,%q,%q), want (US,California,San Francisco)", country, region, city)
	}

	// IPv6-mapped and bare IPv6 addresses resolve through the same single-record tree.
	if c, _, _ := r.Locate("::ffff:203.0.113.7"); c != "US" {
		t.Errorf("IPv4-mapped Lookup country = %q, want US", c)
	}
	if c, _, _ := r.Locate("2001:db8::1"); c != "US" {
		t.Errorf("IPv6 Lookup country = %q, want US", c)
	}

	// Load replaces the previous database: a second file with no city names.
	path2 := writeMMDB(t, dir, true)
	if err := r.Load(path2); err != nil {
		t.Fatalf("Load replacement: %v", err)
	}
	country, region, city = r.Locate("203.0.113.7")
	if country != "US" || city != "" {
		t.Errorf("Locate after replacement = (%q,%q,%q), want (US,'','')", country, region, city)
	}

	// Close (with a loaded db) then Locate must return empty, never hang/panic.
	r.Close()
	if c, r2, c2 := r.Locate("203.0.113.7"); c != "" || r2 != "" || c2 != "" {
		t.Errorf("Locate after Close = (%q,%q,%q), want empty", c, r2, c2)
	}
	// A Load on a closed resolver is a no-op, not a crash or a resurrected db.
	if err := r.Load(path); err != nil {
		t.Errorf("Load after Close = %v, want nil", err)
	}
	if c, _, _ := r.Locate("203.0.113.7"); c != "" {
		t.Errorf("Locate after Close+Load = %q, want empty", c)
	}

	// Close with a pending background download (cancel set) must cancel it.
	downloading := &Resolver{cancel: func() {}}
	downloading.Close()
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

	// Manual path → loads and resolves.
	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open(manual): %v", err)
	}
	if country, _, _ := r.Locate("8.8.8.8"); country != "US" {
		t.Errorf("manual Open locate = %q, want US", country)
	}
	r.Close()

	// Manual path that cannot load → error surfaces to the caller.
	if _, err := Open(filepath.Join(dir, "nope.mmdb")); err == nil {
		t.Error("Open of a missing manual path must error")
	}
}

func TestOpenAutoCached(t *testing.T) {
	dir := t.TempDir()
	writeMMDB(t, dir, false)

	r, err := openAuto(dir, "")
	if err != nil {
		t.Fatalf("openAuto cached: %v", err)
	}
	defer r.Close()
	if country, _, _ := r.Locate("203.0.113.9"); country != "US" {
		t.Errorf("cached openAuto locate = %q, want US", country)
	}

	// A stale file still loads; it just logs a warning (age-based, not a failure).
	older := filepath.Join(dir, "GeoLite2-City.mmdb")
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	r2, err := openAuto(dir, "")
	if err != nil {
		t.Fatalf("openAuto stale cached: %v", err)
	}
	r2.Close()
}

func TestOpenAutoCorruptCachedFallsThroughToDisabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "GeoLite2-City.mmdb"), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No license key → corrupt cache falls through to disabled, not a hard error.
	r, err := openAuto(dir, "")
	if err != nil {
		t.Fatalf("openAuto corrupt/no-key: %v", err)
	}
	defer r.Close()
	if c, _, _ := r.Locate("203.0.113.1"); c != "" {
		t.Errorf("disabled resolver must return empty, got %q", c)
	}
}

func TestOpenDisabledAndLocateNilSafety(t *testing.T) {
	r, err := openAuto(t.TempDir(), "")
	if err != nil {
		t.Fatalf("openAuto disabled: %v", err)
	}
	defer r.Close()
	if c, _, _ := r.Locate("1.2.3.4"); c != "" {
		t.Errorf("disabled locate = %q, want empty", c)
	}

	var nilR *Resolver
	if c, _, _ := nilR.Locate("1.2.3.4"); c != "" {
		t.Errorf("nil resolver locate = %q, want empty", c)
	}
	nilR.Close() // must not panic
}
